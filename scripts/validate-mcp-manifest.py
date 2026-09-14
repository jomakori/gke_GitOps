#!/usr/bin/env python3
"""Static lint for the openagent Helm chart's Hermes MCP manifest.

The manifest is the single source of truth for which MCP servers the gateway
connects to (see ``services/helm/openagent/values.yaml`` →
``hermes-agent.config.mcp_servers``). Runtime breakage observed in the live
cluster always traced back to a manifest defect that nothing checked:

* an unpinned package (``argocd-mcp@latest``) silently changed under us;
* a stdio server with no ``idle_timeout_seconds`` / ``max_lifetime_seconds``
  stayed dead after its first crash until the pod restarted;
* a server with no ``connect_timeout`` raced the 60s default during a cold
  ``npx`` resolve;
* a "parked" server was deleted instead of disabled, so re-enabling it lost
  its configuration;
* an enabled server declared no ``tools.include`` surface, so the runtime
  check had nothing to certify.

This script fails loudly on each of those classes, both locally and in CI.

Usage
-----
    scripts/validate-mcp-manifest.py
    scripts/validate-mcp-manifest.py services/helm/openagent/values.yaml
    scripts/validate-mcp-manifest.py --path hermes-agent.config.mcp_servers

Exit status: 0 = manifest is clean, 1 = one or more violations, 2 = input
error (missing file, missing manifest block, no YAML loader available).

YAML loading
------------
Ideally stdlib-only, but the manifest is YAML and a hand-rolled parser would
be less trustworthy than a real one. We therefore use, in order:

1. PyYAML, which the repo already depends on for
   ``.useful-scripts/validate_litellm_config.sh`` (same CI images), or
2. ``yq -o=json``, which CI installs explicitly in
   ``.github/workflows/helm_lint-test.yaml`` and devbox provides locally.

The JSON is then parsed with the stdlib, so no other dependency is needed.
"""

from __future__ import annotations

import argparse
import json
import re
import shutil
import subprocess
import sys
from typing import Any

DEFAULT_VALUES = "services/helm/openagent/values.yaml"
DEFAULT_PATH = "hermes-agent.config.mcp_servers"

# Servers that exist but are deliberately disabled in the live config. They
# must stay in the manifest as ``enabled: false`` — deleting one silently
# loses its pinned package, env mapping and tool surface. See the comments in
# values.yaml for why each is parked.
PARKED_SERVERS = ("github", "gistpad", "argocd", "grafana", "bitwarden", "drawio")


class Violation:
    def __init__(self, server: str, rule: str, message: str) -> None:
        self.server = server
        self.rule = rule
        self.message = message

    def __str__(self) -> str:
        return f"FAIL {self.server:<16} [{self.rule}] {self.message}"


# ── YAML loading ──────────────────────────────────────────────────────────────


def load_values(path: str) -> dict[str, Any]:
    """Load values.yaml through PyYAML or, failing that, ``yq -o=json``."""
    try:
        import yaml  # type: ignore
    except ImportError:
        return _load_values_with_yq(path)
    with open(path, encoding="utf-8") as handle:
        return yaml.safe_load(handle)


def _load_values_with_yq(path: str) -> dict[str, Any]:
    yq = shutil.which("yq")
    if not yq:
        sys.stderr.write(
            "error: no YAML loader available (install PyYAML or mikefarah/yq)\n"
        )
        raise SystemExit(2)
    proc = subprocess.run(
        [yq, "eval", "-o=json", ".", path],
        capture_output=True,
        text=True,
    )
    if proc.returncode != 0:
        sys.stderr.write(f"error: yq failed to parse {path}:\n{proc.stderr}\n")
        raise SystemExit(2)
    return json.loads(proc.stdout)


def resolve_path(data: dict[str, Any], dotted: str) -> Any:
    node: Any = data
    for part in dotted.split("."):
        if not isinstance(node, dict) or part not in node:
            return None
        node = node[part]
    return node


def is_stdio(server: dict[str, Any]) -> bool:
    """A stdio server launches a child process; an HTTP server has a url."""
    return "command" in server and "url" not in server


def is_enabled(server: dict[str, Any]) -> bool:
    return bool(server.get("enabled", True))


# ── Package pinning ───────────────────────────────────────────────────────────


def package_specs(server: dict[str, Any]) -> list[str]:
    """Extract npm/pip package specifiers from a server's command line.

    Handles ``npx -y pkg``, ``uvx pkg`` and shell-wrapped forms such as
    ``sh -c "exec npx -y pkg | grep jsonrpc"``. Local binaries (obscura, the
    drawio deno checkout) yield no spec and are exempt: there is no registry
    package to pin.
    """
    parts = [str(server.get("command", ""))]
    parts.extend(str(arg) for arg in server.get("args", []) or [])
    tokens = re.split(r"[\s;|&()]+", " ".join(parts))
    specs: list[str] = []
    for index, token in enumerate(tokens):
        if token not in ("npx", "uvx"):
            continue
        for candidate in tokens[index + 1 :]:
            if not candidate or candidate.startswith("-"):
                continue
            if candidate in ("exec", "run", "dlx"):
                # npm exec / uvx run <cmd> — the package is a --package flag,
                # not a positional argument.
                break
            specs.append(candidate)
            break
    return specs


