# openagent

![Version: 1.2.0](https://img.shields.io/badge/Version-1.2.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.0.0](https://img.shields.io/badge/AppVersion-1.0.0-informational?style=flat-square)

Single umbrella chart for the entire openagent stack — Sympozium orchestrator, LiteLLM
gateway, Headroom proxy, Discord bot, shared PostgreSQL, and the Sympozium web dashboard.
Everything is inlined in the umbrella's templates/ — no subchart dependencies, no
duplicated source code. The umbrella is the source of truth.

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| local |  |  |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| clusterDomain | string | `"maklab.net"` |  |
| dashboard.destination.host | string | `"sympozium-apiserver.sympozium-system.svc.cluster.local"` |  |
| dashboard.destination.port | int | `8080` |  |
| dashboard.subdomain | string | `"openagent"` |  |
| discord.config.channelOnly | string | `""` |  |
| discord.config.clientId | string | `""` |  |
| discord.config.conversationMode | string | `"threaded"` |  |
| discord.config.dashboardUrl | string | `"https://openagent.maklab.net"` |  |
| discord.config.mentionOnly | bool | `false` |  |
| discord.config.phaseUpdates | bool | `true` |  |
| discord.config.pollUI | bool | `true` |  |
| discord.config.startupChannel | string | `"chat"` |  |
| discord.enabled | bool | `true` |  |
| discord.ghcrPullSecret | string | `""` |  |
| discord.image.pullPolicy | string | `"IfNotPresent"` |  |
| discord.image.repository | string | `"ghcr.io/jomakori/gke_gitops/openagent-discord-bot"` |  |
| discord.image.tag | string | `"0.6.0"` |  |
| discord.resources.limits.cpu | string | `"500m"` |  |
| discord.resources.limits.memory | string | `"512Mi"` |  |
| discord.resources.requests.cpu | string | `"50m"` |  |
| discord.resources.requests.memory | string | `"128Mi"` |  |
| discord.service.port | int | `8080` |  |
| discord.version.configMapName | string | `"openagent-discord-version"` |  |
| dopplerConfig | string | `"svc_openagent"` |  |
| headroom.enabled | bool | `true` |  |
| headroom.ghcrPullSecret | string | `""` |  |
| headroom.image.pullPolicy | string | `"IfNotPresent"` |  |
| headroom.image.repository | string | `"ghcr.io/chopratejas/headroom"` |  |
| headroom.image.tag | string | `"latest"` |  |
| headroom.litellmUrl | string | `"http://openagent-litellm.openagent.svc.cluster.local:4000/v1"` |  |
| headroom.resources.limits.cpu | string | `"2000m"` |  |
| headroom.resources.limits.memory | string | `"2Gi"` |  |
| headroom.resources.requests.cpu | string | `"200m"` |  |
| headroom.resources.requests.memory | string | `"512Mi"` |  |
| headroom.service.port | int | `8787` |  |
| headroom.storage.accessMode | string | `"ReadWriteOnce"` |  |
| headroom.storage.size | string | `"2Gi"` |  |
| litellm.databaseUrl | string | `"postgresql://openagent:CHANGE_ME@openagent-pg.openagent.svc.cluster.local:5432/litellm"` |  |
| litellm.enabled | bool | `true` |  |
| litellm.fallbacks[0].opencode/big-pickle[0] | string | `"deepseek-v4-flash"` |  |
| litellm.fallbacks[1].opencode/north-mini-code-free[0] | string | `"deepseek-v4-flash"` |  |
| litellm.fallbacks[2].opencode/deepseek-v4-flash-free[0] | string | `"deepseek-v4-flash"` |  |
| litellm.ghcrPullSecret | string | `""` |  |
| litellm.image.pullPolicy | string | `"IfNotPresent"` |  |
| litellm.image.repository | string | `"ghcr.io/berriai/litellm"` |  |
| litellm.image.tag | string | `"main-stable"` |  |
| litellm.model_list[0].litellm_params.api_base | string | `"os.environ/OPENCODE_API_BASE"` |  |
| litellm.model_list[0].litellm_params.api_key | string | `"os.environ/OPENCODE_API_KEY"` |  |
| litellm.model_list[0].litellm_params.model | string | `"openai/big-pickle"` |  |
| litellm.model_list[0].model_name | string | `"opencode/big-pickle"` |  |
| litellm.model_list[10].litellm_params.api_base | string | `"os.environ/MINIMAX_API_BASE"` |  |
| litellm.model_list[10].litellm_params.api_key | string | `"os.environ/MINIMAX_API_KEY"` |  |
| litellm.model_list[10].litellm_params.model | string | `"openai/MiniMax-M3"` |  |
| litellm.model_list[10].model_name | string | `"minimax/M3"` |  |
| litellm.model_list[11].litellm_params.api_base | string | `"os.environ/MINIMAX_API_BASE"` |  |
| litellm.model_list[11].litellm_params.api_key | string | `"os.environ/MINIMAX_API_KEY"` |  |
| litellm.model_list[11].litellm_params.model | string | `"openai/MiniMax-M2.7"` |  |
| litellm.model_list[11].model_name | string | `"minimax/M2.7"` |  |
| litellm.model_list[1].litellm_params.api_base | string | `"os.environ/OPENCODE_API_BASE"` |  |
| litellm.model_list[1].litellm_params.api_key | string | `"os.environ/OPENCODE_API_KEY"` |  |
| litellm.model_list[1].litellm_params.model | string | `"openai/north-mini-code-free"` |  |
| litellm.model_list[1].model_name | string | `"opencode/north-mini-code-free"` |  |
| litellm.model_list[2].litellm_params.api_base | string | `"os.environ/OPENCODE_API_BASE"` |  |
| litellm.model_list[2].litellm_params.api_key | string | `"os.environ/OPENCODE_API_KEY"` |  |
| litellm.model_list[2].litellm_params.model | string | `"openai/deepseek-v4-flash-free"` |  |
| litellm.model_list[2].model_name | string | `"opencode/deepseek-v4-flash-free"` |  |
| litellm.model_list[3].litellm_params.api_base | string | `"os.environ/OPENCODE_API_BASE"` |  |
| litellm.model_list[3].litellm_params.api_key | string | `"os.environ/OPENCODE_API_KEY"` |  |
| litellm.model_list[3].litellm_params.model | string | `"openai/mimo-v2.5-free"` |  |
| litellm.model_list[3].model_name | string | `"opencode/mimo-v2.5-free"` |  |
| litellm.model_list[4].litellm_params.api_key | string | `"os.environ/ANTHROPIC_API_KEY"` |  |
| litellm.model_list[4].litellm_params.model | string | `"anthropic/claude-opus-4-7"` |  |
| litellm.model_list[4].model_name | string | `"anthropic/claude-opus-4-7"` |  |
| litellm.model_list[5].litellm_params.api_key | string | `"os.environ/MOONSHOT_API_KEY"` |  |
| litellm.model_list[5].litellm_params.model | string | `"moonshot/kimi-k2.6"` |  |
| litellm.model_list[5].model_name | string | `"moonshotai/kimi-k2.6"` |  |
| litellm.model_list[6].litellm_params.api_key | string | `"os.environ/DEEPSEEK_API_KEY"` |  |
| litellm.model_list[6].litellm_params.model | string | `"deepseek/deepseek-v4-flash"` |  |
| litellm.model_list[6].model_name | string | `"deepseek-v4-flash"` |  |
| litellm.model_list[7].litellm_params.api_key | string | `"os.environ/DEEPSEEK_API_KEY"` |  |
| litellm.model_list[7].litellm_params.model | string | `"deepseek/deepseek-v4-pro"` |  |
| litellm.model_list[7].model_name | string | `"deepseek-v4-pro"` |  |
| litellm.model_list[8].litellm_params.api_key | string | `"os.environ/ZAI_API_KEY"` |  |
| litellm.model_list[8].litellm_params.model | string | `"zai/glm-4.7"` |  |
| litellm.model_list[8].model_name | string | `"zai/glm-4.7"` |  |
| litellm.model_list[9].litellm_params.api_key | string | `"os.environ/ZAI_API_KEY"` |  |
| litellm.model_list[9].litellm_params.model | string | `"zai/glm-4.7-flash"` |  |
| litellm.model_list[9].model_name | string | `"zai/glm-4.7-flash"` |  |
| litellm.resources.limits.cpu | string | `"2000m"` |  |
| litellm.resources.limits.memory | string | `"2Gi"` |  |
| litellm.resources.requests.cpu | string | `"100m"` |  |
| litellm.resources.requests.memory | string | `"256Mi"` |  |
| litellm.service.port | int | `4000` |  |
| namespace | string | `"openagent"` |  |
| postgres.clusterName | string | `"openagent-pg"` |  |
| postgres.configName | string | `"openagent-pg-config"` |  |
| postgres.enabled | bool | `true` |  |
| postgres.instances | int | `1` |  |
| postgres.poolingConfigName | string | `"openagent-pg-pooling"` |  |
| postgres.profile | string | `"development"` |  |
| postgres.storage | string | `"5Gi"` |  |
| postgres.version | string | `"18"` |  |
| storageClass | string | `"local-path"` |  |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
