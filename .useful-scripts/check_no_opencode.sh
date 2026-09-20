#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ALLOW='no opencode|opencode workstation|local opencode|~/.config/opencode|local agent skills'

hits="$(grep -rIi "opencode" \
  "$ROOT/services/helm/openagent" \
  "$ROOT/.useful-scripts/gitopsctl" 2>/dev/null \
  | grep -Eiv "$ALLOW" || true)"

if [ -n "$hits" ]; then
  echo "OpenCode references remain in the cluster surface:" >&2
  echo "$hits" >&2
  exit 1
fi

echo "ok: no opencode references in the cluster surface"
