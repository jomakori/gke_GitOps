#!/usr/bin/env python3
"""check_workload_policy.py — catch the workload classes nothing else in CI sees.

Why this exists
---------------
Every other gate in this repo validates YAML shape: does it parse, does it match a
schema, do the selectors reference labels that exist. None of them care whether a
container can actually be scheduled with resources, be probed, or survive a node
drain. A chart shipping a container with no requests/limits, no probes and no PDB
renders byte-perfect and passes every gate. This script checks those three things.

What it checks (on RENDERED output, never on values.yaml)
---------------------------------------------------------
1. RESOURCES — every container carries `requests.cpu`, `requests.memory` and
   `limits.memory`. Without requests the pod is BestEffort, which on a contended
   node means the kernel picks it as an OOM victim first; without a memory limit a
   single leak can take the node down with it. CPU *limits* are deliberately not
   required — CPU is compressible and limits cause throttling.
2. PROBES — every container in a long-running workload (Deployment, StatefulSet,
   DaemonSet) declares a readinessProbe and a livenessProbe. One-shot containers
   (Job, CronJob, bare Pod) are exempt: there is nothing to be ready for.
3. PDB — every Deployment/StatefulSet with more than one replica has a matching
   PodDisruptionBudget in the same namespace. A single-replica workload needs no
   PDB (an eviction cannot be made safe by one), so those are skipped rather than
   reported.

Exemptions
----------
Annotate the workload itself, which keeps the reason next to the thing it excuses:

    gitops.maklab/policy-exempt: "the sidecar is a one-shot flusher"

Exempted workloads are skipped with a note, so an exemption is visible in the log
rather than silent.

Usage
-----
    check_workload_policy.py [--strict] <rendered.yaml|directory>...

Default is advisory: findings print as ::warning:: and the exit code stays 0, so
this can land on a repo that is not clean yet. `--strict` prints ::error:: and
exits 1 — flip CI to that once the counts below are zero.

Exit codes: 0 clean (or advisory findings), 1 findings under --strict, 2 bad usage.
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys

LONG_RUNNING_KINDS = {"Deployment", "StatefulSet", "DaemonSet"}
POD_KINDS = LONG_RUNNING_KINDS | {"Job", "CronJob", "Pod", "ReplicaSet", "ReplicationController"}
EXEMPT_ANNOTATION = "gitops.maklab/policy-exempt"


def find_yq() -> str:
    for candidate in ("/opt/data/bin/yq", "/usr/local/bin/yq", "/usr/bin/yq"):
        if os.path.isfile(candidate) and os.access(candidate, os.X_OK):
            return candidate
    found = shutil.which("yq")
    if found:
        return found
    sys.stderr.write("::error::yq not found (install mikefarah/yq); cannot parse rendered manifests\n")
    sys.exit(2)


def load_docs(yq: str, path: str) -> list[dict]:
    """yq converts a multi-doc stream into one JSON array — no PyYAML needed."""
    out = subprocess.run(
        [yq, "eval-all", "-o=json", "-I=0", "[.]", path],
        capture_output=True, text=True,
    )
    if out.returncode != 0:
        sys.stderr.write(f"::error::yq failed on {path}: {out.stderr.strip()[:300]}\n")
        return []
    try:
        docs = json.loads(out.stdout)
    except json.JSONDecodeError as exc:
        sys.stderr.write(f"::error::could not parse {path}: {exc}\n")
        return []
    return [d for d in docs if isinstance(d, dict) and d.get("kind")]


def pod_spec(doc: dict) -> dict | None:
    kind = doc.get("kind")
    if kind in {"Deployment", "StatefulSet", "DaemonSet", "Job", "ReplicaSet", "ReplicationController", "Pod"}:
        template = doc.get("spec", {}).get("template") or (doc.get("spec") if kind == "Pod" else None)
    elif kind == "CronJob":
        template = doc.get("spec", {}).get("jobTemplate", {}).get("spec", {}).get("template")
    else:
        return None
    if not isinstance(template, dict):
        return None
    return template.get("spec") or None


def pod_labels(doc: dict) -> dict:
    """Pod template labels — the thing a PDB selector has to match."""
    kind = doc.get("kind")
    if kind == "CronJob":
        template = doc.get("spec", {}).get("jobTemplate", {}).get("spec", {}).get("template")
    elif kind == "Pod":
        template = doc
    else:
        template = doc.get("spec", {}).get("template")
    if not isinstance(template, dict):
        return {}
    return (template.get("metadata") or {}).get("labels") or {}


def all_containers(spec: dict) -> list[tuple[str, dict]]:
    """Regular containers and init containers both consume node resources."""
    out = []
    for field in ("containers", "initContainers"):
        for c in spec.get(field) or []:
            if isinstance(c, dict):
                out.append((field, c))
    return out


def check_resources(where: str, field: str, container: dict) -> list[str]:
    name = container.get("name", "<unnamed>")
    res = container.get("resources") or {}
    req = res.get("requests") or {}
    lim = res.get("limits") or {}
    label = f"{where} {field[:-1]} '{name}'"
    gaps = []
    if not req.get("cpu"):
        gaps.append(f"{label} has no resources.requests.cpu")
    if not req.get("memory"):
        gaps.append(f"{label} has no resources.requests.memory (BestEffort — first OOM victim on a contended node)")
    if not lim.get("memory"):
        gaps.append(f"{label} has no resources.limits.memory")
    return gaps


def check_probes(where: str, container: dict) -> list[str]:
    name = container.get("name", "<unnamed>")
    label = f"{where} container '{name}'"
    gaps = []
    if not container.get("readinessProbe"):
        gaps.append(f"{label} has no readinessProbe")
    if not container.get("livenessProbe"):
        gaps.append(f"{label} has no livenessProbe")
    return gaps


def selector_subset(workload_labels: dict, pdb_selector_labels: dict) -> bool:
    return all(workload_labels.get(k) == v for k, v in (pdb_selector_labels or {}).items()) and bool(pdb_selector_labels)


def analyse(docs: list[dict], origin: str) -> tuple[list[str], list[str], dict]:
    findings: list[str] = []
    notes: list[str] = []
    stats = {"workloads": 0, "containers": 0, "resources": 0, "probes": 0, "pdb": 0, "exempt": 0}

    pdbs = [d for d in docs if d.get("kind") == "PodDisruptionBudget"]
    hpas = [d for d in docs if d.get("kind") == "HorizontalPodAutoscaler"]

    def hpa_targets(workload_kind: str, name: str, ns: str) -> bool:
        """An HPA raises replicas at runtime, so a chart declaring replicas: 1 can still
        be running many — the PDB question is just as real for those."""
        for h in hpas:
            ref = (h.get("spec") or {}).get("scaleTargetRef") or {}
            if (
                ref.get("kind") == workload_kind
                and ref.get("name") == name
                and (h.get("metadata") or {}).get("namespace", "default") == ns
            ):
                return True
        return False

    for doc in docs:
        kind = doc.get("kind")
        if kind not in POD_KINDS:
            continue
        spec = pod_spec(doc)
        if spec is None:
            continue
        meta = doc.get("metadata") or {}
        where = f"{origin} {kind} {meta.get('namespace', 'default')}/{meta.get('name', '?')}"
        stats["workloads"] += 1

        annotations = meta.get("annotations") or {}
        if EXEMPT_ANNOTATION in annotations:
            stats["exempt"] += 1
            notes.append(f"exempt: {where} — {annotations[EXEMPT_ANNOTATION]}")
            continue

        containers = all_containers(spec)
        stats["containers"] += len(containers)
        for field, container in containers:
            r = check_resources(where, field, container)
            stats["resources"] += len(r)
            findings.extend(r)
            if kind in LONG_RUNNING_KINDS and field == "containers":
                p = check_probes(where, container)
                stats["probes"] += len(p)
                findings.extend(p)

        if kind in LONG_RUNNING_KINDS:
            name = meta.get("name", "?")
            ns = meta.get("namespace", "default")
            replicas = doc.get("spec", {}).get("replicas")
            scaled = isinstance(replicas, int) and replicas > 1
            autoscaled = hpa_targets(kind, name, ns)
            if scaled or autoscaled:
                labels = pod_labels(doc)
                matched = any(
                    (p.get("metadata") or {}).get("namespace", "default") == ns
                    and selector_subset(labels, ((p.get("spec") or {}).get("selector") or {}).get("matchLabels") or {})
                    for p in pdbs
                )
                if not matched:
                    why = f"replicas={replicas}" if scaled else f"scaled by an HPA (replicas={replicas} in git)"
                    stats["pdb"] += 1
                    findings.append(
                        f"{where} runs more than one pod ({why}) but no PodDisruptionBudget in "
                        f"namespace '{ns}' selects its pod labels"
                    )
    return findings, notes, stats


def main() -> int:
    ap = argparse.ArgumentParser(description="Workload policy gate (resources / probes / PDB).")
    ap.add_argument("--strict", action="store_true", help="exit 1 on findings (default: advisory)")
    ap.add_argument("paths", nargs="+", help="rendered manifest files or directories of them")
    args = ap.parse_args()

    yq = find_yq()

    files: list[str] = []
    for p in args.paths:
        if os.path.isdir(p):
            for root, _, names in os.walk(p):
                files.extend(os.path.join(root, n) for n in sorted(names) if n.endswith((".yaml", ".yml")))
        elif os.path.isfile(p):
            files.append(p)
    if not files:
        sys.stderr.write("::error::no rendered manifests found to check\n")
        return 2

    all_findings: list[str] = []
    totals = {"workloads": 0, "containers": 0, "resources": 0, "probes": 0, "pdb": 0, "exempt": 0}
    for path in files:
        docs = load_docs(yq, path)
        if not docs:
            continue
        findings, notes, stats = analyse(docs, os.path.basename(path))
        all_findings.extend(findings)
        for k, v in stats.items():
            totals[k] += v
        for note in notes:
            print(f"::notice::{note}")

    level = "error" if args.strict else "warning"
    for f in all_findings:
        print(f"::{level}::{f}")

    print(
        f"workload policy: {totals['workloads']} workloads / {totals['containers']} containers checked — "
        f"{totals['resources']} resource gaps, {totals['probes']} probe gaps, {totals['pdb']} missing PDBs, "
        f"{totals['exempt']} exempt"
    )

    if all_findings and args.strict:
        print(f"::error::workload policy gate failed ({len(all_findings)} findings; run without --strict for the advisory view)")
        return 1
    if all_findings:
        print("::warning::workload policy gate is ADVISORY here — the gaps above do not fail the build")
    return 0


if __name__ == "__main__":
    sys.exit(main())
