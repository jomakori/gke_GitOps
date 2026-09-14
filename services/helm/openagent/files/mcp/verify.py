#!/usr/bin/env python3
"""Real MCP handshake verification for the openagent gateway's MCP servers.

For every **enabled** server in the rendered manifest this performs a genuine
JSON-RPC handshake — ``initialize`` then ``tools/list`` — over stdio or
Streamable HTTP, with a per-server hard timeout and process-group kill. It then
certifies the declared ``tools.include`` surface: every tool the manifest
promises must appear in the server's ``tools/list`` result.

The client-side error Hermes prints (``Failed to connect to MCP server '<x>'``)
is opaque — it discards the server's own stderr — which is half the original
problem. So on failure this script prints that stderr verbatim.

Two modes:

* ``preflight`` — verbose. ``PASS <server> <n> tools`` per server, ``FAIL
  <server>`` plus the server's stderr on failure. Non-zero if anything failed.
  Run as a Helm/ArgoCD hook Job after a deploy.
* ``drift`` — quiet when the live surface matches. Only on drift does it print
  and exit non-zero, so a watcher can alert on *change*, not on every run.
  Run on a schedule.

Stdlib only.
"""

from __future__ import annotations

import argparse
import json
import os
import selectors
import signal
import socket
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
from typing import Any

PROTOCOL_VERSION = "2024-11-05"
DEFAULT_MANIFEST = "/mcp-verify/mcp-manifest"
STDERR_LINES = 40


class VerifyError(Exception):
    def __init__(self, message: str, stderr: str = "") -> None:
        super().__init__(message)
        self.stderr = stderr


def is_enabled(server: dict[str, Any]) -> bool:
    return bool(server.get("enabled", True))


def is_stdio(server: dict[str, Any]) -> bool:
    return "command" in server and "url" not in server


def expand(value: Any, env: dict[str, str], missing: set[str]) -> str:
    """Expand ``${VAR}`` / ``$VAR`` placeholders from env (unresolved kept)."""
    import re

    def repl(match: "re.Match[str]") -> str:
        name = match.group(1) or match.group(2)
        if name in env:
            return env[name]
        missing.add(name)
        return match.group(0)

    return re.sub(r"\$\{(\w+)\}|\$(\w+)", repl, str(value))


# ── stdio transport ───────────────────────────────────────────────────────────


class Stdio:
    def __init__(self, server: dict[str, Any]) -> None:
        self._missing: set[str] = set()
        child_env = dict(os.environ)
        for key, value in (server.get("env") or {}).items():
            child_env[str(key)] = expand(value, child_env, self._missing)
        self.missing = self._missing
        cmd = [str(server["command"])] + [str(a) for a in server.get("args", []) or []]
        self.proc = subprocess.Popen(
            cmd,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=child_env,
            start_new_session=True,
        )
        self.stderr: list[str] = []
        self._diag: list[str] = []
        self._selector = selectors.DefaultSelector()
        self._selector.register(self.proc.stdout, selectors.EVENT_READ)
        self._buf = b""
        self._stderr_thread = threading.Thread(target=self._drain_stderr, daemon=True)
        self._stderr_thread.start()

    def _drain_stderr(self) -> None:
        assert self.proc.stderr is not None
        try:
            for raw in iter(self.proc.stderr.readline, b""):
                line = raw.decode("utf-8", "replace").rstrip()
                if len(self.stderr) < STDERR_LINES:
                    self.stderr.append(line)
        except (OSError, ValueError):
            pass

    def send(self, payload: dict[str, Any]) -> None:
        assert self.proc.stdin is not None
        self.proc.stdin.write(json.dumps(payload).encode() + b"\n")
        self.proc.stdin.flush()

    def read(self, want_id: Any, deadline: float) -> dict[str, Any]:
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise VerifyError("timed out waiting for response")
            if not self._selector.select(timeout=remaining):
                raise VerifyError("timed out waiting for response")
            chunk = os.read(self.proc.stdout.fileno(), 65536)
            if not chunk:
                raise VerifyError("server closed stdout before responding")
            self._buf += chunk
            while b"\n" in self._buf:
                line, self._buf = self._buf.split(b"\n", 1)
                line = line.strip()
                if not line:
                    continue
                try:
                    message = json.loads(line)
                except json.JSONDecodeError:
                    # Banner / log line on stdout (see the grafana server).
                    self._diag.append(line.decode("utf-8", "replace")[:300])
                    continue
                if want_id is None or message.get("id") == want_id:
                    return message

    def close(self) -> None:
        try:
            if self.proc.poll() is None:
                os.killpg(os.getpgid(self.proc.pid), signal.SIGTERM)
                time.sleep(0.5)
                os.killpg(os.getpgid(self.proc.pid), signal.SIGKILL)
        except (ProcessLookupError, PermissionError):
            pass
        try:
            self.proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            pass


