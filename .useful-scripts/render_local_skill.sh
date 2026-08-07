#!/bin/bash
# render_local_skill.sh
#
# Renders the local opencode SKILL.md for k8s-gitops-context from the single
# canonical source in the GitOps repo (services/helm/openagent chart).
#
# The GitOps repo is the single source of truth. The in-cluster ConfigMap
# (runtimeMode=cluster) and this local SKILL.md (runtimeMode=local) are both
# generated from the same Helm template. This script produces the local copy;
# it is a GENERATED artifact and must never be hand-edited.
#
# Usage:
#   ./.useful-scripts/render_local_skill.sh
#
# Requires: helm, yq (mikefarah yq v4+). macOS-safe (no GNU-isms).

set -euo pipefail

# Resolve repo root (parent of .useful-scripts)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CHART_DIR="${REPO_ROOT}/services/helm/openagent"

OUT_DIR="${HOME}/.config/opencode/skills/k8s-gitops-context"
OUT_FILE="${OUT_DIR}/SKILL.md"

# Local-only frontmatter (NOT rendered into the cluster ConfigMap, which is
# mounted as a memory file and must stay frontmatter-free).
FRONTMATTER='---
name: k8s-gitops-context
description: Use when working on k8s, GitOps, ArgoCD, Terraform, Istio, Cloudflare tunnel, or Doppler ESO for the jmak-lab cluster. Covers both repos (devops_Terraform/k8s-maklab-cluster + gke_GitOps): execution order, variable patterns, secrets chain, sync waves, helm chart patterns, Istio ambient mesh, Cloudflare tunnel gotchas, and ClusterSecretStore setup.
license: MIT
metadata:
  repos: devops_Terraform/k8s-maklab-cluster,gke_GitOps
  domain: maklab.net
---
'

if ! command -v helm >/dev/null 2>&1; then
    echo "ERROR: helm not found. Install via 'brew install helm'." >&2
    exit 1
fi
if ! command -v yq >/dev/null 2>&1; then
    echo "ERROR: yq not found. Install via 'brew install yq' (mikefarah yq v4+)." >&2
    exit 1
fi

# Ensure remote OCI/HTTP chart deps (hermes-agent, litellm-helm) are extracted
# into charts/ so `helm template` can render. Idempotent; charts/ is gitignored.
if [ ! -d "${CHART_DIR}/charts/hermes-agent" ]; then
    echo "...fetching chart dependencies (helm dependency update)"
    ( cd "${CHART_DIR}" && helm dependency update ) >/tmp/render_local_skill_deps.log 2>&1 \
        || { echo "ERROR: helm dependency update failed:" >&2; cat /tmp/render_local_skill_deps.log >&2; exit 1; }
fi

# Render the chart in local mode and extract the skill body. yq unfolds the
# YAML block scalar, so the 4-space nindent is already stripped to column 0.
BODY="$(
    helm template "${CHART_DIR}" \
        --set runtimeMode=local \
        --skip-schema-validation \
        --show-only templates/skills/k8s-gitops-context.yaml 2>/tmp/render_local_skill_helm.err \
        | yq '.data["k8s-gitops-context.md"]'
)"
if [ -z "${BODY}" ] || [ "${BODY}" = "null" ]; then
    echo "ERROR: helm template produced no skill body. helm stderr:" >&2
    cat /tmp/render_local_skill_helm.err >&2
    exit 1
fi

# Sanity check: local render must contain local paths and must NOT contain
# cluster-only GitHub MCP paths. Use here-strings (not pipes) so grep -q does
# not SIGPIPE the upstream printf under `set -o pipefail`.
if ! grep -q '/Users/maklab' <<< "${BODY}"; then
    echo "ERROR: local render missing /Users/maklab paths — aborting." >&2
    exit 1
fi
if grep -q 'github_search_repositories' <<< "${BODY}"; then
    echo "ERROR: local render contains cluster-only GitHub MCP Repo Pair — aborting." >&2
    exit 1
fi

mkdir -p "${OUT_DIR}"

# Prepend frontmatter, strip trailing blank lines, ensure single trailing newline.
{
    printf '%s' "${FRONTMATTER}"
    printf '%s' "${BODY}"
} | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}' > "${OUT_FILE}"

# Ensure the file ends with exactly one newline.
if [ -n "$(tail -c 1 "${OUT_FILE}")" ]; then
    printf '\n' >> "${OUT_FILE}"
fi

echo "Wrote ${OUT_FILE}"
echo "  (generated from ${CHART_DIR} — do not hand-edit)"
