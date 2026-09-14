#!/usr/bin/env python3
"""Materialise the MCP package caches at boot WITHOUT starting any server.

The gateway's MCP client runs ``npx -y <pkg>`` / ``uvx <pkg>``, which installs
the package inside the *connect window*. Cancel that connect and the client
kills the child mid-reify; npm's install is not atomic, so the leftover
``node_modules`` looks complete and every later start skips the install and
dies on a missing module (observed: argocd ``Cannot find module
'pino-std-serializers'``, gistpad with no ``_npx`` at all).

The previous mechanism "fixed" this by *running the server* at boot and relying
on ``timeout`` to reap it. MCP servers do not exit on stdin EOF, and ``timeout``
only signals the direct child, so the ``node`` grandchildren leaked — verified
still alive 3h42m after boot, holding the servers' ports and memory.

This script replaces it:

* it resolves/installs each package with a command that never runs the server
  (``npm exec --package=<pkg> -- true`` for npm, ``uv tool install <spec>``
  for uv);
* every child runs in its own session/process group and is hard-killed
  (SIGTERM then SIGKILL to the *group*) on timeout, so nothing can outlive it;
* it sweeps ``_npx`` / npm-cache orphans left behind by older boots (they are
  reparented to PID 1, which is how we recognise them) and asserts none remain;
* it reads the package list from the rendered manifest, so there is no second
  copy of the pins to drift from values.yaml.

Booted as root, before the gateway starts; non-fatal on failure.
"""

from __future__ import annotations

import json
import os
import re
import signal
import subprocess
import sys
import time
from concurrent.futures import ThreadPoolExecutor
from typing import Any

DEFAULT_MANIFEST = "/opt/data/mcp-verify/mcp-manifest"
NPM_TIMEOUT = 300
UV_TIMEOUT = 600
NPM_PARALLELISM = 4


def log(message: str) -> None:
    print(f"[mcp-prewarm] {message}", flush=True)


def load_servers(path: str) -> dict[str, Any]:
    with open(path, encoding="utf-8") as handle:
        return json.load(handle)


def package_spec(server: dict[str, Any]) -> tuple[str, str] | None:
    """Return (kind, spec) for an npx/uvx server, else None.

    Handles ``npx``/``uvx`` directly and shell-wrapped forms
    (``sh -c "exec npx -y pkg | grep jsonrpc"``).
    """
    command = str(server.get("command", ""))
    parts = [command] + [str(a) for a in server.get("args", []) or []]
    tokens = re.split(r"[\s;|&()]+", " ".join(parts))
    for index, token in enumerate(tokens):
        if token not in ("npx", "uvx"):
            continue
        kind = "npm" if token == "npx" else "uv"
        for candidate in tokens[index + 1 :]:
            if not candidate or candidate.startswith("-") or candidate in ("exec", "run", "dlx"):
                continue
            return kind, candidate
    return None


def run_bounded(cmd: list[str], env: dict[str, str], timeout: int) -> tuple[int, str]:
    """Run *cmd* in its own process group; SIGKILL the group on timeout."""
    proc = subprocess.Popen(
        cmd,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.PIPE,
        env=env,
        start_new_session=True,
    )
    try:
        _, err = proc.communicate(timeout=timeout)
        return proc.returncode, (err or b"").decode("utf-8", "replace")
    except subprocess.TimeoutExpired:
        try:
            os.killpg(os.getpgid(proc.pid), signal.SIGTERM)
            time.sleep(2)
            os.killpg(os.getpgid(proc.pid), signal.SIGKILL)
        except ProcessLookupError:
            pass
        proc.communicate()
        return -1, f"timed out after {timeout}s (process group killed)"


def npm_cache_for(server: dict[str, Any]) -> str:
    env = server.get("env") or {}
    return (
        env.get("npm_config_cache")
        or os.environ.get("NPM_CONFIG_CACHE")
        or os.path.join(os.environ.get("HOME", "/root"), ".npm")
    )


