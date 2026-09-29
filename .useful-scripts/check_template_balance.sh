#!/usr/bin/env bash
# Check every Helm chart for unbalanced Go template blocks using the gitopsctl
# binary (`gitopsctl chart templates`). Usage:
#   .useful-scripts/check_template_balance.sh [chart_dir ...]
# With no arguments every chart under services/helm and apps/helm is checked.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Machines without the Go toolchain skip (CI/devbox install it explicitly).
if ! command -v go >/dev/null 2>&1; then
  echo "go toolchain not installed; skipping Helm template balance check." >&2
  exit 0
fi

if [ "$#" -gt 0 ]; then
  # bash 3.2 safe: no mapfile, no "${arr[@]}" on an empty array.
  charts=$(printf '%s\n' "$@")
else
  # -exec dirname, not -printf: BSD find (macOS) has no -printf.
  charts=$(find "$ROOT/services/helm" "$ROOT/apps/helm" -maxdepth 2 -name Chart.yaml -exec dirname {} \; 2>/dev/null | sort)
fi

if [ -z "$charts" ]; then
  echo "no charts found; skipping Helm template balance check." >&2
  exit 0
fi

cd "$ROOT/.useful-scripts/gitopsctl"
BIN="$(mktemp -d)/gitopsctl"
go build -o "$BIN" ./cmd/gitopsctl

status=0
while IFS= read -r chart; do
  [ -n "$chart" ] || continue
  "$BIN" chart templates --dir "$chart" || status=1
done <<EOF
$charts
EOF
exit "$status"
