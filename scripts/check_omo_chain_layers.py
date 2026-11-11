#!/usr/bin/env python3
"""OMO fallback chains: consecutive hops must sit in DIFFERENT provider layers.

A hop is only a fallback when it can fail independently of the hop before it.
Two aliases served by the same key/quota/proxy are one failure domain: a Claude
Max subscription outage kills `claude-sonnet-5` and `claude-haiku-4-5` together,
and the Go plan's weekly cap 429s every Go model at once, so
`sonnet -> haiku` or `go -> mimo` buy a round-trip and nothing else.

This gate reads `hermes-agent.config.plugins.entries.omo.settings` in the
openagent values file, maps every alias in `chains` / `categories` to a provider
layer, and fails on any pair of consecutive hops that land in the same layer.

Deliberately dependency-free (stdlib only, no PyYAML): a gate that needs its own
bootstrap gets skipped rather than fixed. It reads the YAML by indentation the
same way `.useful-scripts/check_values_reachability.py` does.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

# ---------------------------------------------------------------------------
# LAYER TABLE — the single place an alias is mapped to its provider layer.
# Evaluated top to bottom; the first match wins. `litellm/` is stripped first.
# Aliases with no match get their own layer named after the alias, which keeps
# a genuinely unknown provider distinct instead of silently collapsing it.
# ---------------------------------------------------------------------------
LAYER_RULES: tuple[tuple[str, str, str], ...] = (
    ("suffix", "-direct", "deepseek-direct"),
    ("suffix", "-go", "opencode-go"),
    ("prefix", "claude-", "claude-proxy"),
    ("prefix", "mimo-", "opencode-go"),
    ("prefix", "kimi-", "kilo"),
    ("prefix", "bytedance-", "kilo"),
    ("prefix", "byteplus-coding/", "kilo"),
    ("prefix", "glm-", "kilo"),
    ("prefix", "minimax-", "kilo"),
    ("prefix", "qwen", "kilo"),
    ("prefix", "gemini-", "kilo"),
    ("prefix", "gpt-", "kilo"),
)

# The collections under `omo.settings` that carry fallback ladders.
GUARDED_COLLECTIONS = ("chains", "categories")

DEFAULT_VALUES = "services/helm/openagent/values.yaml"


def layer_of(alias: str) -> str:
    """Provider layer for a chain alias (``litellm/`` prefix stripped)."""
    name = alias.strip()
    if name.startswith("litellm/"):
        name = name[len("litellm/") :]
    for kind, pattern, layer in LAYER_RULES:
        if kind == "suffix" and name.endswith(pattern):
            return layer
        if kind == "prefix" and name.startswith(pattern):
            return layer
    return name


def _indent(line: str) -> int:
    return len(line) - len(line.lstrip(" "))


def read_ladders(text: str) -> list[tuple[str, str, list[str]]]:
    """Return [(collection, name, [aliases...]), ...] in file order.

    Indentation-driven: a block scalar or comment is ignored, a `key:` line
    pushes a frame, and a `- item` line belongs to the nearest enclosing key
    frame (the chain name) inside its collection frame (`chains`/`categories`).
    Only the `omo.settings` subtree is collected.
    """
    ladders: list[tuple[str, str, list[str]]] = []
    frames: list[tuple[int, str]] = []  # (indent, key)
    current: list[str] | None = None

    for raw in text.splitlines():
        stripped = raw.strip()
        if not stripped or stripped.startswith("#"):
            continue
        indent = _indent(raw)
        while frames and frames[-1][0] >= indent:
            frames.pop()

        if stripped.startswith("- "):
            if len(frames) < 2:
                continue
            collection, name = frames[-2][1], frames[-1][1]
            ancestors = [key for _, key in frames[:-2]]
            if collection not in GUARDED_COLLECTIONS or "omo" not in ancestors:
                current = None
                continue
            item = stripped[2:].strip().strip("\"'")
            if current is None:
                current = []
                ladders.append((collection, name, current))
            current.append(item)
            continue

        current = None
        key = stripped.split(":", 1)[0].strip().strip("\"'")
        if stripped.endswith(":"):
            frames.append((indent, key))

    return ladders


def find_violations(
    ladders: list[tuple[str, str, list[str]]],
) -> list[tuple[str, str, str, str, str]]:
    """Return [(collection.name, hop_a, layer_a, hop_b, layer_b), ...]."""
    violations = []
    for collection, name, aliases in ladders:
        for hop_a, hop_b in zip(aliases, aliases[1:]):
            layer_a, layer_b = layer_of(hop_a), layer_of(hop_b)
            if layer_a == layer_b:
                violations.append((f"{collection}.{name}", hop_a, layer_a, hop_b, layer_b))
    return violations


def main(argv: list[str] | None = None) -> int:
    repo_root = Path(__file__).resolve().parents[1]
    parser = argparse.ArgumentParser(
        description="Fail when consecutive OMO chain hops share a provider layer."
    )
    parser.add_argument(
        "--values",
        default=str(repo_root / DEFAULT_VALUES),
        help=f"openagent values file (default: {DEFAULT_VALUES})",
    )
    args = parser.parse_args(argv)

    path = Path(args.values)
    try:
        text = path.read_text(encoding="utf-8")
    except OSError as exc:
        print(f"FAIL: cannot read {path}: {exc}", file=sys.stderr)
        return 2

    ladders = read_ladders(text)
    if not ladders:
        # A silent pass here is the dangerous outcome: the block moved or was
        # renamed and the guard is inspecting nothing.
        print(
            "FAIL: no omo settings chains/categories found in "
            f"{path} — the guard is inspecting nothing",
            file=sys.stderr,
        )
        return 2

    violations = find_violations(ladders)
    if violations:
        for chain, hop_a, layer_a, hop_b, layer_b in violations:
            print(
                f"FAIL {chain}: {hop_a} -> {hop_b} "
                f"({layer_a} -> {layer_b}, same provider layer)",
                file=sys.stderr,
            )
        print(
            f"{len(violations)} consecutive same-layer hop pair(s) across "
            f"{len(ladders)} chain(s) in {path}",
            file=sys.stderr,
        )
        return 1

    hops = sum(len(aliases) for _, _, aliases in ladders)
    print(
        f"OK: {len(ladders)} chains, {hops} hops — every consecutive hop sits "
        "in a distinct provider layer"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
