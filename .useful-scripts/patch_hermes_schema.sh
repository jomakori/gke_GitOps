#!/usr/bin/env bash
# patch_hermes_schema.sh — neutralise the hermes-agent upstream schema blocker.
#
# jyje/hermes-agent-helm ships values.schema.json with root
# `additionalProperties: false` and NO `global` property. Helm always injects
# `global: {}` into subchart values during parent-value coalescing, so any
# schema-validating tool (helm lint, ct lint) fails on this chart
# unconditionally — no values.yaml can fix it (verified still broken at chart
# 1.15.0, 13 releases after 0.9.1). This script patches the LOCAL copy of the
# dependency: it extracts the fetched tgz, adds a permissive `global` property
# to the root schema, and repacks the SAME tarball. `helm dependency
# update/build` re-fetches the pristine tgz on every run, so this must be
# invoked after every dependency update — call sites: ct_check.sh,
# helm_render_and_kubeconform.sh (pre-commit), .github/workflows/helm_lint-test.yaml.
#
# Idempotent: re-runs exit 0 without repacking once `global` is present.
# Version-agnostic: matches hermes-agent-*.tgz, so Renovate bumps keep working.
set -euo pipefail

CHART_DIR="${1:-services/helm/openagent}"
CHART_DIR="${CHART_DIR%/}"

shopt -s nullglob
tgz=( "${CHART_DIR}"/charts/hermes-agent-*.tgz )
if [[ ${#tgz[@]} -eq 0 ]]; then
    echo "patch-hermes-schema: no hermes-agent dependency in ${CHART_DIR}/charts (run 'helm dependency update' first, or chart has no such dep); skipping."
    exit 0
fi
TARBALL="${tgz[0]}"

tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

tar -xzf "${TARBALL}" -C "${tmp}"
schema="${tmp}/hermes-agent/values.schema.json"
if [[ ! -f "${schema}" ]]; then
    echo "patch-hermes-schema: no values.schema.json inside ${TARBALL}; nothing to patch."
    exit 0
fi

python3 - "${schema}" <<'PY'
import json, sys
path = sys.argv[1]
with open(path) as fh:
    schema = json.load(fh)

props = schema.setdefault("properties", {})
if "global" in props:
    print(f"patch-hermes-schema: {path} already accepts 'global'; nothing to do")
    sys.exit(0)

# Helm injects an empty `global` map into every subchart's values during
# parent-value coalescing, even when the parent sets nothing. Accept any
# object; the root additionalProperties:false (typo guard for the real
# config surface) stays intact.
props["global"] = {
    "type": "object",
    "description": "Injected by Helm during parent-value coalescing; upstream schema omits it. Added by .useful-scripts/patch_hermes_schema.sh.",
}
with open(path, "w") as fh:
    json.dump(schema, fh, indent=2)
    fh.write("\n")
print(f"patch-hermes-schema: added 'global' to {path}")
PY

# Repack over the SAME tarball name (fresh copy is re-fetched by dep update
# anyway, so digest drift here is irrelevant and never committed).
tar -czf "${TARBALL}" -C "${tmp}" hermes-agent
echo "patch-hermes-schema: repacked ${TARBALL}"
