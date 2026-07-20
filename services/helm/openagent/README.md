# openagent

![Version: 2.0.0](https://img.shields.io/badge/Version-2.0.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.0.0](https://img.shields.io/badge/AppVersion-1.0.0-informational?style=flat-square)

Umbrella chart for the openagent stack — Hermes Agent (gateway + Discord bot),
Hermes Workspace (swarm controller + UI), LiteLLM gateway, and Headroom proxy.
All components deployed via upstream deps and subcharts.

## Maintainers

No active maintainers.

## Dependencies

| Dependency | Version | Repository | Description |
|-----------|---------|------------|-------------|
| hermes-agent | 0.9.1 | https://jyje.github.io/hermes-agent-helm | Hermes Agent gateway + Discord bot |
| hermes-workspace | 0.1.0 | file://charts/hermes-workspace | Swarm controller + Web UI |
| litellm-helm | 1.92.0 | file://charts/litellm-helm | Multi-provider LLM gateway |
| openagent-component | 0.1.0 | file://charts/openagent-component | Headroom proxy (bot disabled) |

## Under the Hood

The umbrella chart consists of four layers:

1. **Hermes Agent** (upstream dependency): Gateway + Dashboard + Discord bot. Configured via `hermes.*` in values.yaml.

2. **Hermes Workspace** (custom subchart): Swarm controller with 9 OMO-mapped workers + Web UI. Configured via `hermes-workspace.*` in values.yaml.

3. **LiteLLM** (upstream dependency): Multi-provider LLM gateway with model routing, fallbacks, and key management. Configured via `litellm.*` in values.yaml.

4. **OpenAgent Component** (subchart): Headroom proxy with SQLite CCR cache. Bot is disabled (replaced by Hermes Discord bot). Configured via `openagent-component.*` in values.yaml.

5. **Local templates**: Shared resources:
   - `templates/db/` — StackGres CRDs (SGCluster, SGPostgresConfig, SGPoolingConfig)
   - `templates/shared/` — Shared ExternalSecret, GHCR pull secret, VPA, helpers
   - `templates/swarm/` — Swarm ConfigMap (9 OMO-mapped workers)

### LLM Routing

All LLM traffic follows this path:

```
Discord User
  → Hermes Agent Discord Bot (built-in)
    → Hermes Gateway (port 8642)
      → Hermes Workspace (swarm controller, port 3000)
        → Swarm Workers (9 OMO-mapped personas via tmux)
      → LiteLLM (port 4000)
        → Provider APIs
```

The headroom proxy provides a SQLite CCR (Cache-Clause-Response) cache.
LiteLLM manages 12 models across 5 providers with configurable fallbacks.

## Setup

### Namespace
All components deploy to the `openagent` namespace (configurable via `.Values.namespace`).

### Secrets
The chart expects an `openagent-secrets` ExternalSecret pulling from Doppler config `svc_openagent`. Provider keys flow through this secret.

### ArgoCD Sync
The chart includes sync-wave annotations:
- Wave -2: GHCR pull secret
- Wave -1: ExternalSecrets
- Wave 1: Infrastructure (services, PVCs, RBAC, StackGres)
- Wave 2: Deployments, VPAs, CRDs

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| clusterDomain | string | `"maklab.net"` | Cluster domain for DNS |
| dopplerConfig | string | `"svc_openagent"` | Doppler config for ExternalSecrets |
| namespace | string | `"openagent"` | Target namespace |
| ghcrPullSecret | string | `""` | GHCR pull secret (base64) |
| storageClass | string | `"local-path"` | Default storage class |
| hermes.enabled | bool | `true` | Enable Hermes Agent |
| hermes.config.model.provider | string | `"litellm"` | LLM provider |
| hermes.config.providers.litellm.base_url | string | `"http://openagent-litellm.openagent.svc.cluster.local:4000/v1"` | LiteLLM endpoint |
| hermes.env.OPENAI_API_KEY | string | `"unused"` | OpenAI API key (unused, using LiteLLM) |
| hermes.extraEnvFrom | list | `[secretRef: openagent-secrets]` | Additional secrets |
| hermes-workspace.enabled | bool | `true` | Enable Hermes Workspace |
| litellm.enabled | bool | `true` | Enable LiteLLM upstream chart |
| litellm.nameOverride | string | `"openagent-litellm"` | LiteLLM service name |
| litellm.image.repository | string | `"ghcr.io/berriai/litellm"` | LiteLLM image |
| litellm.image.tag | string | `"main-stable"` | LiteLLM image tag |
| litellm.service.port | int | `4000` | LiteLLM service port |
| litellm.resources.requests.cpu | string | `"100m"` | LiteLLM CPU request |
| litellm.resources.requests.memory | string | `"256Mi"` | LiteLLM memory request |
| litellm.resources.limits.cpu | string | `"2000m"` | LiteLLM CPU limit |
| litellm.resources.limits.memory | string | `"2Gi"` | LiteLLM memory limit |
| litellm.db.deployStandalone | bool | `false` | Use existing PostgreSQL |
| litellm.db.useExisting | bool | `true` | Use existing PostgreSQL |
| litellm.db.endpoint | string | `"openagent-pg-openagent-pg.openagent.svc.cluster.local"` | PostgreSQL endpoint |
| litellm.db.database | string | `"litellm"` | LiteLLM database name |
| litellm.db.secret.name | string | `"openagent-litellm-secrets"` | DB secret name |
| litellm.masterkeySecretName | string | `"openagent-litellm-secrets"` | Master key secret |
| litellm.environmentSecrets | list | `["openagent-secrets"]` | Additional secrets |
| litellm.proxy_config | object | `{}` | LiteLLM proxy config (model_list, fallbacks, etc.) |
| component.enabled | bool | `true` | Enable component subchart |
| openagent-component.headroom.enabled | bool | `true` | Enable headroom proxy |
| openagent-component.headroom.image.repository | string | `"ghcr.io/chopratejas/headroom"` | Headroom image |
| openagent-component.headroom.image.tag | string | `"latest"` | Headroom image tag |
| openagent-component.headroom.service.port | int | `8787` | Headroom service port |
| openagent-component.headroom.resources.requests.cpu | string | `"200m"` | Headroom CPU request |
| openagent-component.headroom.resources.requests.memory | string | `"512Mi"` | Headroom memory request |
| openagent-component.headroom.storage.size | string | `"2Gi"` | Headroom PVC size |
| openagent-component.headroom.storage.accessMode | string | `"ReadWriteOnce"` | Headroom PVC access mode |
| openagent-component.headroom.litellmUrl | string | `"http://openagent-litellm.openagent.svc.cluster.local:4000/v1"` | Upstream LiteLLM URL |
| openagent-component.bot.enabled | bool | `false` | Enable Discord bot (disabled, using Hermes) |
| postgres.enabled | bool | `true` | Enable StackGres cluster |
| postgres.clusterName | string | `"openagent-pg"` | StackGres cluster name |
| postgres.instances | int | `1` | PostgreSQL instances |
| postgres.storage | string | `"5Gi"` | PostgreSQL storage size |
| postgres.version | string | `"18"` | PostgreSQL version |
| postgres.profile | string | `"development"` | Postgres profile |
| dashboard.subdomain | string | `"openagent"` | Dashboard subdomain |
| dashboard.destination.host | string | `"openagent-hermes-workspace.openagent.svc.cluster.local"` | Dashboard upstream host |
| dashboard.destination.port | int | `3000` | Dashboard upstream port |
| vpa.enabled | bool | `true` | Enable controller VPA |

## Model List

The chart configures 12 models via LiteLLM proxy_config:

| Model Name | Provider | API Key Source |
|-----------|----------|---------------|
| opencode/big-pickle | OpenCode | OPENCODE_API_KEY |
| opencode/north-mini-code-free | OpenCode | OPENCODE_API_KEY |
| opencode/deepseek-v4-flash-free | OpenCode | OPENCODE_API_KEY |
| opencode/mimo-v2.5-free | OpenCode | OPENCODE_API_KEY |
| anthropic/claude-opus-4-7 | Anthropic | ANTHROPIC_API_KEY |
| moonshotai/kimi-k2.6 | Moonshot | MOONSHOT_API_KEY |
| deepseek-v4-flash | DeepSeek | DEEPSEEK_API_KEY |
| deepseek-v4-pro | DeepSeek | DEEPSEEK_API_KEY |
| zai/glm-4.7 | z.ai | ZAI_API_KEY |
| zai/glm-4.7-flash | z.ai | ZAI_API_KEY |
| minimax/M3 | MiniMax | MINIMAX_API_KEY |
| minimax/M2.7 | MiniMax | MINIMAX_API_KEY |

Fallback chain: opencode models fall back to deepseek-v4-flash.

## Swarm Configuration

The Hermes Workspace manages 9 OMO-mapped workers via the Swarm ConfigMap:

| Worker ID | Name | Role | Model |
|-----------|------|------|-------|
| orchestrator | Sisyphus | Intent Router | claude/sonnet-4 |
| supervisor | Atlas | Quality Gate | claude/sonnet-4 |
| strategist | Prometheus | Strategic Planner | deepseek-v4-pro |
| reviewer | Momus | Plan Reviewer | deepseek-v4-pro |
| architect | Oracle | Architecture Consultant | claude/opus-4 |
| builder | Hephaestus | Implementation Coder | minimax/M3 |
| researcher | Librarian | Docs/RAG Searcher | zai/glm-4.7-flash |
| explorer | Explore | Codebase Discovery | zai/glm-4.7-flash |
| junior-builder | Sisyphus-junior | Focused Task Executor | deepseek-v4-flash |

Workers run as tmux sessions inside the Workspace container. The Workspace Conductor API handles mission assignment and Kanban board state.

## Verification

```bash
# Validate chart
helm lint services/helm/openagent/

# Render templates
helm template openagent services/helm/openagent/

# Test with specific components disabled
helm template openagent services/helm/openagent/ --set openagent-component.headroom.enabled=false
helm template openagent services/helm/openagent/ --set openagent-component.bot.enabled=false

# View complete rendered output with debug info
helm template openagent services/helm/openagent/ --debug 2>&1 | less
```
