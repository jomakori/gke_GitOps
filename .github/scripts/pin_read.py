#!/usr/bin/env python3
"""Print the openkite production image tag pinned in the umbrella chart's values."""
import sys

path = sys.argv[1]
lines = open(path).read().splitlines(keepends=True)
indent = lambda line: len(line) - len(line.lstrip())  # noqa: E731

start = next(k for k, line in enumerate(lines) if line.rstrip("\n") == "openkite:")
for k in range(start + 1, len(lines)):
    line = lines[k]
    if line.strip() and indent(line) == 0:
        break
    if line.rstrip("\n").rstrip() == "    production:":
        for j in range(k + 1, len(lines)):
            candidate = lines[j]
            if candidate.strip() and indent(candidate) <= 4:
                break
            if candidate.strip().startswith("tag:"):
                sys.stdout.write(candidate.split("tag:", 1)[1].strip())
                sys.exit(0)
sys.exit("no openkite production tag found")
