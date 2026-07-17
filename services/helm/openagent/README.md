# openagent

![Version: 2.0.0](https://img.shields.io/badge/Version-2.0.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.0.0](https://img.shields.io/badge/AppVersion-1.0.0-informational?style=flat-square)

Umbrella chart for the openagent stack — LiteLLM gateway, Headroom proxy,
Discord bot, and Sympozium CRDs (Ensemble, SkillPacks). All components
deployed via upstream deps and subcharts. Only CRDs + shared resources
remain as local templates.

## Maintainers

No active maintainers.

## Dependencies

| Dependency | Version | Repository | Description |
|-----------|---------|------------|-------------|
| litellm-helm | 1.92.0 | file://charts/litellm-helm | Multi-provider LLM gateway |
| openagent-component | 0.1.0 | file://charts/openagent-component | Headroom proxy + Discord bot |

## Under the Hood

The umbrella chart consists of three layers:

1. **Upstream dependency**: `litellm-helm` (vendored at `charts/litellm-helm/`) — the LiteLLM gateway providing multi-provider LLM access with model routing, fallbacks, and key management. Configured via `litellm.*` in values.yaml.

2. **Subchart**: `openagent-component` (at `charts/openagent-component/`) — a single component subchart containing both:
   - **Headroom proxy**: LLM proxy with SQLite CCR cache, routes to LiteLLM
   - **Discord bot**: Go-based Discord gateway bot
   Controlled via `component.enabled` (master switch), `openagent-component.headroom.enabled` and `openagent-component.bot.enabled` for individual components.

3. **Local templates**: Organized in subdirectories:
   - `templates/ensemble/` — Ensemble CRD (Sympozium loop engineering)
   - `templates/skillpacks/` — SkillPack CRDs (k8s-ops, hashline-editor, omo-core-skills)
   - `templates/db/` — StackGres CRDs (SGCluster, SGPostgresConfig, SGPoolingConfig)
   - `templates/shared/` — Shared ExternalSecret, GHCR pull secret, VPA, helpers
   - `templates/gateway/` — Istio VirtualService + AuthorizationPolicy

### LLM Routing

All LLM traffic follows this path:

```
Discord Bot (openagent-discord:8080)
  → Sympozium Sisyphus Web Endpoint (sympozium-system:8080)
    → Headroom Proxy (openagent-headroom:8787)
      → LiteLLM Gateway (openagent-litellm:4000)
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

## Source Code
Bot source code is available at `src/` (sibling to chart root). This Go binary implements the Discord gateway bot that polls Discord and routes messages to the Sympozium Sisyphus web endpoint.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| clusterDomain | string | `"maklab.net"` | Cluster domain for DNS |
| dopplerConfig | string | `"svc_openagent"` | Doppler config for ExternalSecrets |
| namespace | string | `"openagent"` | Target namespace |
| ghcrPullSecret | string | `""` | GHCR pull secret (base64) |
| storageClass | string | `"local-path"` | Default storage class |
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
| openagent-component.bot.enabled | bool | `true` | Enable Discord bot |
| openagent-component.bot.image.repository | string | `"ghcr.io/jomakori/gke_gitops/openagent-discord-bot"` | Bot image |
| openagent-component.bot.image.tag | string | `"0.6.0"` | Bot image tag |
| openagent-component.bot.service.port | int | `8080` | Bot service port |
| openagent-component.bot.resources.requests.cpu | string | `"50m"` | Bot CPU request |
| openagent-component.bot.resources.requests.memory | string | `"128Mi"` | Bot memory request |
| openagent-component.bot.config.conversationMode | string | `"threaded"` | Bot conversation mode |
| openagent-component.bot.config.dashboardUrl | string | `"https://openagent.maklab.net"` | Dashboard URL |
| openagent-component.bot.config.thinkMode | string | `"full"` | Think mode: `full` (log streaming), `simple` (phase transitions), `off` (silent) |
| postgres.enabled | bool | `true` | Enable StackGres cluster |
| postgres.clusterName | string | `"openagent-pg"` | StackGres cluster name |
| postgres.instances | int | `1` | PostgreSQL instances |
| postgres.storage | string | `"5Gi"` | PostgreSQL storage size |
| postgres.version | string | `"18"` | PostgreSQL version |
| postgres.profile | string | `"development"` | Postgres profile |
| dashboard.subdomain | string | `"openagent"` | Dashboard subdomain |
| dashboard.destination.host | string | `"sympozium-apiserver.sympozium-system.svc.cluster.local"` | Dashboard upstream host |
| dashboard.destination.port | int | `8080` | Dashboard upstream port |
| ensemble.enabled | bool | `true` | Enable Ensemble CRD |
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

## Conversation Persistence

The bot persists conversation metadata to `/state/conversations.json` on a 10Mi PVC (`openagent-discord-state`).
- **Startup**: Loads existing conversations on boot — threads survive pod restarts
- **Saves**: After every conversation state change (thread creation, registration, updates)
- **Atomic writes**: Uses temp file + rename to prevent corruption
- **Leader election**: Only the leader pod writes state (ConfigMap lock `openagent-discord-leader` in `openagent` namespace)

## Think Mode

Configured via `openagent-component.bot.config.thinkMode`:

| Mode | Value | Behavior |
|------|-------|----------|
| Full | `"full"` | Streams pod logs during AgentRun, posts event labels to Discord (capped at 10) |
| Simple | `"simple"` | Posts phase transitions (Pending → Running → Succeeded/Failed) |
| Off | `"off"` | No thinking updates posted |

Default: `"full"`.

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
