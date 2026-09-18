#!/usr/bin/env python3
"""Render the mechanically-derived blocks of the root README.

Only three blocks are generated, each wrapped in sentinels:

    <!-- BEGIN GENERATED: <name> --> ... <!-- END GENERATED: <name> -->

Everything outside the sentinels is hand-written prose and is never touched.
Blocks are derived from repo state: the workflow files, and an allowlist for
the layout tree. Anything requiring judgement (wave tiers, taxonomy, prose)
stays hand-written on purpose.

Usage:
    .useful-scripts/render_readme_blocks.py           # rewrite the blocks
    .useful-scripts/render_readme_blocks.py --check   # fail if they are stale
"""
from __future__ import annotations

import argparse
import pathlib
import re
import sys

import yaml

REPO = pathlib.Path(__file__).resolve().parents[1]
README = REPO / "README.md"
WORKFLOWS = REPO / ".github" / "workflows"
OWNER, NAME = "jomakori", "gke_GitOps"

# Stack identity + palette: intent, not state, so it lives here by hand.
STACK_BADGES = [
    ("Kubernetes", "4169E1", "kubernetes", "https://kubernetes.io"),
    ("Helm", "5D4ED3", "helm", "https://helm.sh"),
    ("Argo%20CD", "7934C5", "argo", "https://argo-cd.readthedocs.io"),
    ("Terraform", "8A2BE2", "terraform", "https://developer.hashicorp.com/terraform"),
]

# One-line summaries: a workflow's own `name:` is too terse to be the whole story.
CI_SUMMARY = {
    "helm_lint-test.yaml": "yamllint, chart lint/dry-run, render + schema validation, chart unit tests, chart policies, selector guard",
    "image-builds.yaml": "builds and publishes the images under `images/` when their sources change",
}

# Layout tree: allowlisted top-level entries; curation and notes are intent.
# A naive full listing would drag in tooling files that do not orient a reader.
TREE = [
    ("services/", "third-party software we host (charts + the registry)"),
    ("apps/", "first-party workloads we build or test (charts + the registry)"),
    ("images/", "images we build for the cluster"),
    (".useful-scripts/", "validation, rendering and cluster helpers"),
    (".github/workflows/", "CI: chart lint/test, image builds"),
    ("ct_check.sh", "chart lint/dry-run entrypoint"),
    ("renovate.json", "dependency updates"),
    (".pre-commit-config.yaml", "local hooks CI also runs"),
]
TREE_WIDTH = 21


def sentinel(name: str, body: str) -> str:
    return f"<!-- BEGIN GENERATED: {name} -->\n{body}\n<!-- END GENERATED: {name} -->"


def workflows() -> list[pathlib.Path]:
    return sorted(p for p in WORKFLOWS.glob("*.y*ml") if p.is_file())


def render_badges() -> str:
    live = []
    for wf in workflows():
        doc = yaml.safe_load(wf.read_text()) or {}
        label = doc.get("name", wf.name)
        url = f"https://github.com/{OWNER}/{NAME}/actions/workflows/{wf.name}"
        live.append(f"[![{label}]({url}/badge.svg?branch=main)]({url})")
    stack = [
        f"[![{label}](https://img.shields.io/badge/{label}-{colour}?style=for-the-badge&logo={logo}&logoColor=white)]({url})"
        for label, colour, logo, url in STACK_BADGES
    ]
    return "\n".join(live + stack)


def render_ci() -> str:
    rows = ["| Workflow | What it checks |", "|---|---|"]
    for wf in workflows():
        doc = yaml.safe_load(wf.read_text()) or {}
        jobs = ", ".join((doc.get("jobs") or {}).keys())
        rows.append(f"| `{wf.name}` | {CI_SUMMARY.get(wf.name, f'jobs: {jobs}')} |")
    return "\n".join(rows)


def render_tree() -> str:
    lines = ["```text", "."]
    for i, (path, note) in enumerate(TREE):
        if not (REPO / path.rstrip("/")).exists():
            print(f"warning: allowlisted path missing: {path}", file=sys.stderr)
        branch = "└──" if i == len(TREE) - 1 else "├──"
        entry = f"{branch} {path}"
        lines.append((entry + " " * max(1, TREE_WIDTH - len(entry)) + f"← {note}").rstrip())
    lines.append("```")
    return "\n".join(lines)


BLOCKS = {"badges": render_badges, "ci": render_ci, "tree": render_tree}


def apply_blocks(text: str) -> str:
    for name, render in BLOCKS.items():
        pattern = re.compile(rf"<!-- BEGIN GENERATED: {name} -->.*?<!-- END GENERATED: {name} -->", re.S)
        if not pattern.search(text):
            sys.exit(f"error: README is missing the '{name}' sentinel block")
        text = pattern.sub(lambda _m, b=sentinel(name, render()): b, text, count=1)
    return text


def main() -> int:
    parser = argparse.ArgumentParser(description="Render the generated README blocks.")
    parser.add_argument("--check", action="store_true", help="exit non-zero when the blocks are stale")
    args = parser.parse_args()

    current = README.read_text()
    updated = apply_blocks(current)
    if current == updated:
        print("README generated blocks are up to date")
        return 0
    if args.check:
        print("error: README generated blocks are stale — run .useful-scripts/render_readme_blocks.py")
        return 1
    README.write_text(updated)
    print("README generated blocks updated")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