def handshake_stdio(server: dict[str, Any], timeout: int) -> list[str]:
    client = Stdio(server)
    deadline = time.monotonic() + timeout
    try:
        client.send(
            {
                "jsonrpc": "2.0",
                "id": 1,
                "method": "initialize",
                "params": {
                    "protocolVersion": PROTOCOL_VERSION,
                    "capabilities": {},
                    "clientInfo": {"name": "openagent-mcp-verify", "version": "1.0"},
                },
            }
        )
        response = client.read(1, deadline)
        if "error" in response:
            raise VerifyError(f"initialize error: {response['error']}")
        client.send({"jsonrpc": "2.0", "method": "notifications/initialized"})
        return _list_tools(client, deadline)
    except VerifyError as exc:
        exc.stderr = "\n".join(client.stderr)
        raise
    finally:
        client.close()


# ── Streamable HTTP transport ─────────────────────────────────────────────────


def _http_post(
    url: str,
    payload: dict[str, Any],
    session: str | None,
    timeout: int,
    extra_headers: dict[str, str] | None = None,
) -> tuple[int, dict[str, str], dict[str, Any] | None, str]:
    data = json.dumps(payload).encode()
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json, text/event-stream",
        "MCP-Protocol-Version": PROTOCOL_VERSION,
    }
    headers.update(extra_headers or {})
    if session:
        headers["Mcp-Session-Id"] = session
    request = urllib.request.Request(url, data=data, headers=headers, method="POST")
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            body = response.read().decode("utf-8", "replace")
            return response.status, dict(response.headers), _parse_body(body), body
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", "replace")
        return exc.code, dict(exc.headers), _parse_body(body), body


def _parse_body(body: str) -> dict[str, Any] | None:
    events: list[dict[str, Any]] = []
    for line in body.splitlines():
        line = line.strip()
        if line.startswith("data:"):
            try:
                events.append(json.loads(line[5:].strip()))
            except json.JSONDecodeError:
                continue
    if events:
        return events[-1]
    try:
        return json.loads(body)
    except json.JSONDecodeError:
        return None


def handshake_http(server: dict[str, Any], timeout: int) -> list[str]:
    missing: set[str] = set()
    url = expand(server["url"], dict(os.environ), missing)
    headers = {
        str(key): expand(value, dict(os.environ), missing)
        for key, value in (server.get("headers") or {}).items()
    }
    init: dict[str, Any] = {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
            "protocolVersion": PROTOCOL_VERSION,
            "capabilities": {},
            "clientInfo": {"name": "openagent-mcp-verify", "version": "1.0"},
        },
    }
    status, response_headers, message, body = _http_post(url, init, None, timeout, headers)
    if status >= 400 or not message:
        raise VerifyError(f"initialize HTTP {status}: {body.strip()[:400]}")
    if "error" in message:
        raise VerifyError(f"initialize error: {message['error']}")
    session = response_headers.get("Mcp-Session-Id")

    _http_post(
        url,
        {"jsonrpc": "2.0", "method": "notifications/initialized"},
        session,
        timeout,
        headers,
    )

    tools: list[str] = []
    cursor: str | None = None
    rpc_id = 2
    while True:
        params: dict[str, Any] = {"cursor": cursor} if cursor else {}
        payload = {"jsonrpc": "2.0", "id": rpc_id, "method": "tools/list", "params": params}
        status, _, message, body = _http_post(url, payload, session, timeout, headers)
        if status >= 400 or not message:
            raise VerifyError(f"tools/list HTTP {status}: {body.strip()[:400]}")
        if "error" in message:
            raise VerifyError(f"tools/list error: {message['error']}")
        result = message.get("result") or {}
        tools.extend(tool.get("name", "?") for tool in result.get("tools", []))
        cursor = result.get("nextCursor")
        if not cursor:
            return tools
        rpc_id += 1


