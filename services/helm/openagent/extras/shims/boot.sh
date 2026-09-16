#!/bin/sh
# hermes-tools shim — gateway pod entrypoint (values hermes-agent.command).
# Builds the Go boot binary once onto the PVC from the ConfigMap-mounted
# sources (mise-installed Go toolchain), then hands over to it. The old boot.sh
# python flow is gone; the Go boot reconstructs the script 1:1 and ends with
# `exec /init hermes gateway run`.
set -u
if [ ! -x /opt/data/bin/mise ]; then
  curl -fsSL https://mise.run | MISE_INSTALL_PATH=/opt/data/bin/mise sh 2>/dev/null || true
fi
if [ ! -x /opt/data/bin/hermes-tools ] || [ /opt/src/go.mod -nt /opt/data/bin/hermes-tools ]; then
  if ! (cd /opt/src && /opt/data/bin/mise exec go -- go build -o /opt/data/bin/hermes-tools ./cmd/hermes-tools) 2>/dev/null; then
    echo "WARN: hermes-tools build failed — starting gateway without boot toolchain" >&2
    exec /init hermes gateway run
  fi
fi
exec /opt/data/bin/hermes-tools boot "$@"
