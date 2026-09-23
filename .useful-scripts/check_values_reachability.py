#!/usr/bin/env python3
"""check_values_reachability.py — fail when a chart declares values that can never
reach anything.

Why this exists: a top-level key in a chart's values.yaml is only delivered to a
subchart when it matches that dependency's `name` (or its `alias`) in Chart.yaml.
Anything else is either read by one of the chart's own templates, or it is dead
config that renders as valid YAML and passes `helm lint`, `helm template` and
`kubeconform` without complaint.

The concrete incident this gate was written for: services/helm/postgres-operator
carried ~65 lines under `stackgresOperator:` while its dependency is named
`stackgres-operator` with no alias. Only the `condition:` read that spelling, so
`authentication.createAdminSecret`, `extensions.cache.enabled` and
`grafana.autoEmbed` silently never applied - visible only by diffing the live
SGConfig against upstream defaults. A reviewer reading values.yaml saw correct,
wired-looking config.

Deliberately dependency-free (no PyYAML): it runs from pre-commit with the system
python, and a gate that needs its own bootstrap gets skipped instead of fixed.
Only top-level keys, dependency names and template references are needed, all of
which are indent-anchored and unambiguous.

Usage:
  check_values_reachability.py [chart-dir ...]      # default: every chart in the repo
Exit 0 = every declared value reaches something. Exit 1 = at least one dead key.
"""
from __future__ import annotations

import glob
import os
import re
import sys

# Keys Helm/sprig or a subchart may consume without a template mentioning them.
ALLOWED_ALWAYS = {"global", "nameOverride", "fullnameOverride"}

VALUES_REF = re.compile(r"\.Values\.([A-Za-z_][A-Za-z0-9_]*)")
# A top-level mapping key: column 0, key-ish name, colon.
TOP_LEVEL_KEY = re.compile(r"^([A-Za-z_][A-Za-z0-9_.-]*):")
DEP_FIELD = re.compile(r"^(name|alias|condition):\s*(.+?)\s*$")


def chart_dirs(argv: list[str]) -> list[str]:
    if argv:
        return argv
    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    found = []
    for path in glob.glob(os.path.join(root, "**", "Chart.yaml"), recursive=True):
        if "/.git/" in path or "/charts/" in path:
            continue
        found.append(os.path.dirname(path))
    return sorted(found)


def template_refs(chart_dir: str) -> set[str]:
    refs: set[str] = set()
    for dirpath, _dirnames, filenames in os.walk(os.path.join(chart_dir, "templates")):
        for name in filenames:
            if not name.endswith((".yaml", ".yml", ".tpl")):
                continue
            with open(os.path.join(dirpath, name), encoding="utf-8", errors="replace") as fh:
                refs.update(VALUES_REF.findall(fh.read()))
    return refs


def parse_dependencies(chart_text: str) -> list[dict[str, str]]:
    """Read the `dependencies:` list from Chart.yaml (name/alias/condition only)."""
    deps: list[dict[str, str]] = []
    in_deps = False
    current: dict[str, str] = {}
    for raw in chart_text.splitlines():
        if re.match(r"^dependencies:\s*(#.*)?$", raw):
            in_deps = True
            continue
        if not in_deps:
            continue
        # A column-0 line ends the block (nested `- name:` entries are indented).
        if raw and not raw[0].isspace():
            break
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("- "):
            if current:
                deps.append(current)
            current = {}
            line = line[2:].strip()
        match = DEP_FIELD.match(line)
        if match:
            current[match.group(1)] = match.group(2).strip().strip("\"'")
    if current:
        deps.append(current)
    return deps


def top_level_keys(values_text: str) -> list[str]:
    keys = []
    for raw in values_text.splitlines():
        if not raw or raw[0] in "# \t":
            continue
        match = TOP_LEVEL_KEY.match(raw)
        if match:
            keys.append(match.group(1))
    return keys


def check(chart_dir: str) -> list[str]:
    problems: list[str] = []
    chart_file = os.path.join(chart_dir, "Chart.yaml")
    values_file = os.path.join(chart_dir, "values.yaml")
    if not os.path.isfile(chart_file) or not os.path.isfile(values_file):
        return problems
    with open(chart_file, encoding="utf-8") as fh:
        deps = parse_dependencies(fh.read())
    if not deps:
        return problems  # nothing to route

    routable: set[str] = {
        dep["alias"] if dep.get("alias") else dep["name"]
        for dep in deps
        if dep.get("alias") or dep.get("name")
    }
    cond_roots = {dep["condition"].split(".")[0] for dep in deps if dep.get("condition")}
    with open(values_file, encoding="utf-8") as fh:
        keys = top_level_keys(fh.read())
    refs = template_refs(chart_dir)

    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    rel = os.path.relpath(chart_dir, root)
    for key in keys:
        if key in routable or key in refs or key in ALLOWED_ALWAYS:
            continue
        if key in cond_roots:
            problems.append(
                f"{rel}: values.yaml top-level key '{key}' is used as a `condition:` root "
                f"but is not a dependency name/alias ({sorted(routable)}) - the subchart "
                f"never receives it. Nest these values under one of {sorted(routable)} "
                f"and point `condition:` at the same key."
            )
            continue
        problems.append(
            f"{rel}: values.yaml top-level key '{key}' reaches nothing - it is not a "
            f"dependency name/alias ({sorted(routable)}) and no template references "
            f"`.Values.{key}`. Values here are dead config."
        )
    return problems


def main() -> int:
    problems: list[str] = []
    for chart_dir in chart_dirs(sys.argv[1:]):
        problems.extend(check(chart_dir))
    if problems:
        for line in problems:
            print(f"::error::{line}")
        print(f"\n{len(problems)} unreachable values key(s) found.")
        return 1
    print("All declared chart values reach a dependency or a template.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
