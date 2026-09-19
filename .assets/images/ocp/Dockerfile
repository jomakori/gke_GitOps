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
# Image is a pristine upstream release — no vendored patches (upstream #494,
# released in v3.37.0, classified the 5-hour session wall as a 429; we carried
# that fix as a patch until the release landed).
#
# Build target: linux/arm64 — the cluster (k3d on OrbStack, Apple Silicon) is arm64.

ARG NODE_IMAGE=node:22-bookworm-slim
FROM ${NODE_IMAGE}

ARG OCP_VERSION=v3.37.0
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

# Vendored-patch block deleted with the v3.37.0 bump — upstream #494 makes the
# 5-hour session wall a 429, so `patches/` and its verify script are gone.
# Keeping it would re-apply legacy content to a release that owns the hunk.
RUN set -eux; \
    rm -rf /opt/ocp/.git

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
