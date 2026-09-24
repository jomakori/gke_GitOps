#!/usr/bin/env python3
"""Fail when a YAML file defines the same mapping key twice in the same block.

Why this exists: appending a values block by hand instead of editing the existing
one produced this values.yaml:

    litellm:            # line 34, 333 lines of real config (models, db, migrationJob)
    ...
    litellm:            # line 1443, 3 lines, added later by hand
      podDisruptionBudget: ...

YAML keeps the LAST mapping key and silently discards the first, so the entire
litellm config stopped reaching the subchart. Every other gate stayed green: the
chart rendered, helm lint passed, kubeconform passed, CI was happy. The only
visible symptom was a live object diffing against its own previous revision.

Stdlib only, no PyYAML -- a gate that needs its own bootstrap gets skipped.
"""

import os
import re
import sys

KEY = re.compile(r"^([A-Za-z_][A-Za-z0-9_.\-]*):(?:\s|$)")
BLOCK_SCALAR = re.compile(r":\s*[|>][-+]?\s*$")

DEFAULT_GLOBS = (
    "services/helm",
    "apps",
)


def find_files(roots, explicit):
    if explicit:
        return [p for p in explicit if os.path.isfile(p)]
    out = []
    for root in roots:
        if not os.path.isdir(root):
            continue
        for base, dirs, names in os.walk(root):
            # charts/ holds pulled subcharts we do not own; templates/ is Go
            # templates, where a repeated key inside {{ range }} is legitimate
            dirs[:] = [d for d in dirs if d not in (".git", "charts", "templates")]
            for n in names:
                if n.endswith((".yaml", ".yml")):
                    out.append(os.path.join(base, n))
    return sorted(out)


def duplicates(path):
    """Return [(line_no, key, first_line_no)] for repeated keys in one block."""
    findings, seen, block_indent = [], {}, None
    with open(path, encoding="utf-8", errors="replace") as fh:
        for i, raw in enumerate(fh, 1):
            line = raw.rstrip("\n")
            if not line.strip() or line.lstrip().startswith("#"):
                continue
            indent = len(line) - len(line.lstrip(" "))
            if block_indent is not None:
                if indent > block_indent:
                    continue
                block_indent = None
            stripped = line.strip()
            # document separator: everything recorded so far belongs to a
            # different document, so repeated keys across documents are fine
            if stripped in ("---", "..."):
                seen.clear()
                continue
            # list item: starts a new mapping element, so keys recorded for the
            # previous element at this indent or deeper are not duplicates
            if stripped.startswith("-"):
                for slot in [s for s in seen if s[0] >= indent]:
                    del seen[slot]
                stripped = stripped.lstrip("-").lstrip()
            m = KEY.match(stripped)
            if not m:
                continue
            if BLOCK_SCALAR.search(line):
                block_indent = indent
            # a new key at this indent invalidates everything recorded deeper
            for slot in [s for s in seen if s[0] > indent]:
                del seen[slot]
            key = m.group(1)
            slot = (indent, key)
            if slot in seen:
                findings.append((i, key, seen[slot]))
            else:
                seen[slot] = i
    return findings


def main(argv):
    explicit = [a for a in argv if not a.startswith("-")]
    roots = [a for a in argv if a.startswith("--root=")]
    roots = [r.split("=", 1)[1] for r in roots] or list(DEFAULT_GLOBS)
    files = find_files(roots, explicit)
    bad = 0
    for path in files:
        for line_no, key, first_line in duplicates(path):
            print(
                f"DUPLICATE KEY {path}:{line_no}: '{key}' already defined at line "
                f"{first_line} - YAML keeps the last one, the earlier block is "
                f"silently discarded",
                file=sys.stderr,
            )
            bad += 1
    if bad:
        print(f"\n{bad} duplicate mapping key(s) across {len(files)} file(s).", file=sys.stderr)
        return 1
    print(f"duplicate-key check: clean ({len(files)} YAML files)")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
