#!/bin/bash
# render_skill_configmap.sh <skill-name>
#
# Regenerates services/helm/openagent/templates/skills/<skill-name>.yaml
# from the live skill at /opt/data/skills/<category>/<skill-name>/SKILL.md.
#
# Canonical source of the skill content is the local skill dir (PVC-backed);
# this script ships it into the Helm chart as a ConfigMap (data key
# <skill-name>.SKILL.md) so ArgoCD delivers it to the pod on every sync.
#
# NOTE: braces in the template header are written with plain string
# concatenation (NOT an f-string) — `{{` must stay literal for Helm.
# Do not "fix" that back into an f-string.

set -euo pipefail

SKILL_NAME="${1:?usage: render_skill_configmap.sh <skill-name>}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
OUT="${REPO_ROOT}/services/helm/openagent/templates/skills/${SKILL_NAME}.yaml"

SRC="$(find /opt/data/skills -maxdepth 2 -type d -name "${SKILL_NAME}" | head -1)"
if [ -z "${SRC}" ]; then
    echo "ERROR: skill '${SKILL_NAME}' not found under /opt/data/skills" >&2
    exit 1
fi
SKILL_MD="${SRC}/SKILL.md"

{
    echo "apiVersion: v1"
    echo "kind: ConfigMap"
    echo "metadata:"
    echo "  name: openagent-skill-${SKILL_NAME}"
    echo "  namespace: {{ .Values.namespace }}"
    echo "  labels:"
    echo "    app.kubernetes.io/part-of: openagent"
    echo "  annotations:"
    echo '    argocd.argoproj.io/sync-wave: "0"'
    echo "data:"
    echo "  ${SKILL_NAME}.SKILL.md: |"
    sed 's/^/    /' "${SKILL_MD}"
} > "${OUT}"

echo "wrote ${OUT}"