def _list_tools(client: Stdio, deadline: float) -> list[str]:
    """Walk tools/list (following nextCursor) on a stdio client."""
    tools: list[str] = []
    cursor: str | None = None
    rpc_id = 2
    while True:
        params: dict[str, Any] = {"cursor": cursor} if cursor else {}
        client.send({"jsonrpc": "2.0", "id": rpc_id, "method": "tools/list", "params": params})
        response = client.read(rpc_id, deadline)
        if "error" in response:
            raise VerifyError(f"tools/list error: {response['error']}")
        result = response.get("result") or {}
        tools.extend(tool.get("name", "?") for tool in result.get("tools", []))
        cursor = result.get("nextCursor")
        if not cursor:
            return tools
        rpc_id += 1


# ── comparison ────────────────────────────────────────────────────────────────


def declared_tools(server: dict[str, Any]) -> list[str]:
    tools = server.get("tools")
    tools = tools if isinstance(tools, dict) else {}
    include = tools.get("include")
    return [str(t) for t in include] if isinstance(include, list) else []


def verify_server(name: str, server: dict[str, Any]) -> tuple[list[str], str]:
    timeout = int(server.get("connect_timeout") or 60) * 2
    if is_stdio(server):
        return handshake_stdio(server, timeout), ""
    return handshake_http(server, timeout), ""


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Verify the MCP manifest against live servers.")
    parser.add_argument("--manifest", default=DEFAULT_MANIFEST)
    parser.add_argument("--mode", choices=("preflight", "drift"), default="preflight")
    parser.add_argument("--only", action="append", default=None, help="limit to named server(s)")
    args = parser.parse_args(argv)

    with open(args.manifest, encoding="utf-8") as handle:
        servers = json.load(handle)

    selected = [
        (name, server)
        for name, server in servers.items()
        if server and is_enabled(server) and (not args.only or name in args.only)
    ]
    selected.sort()

    if args.mode == "preflight":
        print(f"== MCP preflight: {len(selected)} enabled server(s)")

    failures = 0
    for name, server in selected:
        try:
            actual, _ = verify_server(name, server)
        except VerifyError as exc:
            failures += 1
            print(f"FAIL {name}: {exc}")
            if exc.stderr:
                for line in exc.stderr.splitlines():
                    print(f"    {line}")
            continue
        except (OSError, socket.timeout) as exc:
            failures += 1
            print(f"FAIL {name}: {exc}")
            continue

        declared = declared_tools(server)
        missing = [tool for tool in declared if tool not in actual]
        if missing:
            failures += 1
            if args.mode == "preflight":
                print(f"FAIL {name}")
                print(f"    declared tools missing from live surface: {', '.join(missing)}")
            else:
                print(f"DRIFT {name}: declared tools missing from live surface: {', '.join(missing)}")
        elif args.mode == "preflight":
            print(f"PASS {name} {len(actual)} tools")

    if failures:
        noun = "failed" if args.mode == "preflight" else "drifted"
        print(f"{failures} server(s) {noun}")
        return 1
    if args.mode == "preflight":
        print("all enabled MCP servers passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
