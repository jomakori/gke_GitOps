#!/usr/bin/env python3
"""
gen-split.py — Reads CUE's exported files map and writes individual YAML files.

Usage:
    cd <repo-root>
    python3 scripts/gen-split.py            # generate files
    python3 scripts/gen-split.py --check    # verify generated files match (CI)

Requires:
    - cue binary available via the CUE environment or ~/.local/bin/cue
    - Run from the repository root (parent of cue/ and gen/)
"""

import json
import os
import subprocess
import sys
from pathlib import Path


def get_cue_binary() -> str:
    candidates = [
        "cue",
        os.path.expanduser("~/.local/bin/cue"),
    ]
    for c in candidates:
        try:
            subprocess.run([c, "version"], capture_output=True, check=True)
            return c
        except (FileNotFoundError, subprocess.CalledProcessError):
            continue
    print("ERROR: CUE binary not found. Install from https://cuelang.org/", file=sys.stderr)
    sys.exit(1)


def export_files(cue_bin: str, repo_root: Path) -> dict[str, str]:
    result = subprocess.run(
        [cue_bin, "export", "./cue", "--out", "json", "-e", "files"],
        capture_output=True,
        text=True,
        check=True,
        cwd=repo_root,
    )
    return json.loads(result.stdout)


def write_files(files: dict[str, str], output_root: Path) -> list[Path]:
    written: list[Path] = []
    for rel_path, content in files.items():
        abs_path = output_root / rel_path
        abs_path.parent.mkdir(parents=True, exist_ok=True)
        abs_path.write_text(content)
        written.append(abs_path)
    return written


def check_files(files: dict[str, str], existing_root: Path) -> bool:
    all_match = True
    for rel_path, content in files.items():
        existing = existing_root / rel_path
        if not existing.exists():
            print(f"MISSING: {rel_path}", file=sys.stderr)
            all_match = False
        elif existing.read_text() != content:
            print(f"DIFFERS: {rel_path}", file=sys.stderr)
            all_match = False
    return all_match


def main() -> None:
    repo_root = Path(__file__).resolve().parent.parent
    check_mode = "--check" in sys.argv

    cue_bin = get_cue_binary()

    print(f"Exporting from {repo_root}/cue ...")
    files = export_files(cue_bin, repo_root)

    if check_mode:
        print(f"Checking {len(files)} files against {repo_root}/ ...")
        ok = check_files(files, repo_root)
        if ok:
            print("OK — generated output matches.")
            sys.exit(0)
        else:
            print("ERROR — generated output differs. Run 'make gen' to update.", file=sys.stderr)
            sys.exit(1)

    print(f"Writing {len(files)} files to {repo_root}/ ...")
    written = write_files(files, repo_root)

    print("Done. Generated files:")
    for path in written:
        print(f"  {path.relative_to(repo_root)}")


if __name__ == "__main__":
    main()
