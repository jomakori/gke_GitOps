# Pin HOME+caches to the PVC so boot installs survive restarts and MCPs
# (uid 10000, HOME=/opt/data/home) reuse them instead of re-downloading.
export HOME=/opt/data/home
export NPM_CONFIG_CACHE=$HOME/.npm
export UV_CACHE_DIR=$HOME/.cache/uv
export UV_TOOL_DIR=$HOME/.local/share/uv/tools
export UV_TOOL_BIN_DIR=$HOME/.local/bin
# Toolchain via mise (/mise/mise.toml) — node/deno/uv/go on the PVC.
export MISE_DATA_DIR=/opt/data/mise
export MISE_CACHE_DIR=/opt/data/mise-cache
export MISE_CONFIG_FILE=/mise/mise.toml
if [ ! -x /opt/data/bin/mise ]; then
  curl -fsSL https://mise.run | MISE_INSTALL_PATH=/opt/data/bin/mise sh
fi
export PATH=/opt/data/mise/shims:/opt/data/bin:$HOME/.local/bin:$PATH
mise install -y 2>&1 || true
mise ls >/dev/null 2>&1 || { echo "ERROR: mise config failed to parse — see /mise/mise.toml" >&2; }
# Desktop-E2E X11 toolchain (bats user flows + visual regression run
# locally; mirrors the openkite e2e.yml apt list). Root at boot;
# idempotent — skipped when already present.
if ! command -v bats >/dev/null 2>&1 || ! command -v xdotool >/dev/null 2>&1; then
  apt-get update -qq 2>/dev/null || true
  DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    xvfb xdotool openbox imagemagick dbus-x11 bats 2>/dev/null || true
fi
# Rust link-stage build deps (webkit2gtk/gtk/glib .pc files) so local
# `cargo test` runs instead of blind CI round-trips; mirrors the
# openkite .github/actions/rust-setup apt list. Idempotent guard.
if ! pkg-config --exists glib-2.0 2>/dev/null; then
  DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    libwebkit2gtk-4.1-dev libgtk-3-dev libglib2.0-dev \
    libayatana-appindicator3-dev librsvg2-dev libxdo-dev libssl-dev 2>/dev/null || true
