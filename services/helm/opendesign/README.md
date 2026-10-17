# opendesign

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![AppVersion: 0.7.0](https://img.shields.io/badge/AppVersion-0.7.0-informational?style=flat-square)
Open Design — self-hosted spec-driven chart

## Under the hood

This chart deploys **Open Design** (`ghcr.io/nexu-io/od`), a self-hosted spec-driven development environment. Code generation is delegated to a configurable AI agent harness, which talks to Anthropic/OpenAI-compatible endpoints that are by default pointed at the in-cluster `provider` (LiteLLM `responses-proxy` → `claude-proxy` → Claude).

### Agent harness (`values.yaml` `agents` block)

| Agent | Role | Config |
|-------|------|--------|
| **Claude Code** | **Default agent** | `agents.claude.ANTHROPIC_BASE_URL` / `agents.claude.ANTHROPIC_API_KEY` — left empty, the template fills them from the chart's `provider` block (endpoint + model) |
| **Codex** | Fallback / alternate agent | `agents.codex.OPENAI_BASE_URL` / `agents.codex.OPENAI_API_KEY` — same empty-value fallback to `provider`; stays available so a session can be switched to Codex instead of Claude Code |

### Claude Code model behavior (`opusplan`)

Claude Code's default model alias is `opusplan`. It pairs two models **by mode**, not by task complexity:

- **Plan mode** → **Opus** — used while drafting/reviewing a plan.
- **Execution mode** → **Sonnet** — used once the plan is approved and code is actually being written.

This is a fixed, mode-based switch: Opus is not selected because a task is judged "complex"; it only runs during planning, and Sonnet takes over for execution regardless of how large or small the task is. There is no literal task-complexity auto-routing in this chart.

### Fallback agent

Codex remains available as an alternate agent in the same `agents` block (`agents.codex.*`). It can be selected for a session in place of Claude Code; both agents route through the same `provider` (LiteLLM `responses-proxy` → `claude-proxy` → Claude) whenever their own `OPENAI_BASE_URL`/`ANTHROPIC_BASE_URL` overrides are left empty.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| agents.claude.ANTHROPIC_API_KEY | string | `""` |  |
| agents.claude.ANTHROPIC_BASE_URL | string | `""` |  |
| agents.codex.OPENAI_API_KEY | string | `""` |  |
| agents.codex.OPENAI_BASE_URL | string | `""` |  |
| config.allowedOrigins | string | `""` |  |
| config.apiToken | string | `""` |  |
| config.bindHost | string | `"0.0.0.0"` |  |
| config.disableApiAuth | int | `1` |  |
| config.publicBaseUrl | string | `""` |  |
| domain | string | `"design.maklab.net"` |  |
| enable_auth_proxy | bool | `false` |  |
| image.pullPolicy | string | `"IfNotPresent"` |  |
| image.repository | string | `"ghcr.io/nexu-io/od"` |  |
| image.tag | string | `"latest"` |  |
| provider.apiKeyKey | string | `"LITELLM_MASTER_KEY"` |  |
| provider.apiKeyRef | string | `"opendesign-secrets"` |  |
| provider.endpoint | string | `"http://responses-proxy.openagent.svc.cluster.local:4524/v1"` |  |
| provider.mcpArgs | string | `"[\"/app/apps/daemon/dist/cli.js\", \"mcp\", \"--daemon-url\", \"http://127.0.0.1:7456\"]"` |  |
| provider.mcpCommand | string | `"/usr/local/bin/node"` |  |
| provider.model | string | `"claude-sonnet-5"` |  |
| replicaCount | int | `1` |  |
| resources.limits.cpu | string | `"500m"` |  |
| resources.limits.memory | string | `"512Mi"` |  |
| resources.requests.cpu | string | `"200m"` |  |
| resources.requests.memory | string | `"256Mi"` |  |
| service.port | int | `80` |  |
| service.type | string | `"ClusterIP"` |  |
| storage.class | string | `"local-path"` |  |
| storage.size | string | `"10Gi"` |  |
