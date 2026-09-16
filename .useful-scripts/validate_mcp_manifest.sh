#!/usr/bin/env bash
# Render the MCP manifest from values.yaml and lint it with the hermes-tools
# binary (mcp verify --mode validate). Usage: ./useful-scripts/validate_mcp_manifest.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Machines without the Go toolchain skip (CI/devbox install it explicitly).
if ! command -v go >/dev/null 2>&1; then
  echo "go toolchain not installed; skipping MCP manifest validation." >&2
  exit 0
fi

CHART="$ROOT/services/helm/openagent"
cd "$CHART"

helm dependency update . >/dev/null
"$SCRIPT_DIR/patch_hermes_schema.sh" . >/dev/null

helm template . --skip-schema-validation --validate=false > /tmp/openagent-rendered.yaml
yq '. | select(.kind=="ConfigMap" and .metadata.name=="openagent-mcp-manifest") | .data."mcp-manifest"' /tmp/openagent-rendered.yaml > /tmp/mcp-manifest.json

cd extras
go run ./cmd/hermes-tools mcp verify --manifest /tmp/mcp-manifest.json --mode validate
