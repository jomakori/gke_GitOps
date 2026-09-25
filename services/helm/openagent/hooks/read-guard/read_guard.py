#!/usr/bin/env python3
"""``pre_tool_call`` guard: refuse an unbounded read of a large file.

Hermes writes the tool call to a shell hook as JSON on stdin. A ``read_file``
with no ``limit`` (or one above the cap) on a large file is what puts ~90 KB
into the context and re-bills it on every later hop — including hops whose route
has no prefix cache. The guard allows bounded reads and small files, and answers
an unbounded large read with the delegation directive instead of the payload.

``action: block`` is the hook contract's own channel (exit 2 works too); the
process still exits 0 so a parse problem can never look like a deliberate block.
Thresholds are env-overridable so the boundary can be tested without a big file.
"""

from __future__ import annotations

import json
import os
import sys

MIN_BYTES = int(os.environ.get("READ_GUARD_MIN_BYTES", "65536"))
MAX_LINES = int(os.environ.get("READ_GUARD_MAX_LINES", "500"))

_DIRECTIVE = (
    "read_file: {path} is {kb:.0f} KB and this call sets no bound (limit above {cap} lines). "
    "Read it in windows (`offset` + `limit` <= {cap}), or hand the survey to a subagent "
    "(omo / delegate_task) and ask for a bounded summary with file:line citations. An unbound "
    "read this size rides every later hop, including the routes that bill it at full rate."
)


def _bounded(args: dict) -> bool:
    limit = args.get("limit")
    if limit is None:
        return False
    try:
        return 0 < int(limit) <= MAX_LINES
    except (TypeError, ValueError):
        return False


def main() -> int:
    try:
        payload = json.load(sys.stdin)
    except Exception:
        return 0
    if not isinstance(payload, dict):
        return 0
    if payload.get("hook_event_name") != "pre_tool_call" or payload.get("tool_name") != "read_file":
        return 0
    args = payload.get("tool_input")
    if not isinstance(args, dict) or _bounded(args):
        return 0
    path = args.get("path") or args.get("file_path") or ""
    if not isinstance(path, str) or not path:
        return 0
    try:
        size = os.path.getsize(os.path.expanduser(path))
    except OSError:
        return 0  # missing, unreadable, a directory or a glob: not this guard's call
    if size < MIN_BYTES:
        return 0
    print(json.dumps({"action": "block", "message": _DIRECTIVE.format(
        path=path, kb=size / 1024, cap=MAX_LINES)}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