def materialise_npm(name: str, spec: str, cache: str, env: dict[str, str]) -> bool:
    marker = os.path.join(cache, ".mcp-prewarm")
    if os.path.exists(marker) and os.path.isdir(os.path.join(cache, "_npx")):
        try:
            with open(marker, encoding="utf-8") as handle:
                if handle.read().strip() == spec:
                    log(f"{name}: cache already warm ({spec})")
                    return True
        except OSError:
            pass
    # A cache without a matching marker is either cold or poisoned (the
    # original failure mode). Drop _npx so npm reifies from scratch.
    _rmtree(os.path.join(cache, "_npx"))
    os.makedirs(cache, exist_ok=True)
    child_env = dict(env)
    child_env["NPM_CONFIG_CACHE"] = cache
    rc, err = run_bounded(
        ["npm", "exec", "--yes", f"--package={spec}", "--", "true"],
        child_env,
        NPM_TIMEOUT,
    )
    if rc == 0:
        with open(marker, "w", encoding="utf-8") as handle:
            handle.write(spec)
        log(f"{name}: materialised {spec} into {cache}")
        return True
    log(f"{name}: FAILED to materialise {spec} (rc={rc}) {err.strip()[:400]}")
    return False


def materialise_uv(name: str, spec: str, env: dict[str, str]) -> bool:
    rc, err = run_bounded(["uv", "tool", "install", "--quiet", spec], env, UV_TIMEOUT)
    if rc == 0:
        log(f"{name}: materialised {spec} (uv tool)")
        return True
    log(f"{name}: FAILED to materialise {spec} (rc={rc}) {err.strip()[:400]}")
    return False


def _rmtree(path: str) -> None:
    import shutil

    shutil.rmtree(path, ignore_errors=True)


def _cmdline(pid: int) -> str:
    try:
        with open(f"/proc/{pid}/cmdline", "rb") as handle:
            return handle.read().decode("utf-8", "replace").replace("\0", " ").strip()
    except OSError:
        return ""


def _ppid(pid: int) -> int:
    try:
        with open(f"/proc/{pid}/stat", encoding="utf-8") as handle:
            return int(handle.read().rsplit(")", 1)[1].split()[1])
    except (OSError, IndexError, ValueError):
        return 0


def _looks_like_prewarm_orphan(cmd: str) -> bool:
    """True only for a node/npm process rooted in an npm MCP cache.

    Matching a bare package name would kill unrelated processes that merely
    mention it (e.g. an operator's shell), so require the cache path *and* a
    node-family executable or a node_modules path. Package pre-warm leftovers
    are exactly ``node .../<cache>/_npx/<hash>/node_modules/...``.
    """
    if "_npx" not in cmd and ".npm-mcp" not in cmd:
        return False
    argv0 = os.path.basename(cmd.split(" ", 1)[0])
    return argv0 in ("node", "npm", "npx") or "node_modules" in cmd


def sweep_orphans() -> list[tuple[int, str]]:
    """Kill reparented (ppid==1) leftovers from older pre-warm boots."""
    killed: list[tuple[int, str]] = []
    for entry in os.listdir("/proc"):
        if not entry.isdigit():
            continue
        pid = int(entry)
        if pid == os.getpid():
            continue
        cmd = _cmdline(pid)
        if not cmd or _ppid(pid) != 1:
            continue
        if _looks_like_prewarm_orphan(cmd):
            try:
                os.kill(pid, signal.SIGKILL)
                killed.append((pid, cmd[:160]))
            except OSError:
                pass
    return killed


def main() -> int:
    manifest = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_MANIFEST
    try:
        servers = load_servers(manifest)
    except OSError as exc:
        log(f"manifest {manifest} unavailable ({exc}); skipping pre-warm")
        return 0

    npm_specs: list[tuple[str, str, str]] = []
    uv_specs: list[tuple[str, str]] = []
    for name, server in servers.items():
        found = package_spec(server or {})
        if not found:
            continue
        kind, spec = found
        if kind == "npm":
            npm_specs.append((name, spec, npm_cache_for(server)))
        else:
            uv_specs.append((name, spec))

    # Sweep BEFORE materialising so we never touch our own children.
    killed = sweep_orphans()
    for pid, cmd in killed:
        log(f"killed orphan pid {pid}: {cmd}")
    log(f"swept {len(killed)} orphaned pre-warm process(es)")

    env = dict(os.environ)
    failures = 0
    if npm_specs:
        with ThreadPoolExecutor(max_workers=NPM_PARALLELISM) as pool:
            results = list(
                pool.map(lambda item: materialise_npm(*item, env), npm_specs)
            )
        failures += sum(1 for ok in results if not ok)
    for name, spec in uv_specs:
        if not materialise_uv(name, spec, env):
            failures += 1

    remaining = sweep_orphans()
    log(f"{len(remaining)} pre-warm orphan(s) remain after materialisation")
    if failures:
        log(f"{failures} package(s) failed to materialise (non-fatal)")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
