#!/usr/bin/env bash
# OpenAgent helm-unittest runner. Usage: .useful-scripts/helm_unittest_openagent.sh [chart-path]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CHART="${1:-services/helm/openagent}"
CHART_DIR="$ROOT/$CHART"

cd "$CHART_DIR"

# Skip when the plugin isn't installed — pre-commit runs on machines without
# it (CI/devbox install it explicitly).
if ! helm plugin list 2>/dev/null | grep -qw unittest; then
  echo "helm-unittest plugin not installed; skipping." >&2
  echo "Install: helm plugin install https://github.com/helm-unittest/helm-unittest.git --version v1.1.2 --verify=false" >&2
  exit 0
fi

# helm-unittest extracts OCI subchart templates into charts/<name>/ but omits
# Chart.yaml, so the leftover dir makes the next run fail to unpack the
# subchart. Clear only the tgz-backed OCI extracts; local subcharts keep dirs.
rm -rf charts/litellm-helm charts/hermes-agent

helm dependency update . >/dev/null

# Add `global` to the fetched hermes-agent values schema (idempotent) so the
# schema-validating unittest run does not reject Helm's injected global key.
"$SCRIPT_DIR/patch_hermes_schema.sh" . >/dev/null

helm unittest .
