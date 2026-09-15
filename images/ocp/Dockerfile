# syntax=docker/dockerfile:1
#
# OCP (open-claude-proxy) — OpenAI-compatible gateway in front of the Claude Code
# CLI, with caller-side tool calling (upstream ADR 0022): client `tools` are handed
# to the `claude -p` spawn as an internal MCP server and a chosen call comes back as
# `choices[0].message.tool_calls` with `finish_reason: "tool_calls"` — the caller
# executes the tool. That is the capability claude-pipe (the previous image) lacks:
# it drives the same CLI but silently drops client tools.
#
# Upstream (MIT): https://github.com/dtzp555-max/ocp — publishes neither an npm
# package nor a container image, so we vendor a pinned release tag at build time.
# Pinned deliberately: OCP spawns the CLI and is the only thing between our agents
# and the subscription, so the version in the image is the version we validated.
# One vendored patch IS applied on top of that tag — see the `patches/` block
# below; it is deleted the moment an upstream release carries the fix.
#
# Build target: linux/arm64 — the cluster (k3d on OrbStack, Apple Silicon) is arm64.

ARG NODE_IMAGE=node:22-bookworm-slim
FROM ${NODE_IMAGE}

ARG OCP_VERSION=v3.36.0
ARG CLAUDE_CODE_VERSION=2.1.272

LABEL org.opencontainers.image.title="ocp" \
      org.opencontainers.image.description="OCP (open-claude-proxy): OpenAI-compatible gateway for the Claude Code CLI with caller-side tool calling" \
      org.opencontainers.image.source="https://github.com/jomakori/gke_GitOps" \
      org.opencontainers.image.version="${OCP_VERSION}" \
      org.opencontainers.image.licenses="MIT"

# Defaults are set here so the container works the same way outside k8s; the
# Deployment overrides the ones that are cluster-specific.
ENV NODE_ENV=production \
    HOME=/home/ocp \
    CLAUDE_PROXY_PORT=4523 \
    CLAUDE_BIND=0.0.0.0 \
    OCP_TOOL_CALLING=1 \
    CLAUDE_NO_CONTEXT=true

# tini reaps the `claude -p` grandchildren OCP spawns on every request; git is
# needed only to vendor the source below and is purged afterwards.
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates git tini; \
    rm -rf /var/lib/apt/lists/*

RUN set -eux; \
    git clone --depth 1 --branch "${OCP_VERSION}" https://github.com/dtzp555-max/ocp.git /opt/ocp

# --- Vendored patch: the subscription wall is a 429, not a 500 -----------------
# Upstream v3.36.0's classifier (`lib/upstream-errors.mjs`) matches `usage limit`,
# `rate limit` and the quota nouns — but not the phrasing the 5-hour session wall
# actually prints: `You've hit your session limit · resets 5am (UTC)`. So that wall
# fails CLOSED and callers receive `500 proxy_error`, which agent frameworks treat
# as retryable-and-NOT-failover. Measured on the live gateway: 26 wall frames over
# ~40 minutes, arriving as bursts of three retries every five minutes, every one
# answered 500 — the client spent its retry budget on a wall instead of leaving.
#
# REMOVE this block — `patches/` and the verify script together — once OCP_VERSION
# is a release containing https://github.com/dtzp555-max/ocp/pull/492. Both rot
# directions fail the build on purpose: if upstream edits the same hunk the patch
# stops applying, and if upstream adds the phrase itself the verify script below
# goes red instead of leaving us patching a file nobody watches.
COPY patches/ /tmp/ocp-patches/
RUN set -eux; \
    cd /opt/ocp; \
    for p in /tmp/ocp-patches/*.patch; do git apply --verbose "$p"; done; \
    node /tmp/ocp-patches/verify-session-limit.mjs; \
    rm -rf /opt/ocp/.git /tmp/ocp-patches

# The Claude Code CLI is the upstream OCP actually drives — version-pinned so a
# CLI release cannot change our behaviour silently.
RUN set -eux; \
    npm install -g "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}"; \
    npm cache clean --force; \
    claude --version

RUN set -eux; \
    mkdir -p /home/ocp; \
    apt-get purge -y git; \
    apt-get autoremove -y; \
    rm -rf /var/lib/apt/lists/*

WORKDIR /opt/ocp

EXPOSE 4523

# Liveness signal for non-k8s runtimes; the Deployment uses HTTP probes on /health.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
  CMD node -e "fetch('http://127.0.0.1:'+(process.env.CLAUDE_PROXY_PORT||4523)+'/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"

ENTRYPOINT ["/usr/bin/tini", "--", "node", "/opt/ocp/server.mjs"]