fi
# /init resets PATH — symlinks into /opt/data/bin (first on PATH) are
# what reach MCPs.
# Link ALL mise shims into /opt/data/bin (first on PATH). Direct-download
# only for tools mise doesn't ship (obscura, agent-reach, k3d stub).
for sh in /opt/data/mise/shims/*; do
  [ -x "$sh" ] && ln -sf "$sh" /opt/data/bin/"$(basename "$sh")"
done
cd /opt/data && go mod download
# obscura (stealth browser) — not in mise registry
if [ ! -x /opt/data/bin/obscura ]; then
  curl -sL https://github.com/h4ckf0r0day/obscura/releases/download/v0.2.0/obscura-aarch64-linux-stealth.tar.gz | tar -xz -C /opt/data/bin
fi
# agent-reach — not in mise registry
command -v agent-reach >/dev/null 2>&1 || uv tool install https://github.com/Panniantong/agent-reach/archive/main.zip 2>&1
mkdir -p $HOME/.config/yt-dlp && { grep -qxF -- '--js-runtimes node' $HOME/.config/yt-dlp/config 2>/dev/null || printf '%s\n' '--js-runtimes node' >> $HOME/.config/yt-dlp/config; }
# OCR-first vision — mise task (see /mise/mise.toml [tasks.paddle-ocr]):
# self-bootstraps the py3.12 venv on first use (persists on PVC); PP-OCR
# models via rapidocr-onnxruntime (native paddlepaddle crashes on aarch64).
cat > /opt/data/bin/paddle-ocr <<'PADDLEOCR'
#!/bin/bash
exec mise run paddle-ocr -- "$@"
PADDLEOCR
chmod +x /opt/data/bin/paddle-ocr
# deno fallback: release zip + python zipfile (install.sh needs unzip/7z,
# absent); v2.9.5 pinned for drawio's deno.lock v5.
if [ ! -x /opt/data/bin/deno ] && [ ! -x $HOME/.deno/bin/deno ]; then
  mkdir -p $HOME/.deno/bin /tmp/deno_x
  DENO_ARCH=$(uname -m | sed 's/aarch64/arm64/')
  curl -fsSL --retry 3 "https://github.com/denoland/deno/releases/download/v2.9.5/deno-${DENO_ARCH}-unknown-linux-gnu.zip" -o /tmp/deno.zip \
    && python3 -c "import zipfile; zipfile.ZipFile('/tmp/deno.zip').extractall('/tmp/deno_x')" \
    && mv /tmp/deno_x/deno $HOME/.deno/bin/deno \
    && chmod +x $HOME/.deno/bin/deno \
    || rm -f /tmp/deno.zip
fi
[ -x $HOME/.deno/bin/deno ] && ln -sf $HOME/.deno/bin/deno /opt/data/bin/deno
# drawio MCP needs a local checkout (remote-URL run never discovers deno.json).
if [ ! -d /opt/data/drawio-mcp-server/.git ]; then
  git clone --depth 1 https://github.com/simonkurtz-MSFT/drawio-mcp-server /opt/data/drawio-mcp-server 2>&1 || true
fi
# Doppler CLI → PVC. gpgv verify skipped (no gnupg as uid 10000).
if [ ! -x /opt/data/bin/doppler ]; then
  mkdir -p /opt/data/bin
  DOP_ARCH=$(uname -m | sed 's/aarch64/arm64/; s/x86_64/amd64/')
  curl -fsSL --retry 3 "https://cli.doppler.com/download?os=linux&arch=${DOP_ARCH}&format=tar" -o /tmp/doppler.tgz \
    && tar -xzf /tmp/doppler.tgz -C /opt/data/bin doppler \
    && chmod +x /opt/data/bin/doppler \
    || rm -f /tmp/doppler.tgz
fi
# ── Pre-warm the private npx/uvx caches WITHOUT starting a server ──────
# The MCP client runs `npx -y <pkg>`, which installs the package at every
# start into that server's private npm_config_cache. That install happens
# INSIDE the MCP connect window: cancel the connect and the client kills
# the child mid-reify, and npm's install is not atomic — the leftover
# node_modules looks complete, so every later start SKIPS the install and
# the server dies on a missing module (argocd "Cannot find module
# 'pino-std-serializers'", gistpad with no _npx at all).
#
# The previous mechanism fixed that by RUNNING each server at boot and
# relying on `timeout` to reap it. MCP servers do not exit on stdin EOF
# and `timeout` only signals the direct child, so the `node` grandchildren
# leaked — verified alive 3h42m after boot. files/mcp/prewarm.py instead:
#   * materialises each cache with a package-resolution command that never
#     runs the server (`npm exec --package=<pkg> -- true`, `uv tool install`);
#   * runs every child in its own session/process group and hard-kills the
#     GROUP (SIGTERM then SIGKILL) on timeout, so nothing can outlive it;
#   * sweeps `_npx`/npm-cache orphans left by older boots and asserts none
#     remain;
#   * reads the package list from the rendered manifest (no second copy to
#     drift) and skips work when a cache is already warm.
# Non-fatal: a pre-warm failure must never block the gateway.
if [ -f /opt/data/mcp-verify/prewarm.py ]; then
  python3 /opt/data/mcp-verify/prewarm.py || echo "WARN: MCP cache pre-warm reported errors (non-fatal)" >&2
else
  echo "WARN: /opt/data/mcp-verify/prewarm.py missing — MCP caches not pre-warmed" >&2
fi
# Boot ran as root — chown caches/tools to runtime uid 10000 (npx/uvx/
# mise shims EACCES otherwise). Includes the MCP private caches: this
# script runs as root, and root-owned entries in a cache the gateway
# writes to are a "Permission denied" waiting to happen.
chown -R 10000:10000 $HOME/.npm $HOME/.cache $HOME/.deno $HOME/.local $HOME/.config /opt/data/mise /opt/data/mise-cache /opt/data/npm-global /opt/data/.npm-mcp 2>/dev/null || true
# Anything still root-owned elsewhere on the shared volume. The agent pod
# (opencode-server) had no runAsUser, so it wrote as the image default uid
# into a tree owned by 10000 — root-owned git object directories were the
# visible symptom ("insufficient permission for adding an object to
# repository database" when committing in a shared worktree). Targeted,
# not recursive, so the boot stays fast on a multi-GB volume.
find /opt/data/repos /opt/data/wt /opt/data/.local /opt/data/.config /opt/data/.omo -user root -exec chown 10000:10000 {} + 2>/dev/null || true
# ── OpenCode CLI + hermes-opencode-plugin (replaces OMO fleet bootstrap) ──
# Heavy engineering dispatches via opencode tool → opencode run subprocess
# → agents in ~/.config/opencode/opencode.json (mounted ConfigMap). All
# idempotent; failure non-fatal via ||; must never block the gateway.
# opencode CLI → PVC (npm; survives restarts like the mise shims).
if [ ! -x /opt/data/bin/opencode ]; then
  export NPM_CONFIG_PREFIX=/opt/data/npm-global
  npm install -g opencode-ai 2>&1 || true
  ln -sf /opt/data/npm-global/bin/opencode /opt/data/bin/opencode 2>/dev/null || true
  unset NPM_CONFIG_PREFIX
fi
# Plugin: clone into the default profile's plugins dir (HERMES_HOME=/opt/data).
if [ ! -d /opt/data/plugins/opencode/.git ]; then
  mkdir -p /opt/data/plugins
  git clone --depth 1 https://github.com/zaycruz/hermes-opencode-plugin.git /opt/data/plugins/opencode 2>&1 || true
fi
# Plugin skill → skills tree (opencode-driven-development).
mkdir -p /opt/data/skills/software-development/opencode-driven-development
cp /opt/data/plugins/opencode/SKILL.md \
  /opt/data/skills/software-development/opencode-driven-development/SKILL.md 2>/dev/null || true
# OpenCode/OMO runtime configs. opencode.json is mounted read-only at
# ~/.config/opencode/opencode.json (never rewritten by opencode). OMO
# NORMALIZES ~/.omo/omo.jsonc (migrations, model dedupe) → copy the
# template to a writable path each boot + chown to runtime uid.
mkdir -p /opt/data/.omo
cp /opt/opencode/omo.jsonc /opt/data/.omo/omo.jsonc 2>/dev/null || true
chown -R 10000:10000 /opt/data/plugins /opt/data/.omo 2>/dev/null || true
exec /init hermes gateway run
