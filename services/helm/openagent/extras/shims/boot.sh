#!/bin/sh
# hermes-tools shim — gateway pod entrypoint (values hermes-agent.command).
# Rebuilds the Go sources from the indexed ConfigMap blobs (keys cannot hold
# slashes — see templates/boot.yaml), builds the boot binary once onto the PVC
# with the mise-installed Go toolchain, then hands over to it. The old boot.sh
# python flow is gone; the Go boot reconstructs the script 1:1 and ends with
# `exec /init hermes gateway run`.
set -u
src=/opt/src
build=/opt/data/tools-src
if [ ! -x /opt/data/bin/mise ]; then
  curl -fsSL https://mise.run | MISE_INSTALL_PATH=/opt/data/bin/mise sh 2>/dev/null || true
fi
if [ ! -x /opt/data/bin/hermes-tools ] || [ "$src/tools-src.manifest" -nt /opt/data/bin/hermes-tools ]; then
  rm -rf "$build"
  mkdir -p "$build"
  i=0
  while IFS= read -r rel; do
    [ -n "$rel" ] || continue
    i=$((i + 1))
    dir=${rel%/*}
    [ "$dir" = "$rel" ] || mkdir -p "$build/$dir"
    cat "$src/f$(printf '%04d' "$i")" > "$build/$rel"
  done < "$src/tools-src.manifest"
  if ! (cd "$build" && /opt/data/bin/mise exec go -- go build -o /opt/data/bin/hermes-tools ./cmd/hermes-tools) 2>/dev/null; then
    echo "WARN: hermes-tools build failed — starting gateway without boot toolchain" >&2
    exec /init hermes gateway run
  fi
fi
exec /opt/data/bin/hermes-tools boot "$@"