def pin_violation(spec: str) -> str | None:
    """Return a reason string when *spec* is not pinned to an exact version."""
    if "@latest" in spec:
        return f"unpinned package argument {spec!r} (uses @latest)"
    if "==" in spec:
        # uv/pip PEP 440 pin: plane-mcp-server==0.3.2
        name, _, version = spec.partition("==")
        if name and version and not version.startswith("*"):
            return None
        return f"unpinned package argument {spec!r} (empty version after '==')"
    if spec.startswith("@"):
        match = re.match(r"^@[^/]+/[^@]+@(.+)$", spec)
    else:
        match = re.match(r"^[^@]+@(.+)$", spec)
    version = match.group(1) if match else None
    if version and version != "latest":
        return None
    return f"unpinned package argument {spec!r} (no exact version)"


# ── Rules ─────────────────────────────────────────────────────────────────────


def validate(servers: dict[str, Any]) -> list[Violation]:
    violations: list[Violation] = []

    for name in sorted(servers):
        server = servers[name] or {}
        if not isinstance(server, dict):
            violations.append(Violation(name, "shape", "server entry is not a mapping"))
            continue

        enabled = is_enabled(server)
        stdio = is_stdio(server)

        # Rule 1 — stdio package arguments must be pinned to an exact version.
        if stdio:
            specs = package_specs(server)
            if server.get("command") in ("npx", "uvx") and not specs:
                violations.append(
                    Violation(name, "pin", "no package argument found for npx/uvx command")
                )
            for spec in specs:
                reason = pin_violation(spec)
                if reason:
                    violations.append(Violation(name, "pin", reason))

        # Rule 2 — every server has a connect timeout.
        if not _positive_int(server.get("connect_timeout")):
            violations.append(
                Violation(name, "connect_timeout", "missing or non-positive connect_timeout")
            )

        # Rule 3 — every stdio server has a lifecycle.
        if stdio:
            for key in ("idle_timeout_seconds", "max_lifetime_seconds"):
                if not _positive_int(server.get(key)):
                    violations.append(Violation(name, "lifecycle", f"missing or non-positive {key}"))

        # Rule 4 — every enabled server declares a resources/prompts policy.
        if enabled:
            tools = server.get("tools")
            tools = tools if isinstance(tools, dict) else {}
            for key in ("resources", "prompts"):
                if key not in tools:
                    violations.append(
                        Violation(name, "tools-policy", f"enabled server has no tools.{key} policy")
                    )

        # Rule 5 — every enabled server declares the golden tool surface.
        if enabled:
            tools = server.get("tools")
            tools = tools if isinstance(tools, dict) else {}
            include = tools.get("include")
            if not isinstance(include, list) or not include:
                violations.append(
                    Violation(name, "tools-include", "enabled server has no non-empty tools.include")
                )

    # Rule 6 — parked servers are present and disabled, never silently absent.
    for name in PARKED_SERVERS:
        server = servers.get(name)
        if not isinstance(server, dict):
            violations.append(
                Violation(name, "parked", "parked server missing from manifest (must stay enabled: false)")
            )
        elif server.get("enabled") is not False:
            violations.append(
                Violation(name, "parked", "parked server is not explicitly enabled: false")
            )

    return violations


def _positive_int(value: Any) -> bool:
    if isinstance(value, bool):
        return False
    if isinstance(value, int):
        return value > 0
    if isinstance(value, str):
        try:
            return int(value) > 0
        except ValueError:
            return False
    return False


# ── Entry point ───────────────────────────────────────────────────────────────


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Lint the Hermes MCP manifest.")
    parser.add_argument("values", nargs="?", default=DEFAULT_VALUES, help="values.yaml to lint")
    parser.add_argument("--path", default=DEFAULT_PATH, help="dotted path to the manifest block")
    args = parser.parse_args(argv)

    values = load_values(args.values)
    servers = resolve_path(values, args.path)
    if not isinstance(servers, dict) or not servers:
        sys.stderr.write(f"error: no manifest block found at {args.path!r} in {args.values}\n")
        return 2

    enabled = sum(1 for s in servers.values() if isinstance(s, dict) and is_enabled(s))
    parked = sum(1 for n in PARKED_SERVERS if n in servers)

    print(f"MCP manifest lint: {args.values} :: {args.path}")
    print(f"  servers: {len(servers)} (enabled: {enabled}, parked: {parked})\n")

    violations = validate(servers)
    for violation in violations:
        print(f"  {violation}")

    if violations:
        print(f"\n{len(violations)} violation(s) — MCP manifest is NOT production-safe")
        return 1

    print("OK — manifest is pinned, bounded and fully declared")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
