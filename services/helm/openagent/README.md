# openagent

![Version: 2.1.0](https://img.shields.io/badge/Version-2.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.0.0](https://img.shields.io/badge/AppVersion-1.0.0-informational?style=flat-square)
Umbrella chart for the openagent stack — LiteLLM gateway, Hermes Agent, Claude proxy, and supporting infrastructure.

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| local |  |  |

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| file://charts/claude-proxy | claude-proxy | 0.1.0 |
| file://charts/hermes-workspace | hermes-workspace | 0.1.0 |
| oci://ghcr.io/berriai | litellm(litellm-helm) | 1.92.0 |
| oci://ghcr.io/jyje/hermes-agent-helm | hermes-agent | 0.9.1 |

## Skill Single-Source: k8s-gitops-context

The `k8s-gitops-context` skill body is owned here, in `templates/skills/k8s-gitops-context.yaml`, and rendered for two runtimes via the `runtimeMode` value.

| `runtimeMode` | Rendered For | Path Context | Repo Pair Variant | `Repo Locations` |
|---------------|--------------|--------------|-------------------|---------------------|
| `cluster` (default) | In-cluster ConfigMap → hermes workspace memory | `/workspace/repos/...` | GitHub MCP (`github_*` tools) | cluster refs (Doppler/GHCR/Terraform Cloud/Dockerfile), no local abs paths |
| `local` | opencode workstation (`~/.config/opencode/skills/k8s-gitops-context/SKILL.md`) | `/Users/maklab/...` | `## Repo Pair — LOCAL PATHS` | full refs + local abs paths |

**GitOps repo is the single canonical source.** Edit the skill body in this template; the cluster ConfigMap is rendered by ArgoCD during sync (`runtimeMode: cluster`), and the local opencode workstation copy is a **generated artifact** — never hand-edited.

Regenerate the local copy from the gitops source:

```bash
./.useful-scripts/render_local_skill.sh
```

The script runs `helm template --set runtimeMode=local`, extracts `data.k8s-gitops-context.md`, prepends the opencode frontmatter, and writes `~/.config/opencode/skills/k8s-gitops-context/SKILL.md`. It enforces a sanity check (local paths present, cluster-only GitHub MCP content absent) and fails loudly on render errors.

### Templating gotcha

The skill body is Helm-templated — it carries `runtimeMode` conditionals around the two path-context regions. Because Helm processes the body as a template, any literal Go-template braces you want to DISPLAY inside the skill text must use Helm's `escapeBrace` helper (`lit.Brace` / the two-brace escape idiom). Keep in mind: this README's generated section is itself rendered through helm-docs, so brace-showing examples here are deliberately shown in plain words rather than as raw brace tokens.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| claude-proxy.enabled | bool | `true` |  |
| clusterDomain | string | `"maklab.net"` |  |
| dashboard.destination.host | string | `"openagent-hermes-workspace.openagent.svc.cluster.local"` |  |
| dashboard.destination.port | int | `3000` |  |
| dashboard.subdomain | string | `"openagent"` |  |
| dopplerConfig | string | `"svc_openagent"` |  |
| ghcrPullSecret | string | `""` |  |
| hermes-agent.command[0] | string | `"sh"` |  |
| hermes-agent.command[1] | string | `"-c"` |  |
| hermes-agent.command[2] | string | `"npm install -g @bitwarden/cli 2>&1\nexec /init hermes gateway run\n"` |  |
| hermes-agent.config.agent.system_prompt | string | `"You are Sisyphus — OMO Orchestrator. You field ALL prompts.\nClassify every request BEFORE acting.\n\n## Classification\n\n- TRIVIAL (typo, single config, known pattern): Answer directly. No delegation.\n- STANDARD (new feature, refactor, multi-file): Route through planning pipeline.\n- COMPLEX (architecture, cross-system, security): Full pipeline with review gates.\n\n## Delegation Pipeline (Standard + Complex)\n\n1. Assess context: is the request clear and unambiguous?\n   → NO: Ask ONE clarifying question first.\n   → YES: Proceed.\n\n2. Standard: Build a plan → present for USER APPROVAL → wait for \"go\" / \"approved\".\n   Complex: Analyze (Metis) → Architect (Oracle) → Plan (Prometheus) → Review (Momus)\n   → present for USER APPROVAL.\n\n3. NEVER execute Standard/Complex work without explicit user sign-off.\n\n## Approval Gates (BLOCK these without asking)\n\n- merge / commit\n- publish / deploy / push\n- destructive (delete, teardown, drop)\n- external-send (email, API, webhook)\n\n## Style\n\n- Terse. Caveman mode. Drop articles and filler.\n- ALWAYS verbalize your classification: \"Classified as [tier].\"\n- Show your work. Tell user what you're doing.\n- When delegating: \"Delegating to [persona] for [task].\"\n"` |  |
| hermes-agent.config.auxiliary.vision.model | string | `"claude/sonnet-5"` |  |
| hermes-agent.config.auxiliary.vision.provider | string | `"litellm"` |  |
| hermes-agent.config.delegation.api_key | string | `"${LITELLM_MASTER_KEY}"` |  |
| hermes-agent.config.delegation.base_url | string | `"http://openagent-litellm.openagent.svc.cluster.local:4000/v1"` |  |
| hermes-agent.config.delegation.max_iterations | int | `30` |  |
| hermes-agent.config.delegation.model | string | `"claude/sonnet-5"` |  |
| hermes-agent.config.delegation.provider | string | `"litellm"` |  |
| hermes-agent.config.delegation.reasoning_effort | string | `"medium"` |  |
| hermes-agent.config.mcp_servers.argocd.args[0] | string | `"-y"` |  |
| hermes-agent.config.mcp_servers.argocd.args[1] | string | `"argocd-mcp@latest"` |  |
| hermes-agent.config.mcp_servers.argocd.args[2] | string | `"stdio"` |  |
| hermes-agent.config.mcp_servers.argocd.command | string | `"npx"` |  |
| hermes-agent.config.mcp_servers.argocd.env.ARGOCD_API_TOKEN | string | `"${MCP_ARGOCD_TOKEN}"` |  |
| hermes-agent.config.mcp_servers.argocd.env.ARGOCD_BASE_URL | string | `"${MCP_ARGOCD_URL}"` |  |
| hermes-agent.config.mcp_servers.argocd.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.argocd.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.bitwarden.args[0] | string | `"-c"` |  |
| hermes-agent.config.mcp_servers.bitwarden.args[1] | string | `"BW_CLIENTID=$BW_CLIENTID BW_CLIENTSECRET=$BW_CLIENTSECRET bw login --apikey 2>/dev/null\nexport BW_SESSION=$(BW_PASSWORD=$BW_PASSWORD bw unlock --passwordenv BW_PASSWORD 2>/dev/null | grep \"BW_SESSION=\" | sed \"s/.*BW_SESSION=\\\"//;s/\\\".*//\")\nexec npx -y @bitwarden/mcp-server\n"` |  |
| hermes-agent.config.mcp_servers.bitwarden.command | string | `"sh"` |  |
| hermes-agent.config.mcp_servers.bitwarden.env.BW_CLIENTID | string | `"${BW_CLIENTID}"` |  |
| hermes-agent.config.mcp_servers.bitwarden.env.BW_CLIENTSECRET | string | `"${BW_CLIENTSECRET}"` |  |
| hermes-agent.config.mcp_servers.bitwarden.env.BW_PASSWORD | string | `"${BW_PASSWORD}"` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.doppler.args[0] | string | `"-y"` |  |
| hermes-agent.config.mcp_servers.doppler.args[1] | string | `"@dopplerhq/mcp-server"` |  |
| hermes-agent.config.mcp_servers.doppler.command | string | `"npx"` |  |
| hermes-agent.config.mcp_servers.doppler.env.DOPPLER_TOKEN | string | `"${MCP_DOPPLER_TOKEN}"` |  |
| hermes-agent.config.mcp_servers.doppler.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.doppler.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.ferryhopper.timeout | int | `60` |  |
| hermes-agent.config.mcp_servers.ferryhopper.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.ferryhopper.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.ferryhopper.url | string | `"https://mcp.ferryhopper.com/mcp"` |  |
| hermes-agent.config.mcp_servers.github.args[0] | string | `"-y"` |  |
| hermes-agent.config.mcp_servers.github.args[1] | string | `"@modelcontextprotocol/server-github"` |  |
| hermes-agent.config.mcp_servers.github.command | string | `"npx"` |  |
| hermes-agent.config.mcp_servers.github.env.GITHUB_PERSONAL_ACCESS_TOKEN | string | `"${MCP_GITHUB_TOKEN}"` |  |
| hermes-agent.config.mcp_servers.github.env.npm_config_loglevel | string | `"error"` |  |
| hermes-agent.config.mcp_servers.github.tools.include[0] | string | `"list_issues"` |  |
| hermes-agent.config.mcp_servers.github.tools.include[1] | string | `"create_issue"` |  |
| hermes-agent.config.mcp_servers.github.tools.include[2] | string | `"update_issue"` |  |
| hermes-agent.config.mcp_servers.github.tools.include[3] | string | `"search_code"` |  |
| hermes-agent.config.mcp_servers.github.tools.include[4] | string | `"search_repositories"` |  |
| hermes-agent.config.mcp_servers.github.tools.include[5] | string | `"get_file_contents"` |  |
| hermes-agent.config.mcp_servers.github.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.github.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.google-workspace.args[0] | string | `"workspace-mcp"` |  |
| hermes-agent.config.mcp_servers.google-workspace.args[1] | string | `"--tool-tier"` |  |
| hermes-agent.config.mcp_servers.google-workspace.args[2] | string | `"complete"` |  |
| hermes-agent.config.mcp_servers.google-workspace.command | string | `"uvx"` |  |
| hermes-agent.config.mcp_servers.google-workspace.env.GOOGLE_OAUTH_CLIENT_ID | string | `"${GOOGLE_OAUTH_CLIENT_ID}"` |  |
| hermes-agent.config.mcp_servers.google-workspace.env.GOOGLE_OAUTH_CLIENT_SECRET | string | `"${GOOGLE_OAUTH_CLIENT_SECRET}"` |  |
| hermes-agent.config.mcp_servers.grafana.args[0] | string | `"-y"` |  |
| hermes-agent.config.mcp_servers.grafana.args[1] | string | `"@leval/mcp-grafana"` |  |
| hermes-agent.config.mcp_servers.grafana.command | string | `"npx"` |  |
| hermes-agent.config.mcp_servers.grafana.env.GRAFANA_SERVICE_ACCOUNT_TOKEN | string | `"${MCP_GRAFANA_TOKEN}"` |  |
| hermes-agent.config.mcp_servers.grafana.env.GRAFANA_URL | string | `"${MCP_GRAFANA_URL}"` |  |
| hermes-agent.config.mcp_servers.grafana.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.grafana.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.kiwi.timeout | int | `60` |  |
| hermes-agent.config.mcp_servers.kiwi.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.kiwi.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.kiwi.url | string | `"https://mcp.kiwi.com"` |  |
| hermes-agent.config.mcp_servers.kubectl.timeout | int | `60` |  |
| hermes-agent.config.mcp_servers.kubectl.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.kubectl.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.kubectl.url | string | `"http://kubernetes-mcp-server.openagent.svc.cluster.local:8000/mcp"` |  |
| hermes-agent.config.mcp_servers.skiplagged.timeout | int | `60` |  |
| hermes-agent.config.mcp_servers.skiplagged.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.skiplagged.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.skiplagged.url | string | `"https://mcp.skiplagged.com/mcp"` |  |
| hermes-agent.config.model.default | string | `"deepseek-v4-flash"` |  |
| hermes-agent.config.model.provider | string | `"litellm"` |  |
| hermes-agent.config.plugins.enabled[0] | string | `"discord-platform"` |  |
| hermes-agent.config.providers.litellm.base_url | string | `"http://openagent-litellm.openagent.svc.cluster.local:4000/v1"` |  |
| hermes-agent.config.providers.litellm.discover_models | bool | `true` |  |
| hermes-agent.config.providers.litellm.key_env | string | `"LITELLM_MASTER_KEY"` |  |
| hermes-agent.env | object | `{}` |  |
| hermes-agent.extraEnvFrom[0].secretRef.name | string | `"openagent-secrets"` |  |
| hermes-agent.extraEnv[0].name | string | `"DISCORD_BOT_TOKEN"` |  |
| hermes-agent.extraEnv[0].valueFrom.secretKeyRef.key | string | `"DISCORD_BOT_TOKEN"` |  |
| hermes-agent.extraEnv[0].valueFrom.secretKeyRef.name | string | `"openagent-secrets"` |  |
| hermes-agent.extraEnv[10].name | string | `"API_SERVER_KEY"` |  |
| hermes-agent.extraEnv[10].value | string | `"hermes-api-server-key-1234"` |  |
| hermes-agent.extraEnv[11].name | string | `"API_SERVER_HOST"` |  |
| hermes-agent.extraEnv[11].value | string | `"0.0.0.0"` |  |
| hermes-agent.extraEnv[12].name | string | `"API_SERVER_PORT"` |  |
| hermes-agent.extraEnv[12].value | string | `"8642"` |  |
| hermes-agent.extraEnv[13].name | string | `"API_SERVER_CORS_ORIGINS"` |  |
| hermes-agent.extraEnv[13].value | string | `"https://openagent.maklab.net"` |  |
| hermes-agent.extraEnv[14].name | string | `"DASHBOARD_BASE_URL"` |  |
| hermes-agent.extraEnv[14].value | string | `"https://openagent.maklab.net"` |  |
| hermes-agent.extraEnv[1].name | string | `"DISCORD_BOT_CLIENT_ID"` |  |
| hermes-agent.extraEnv[1].valueFrom.secretKeyRef.key | string | `"DISCORD_BOT_CLIENT_ID"` |  |
| hermes-agent.extraEnv[1].valueFrom.secretKeyRef.name | string | `"openagent-secrets"` |  |
| hermes-agent.extraEnv[2].name | string | `"HERMES_DASHBOARD"` |  |
| hermes-agent.extraEnv[2].value | string | `"1"` |  |
| hermes-agent.extraEnv[3].name | string | `"HERMES_DASHBOARD_HOST"` |  |
| hermes-agent.extraEnv[3].value | string | `"0.0.0.0"` |  |
| hermes-agent.extraEnv[4].name | string | `"HERMES_DASHBOARD_PORT"` |  |
| hermes-agent.extraEnv[4].value | string | `"9119"` |  |
| hermes-agent.extraEnv[5].name | string | `"HERMES_DASHBOARD_BASIC_AUTH_USERNAME"` |  |
| hermes-agent.extraEnv[5].valueFrom.secretKeyRef.key | string | `"HERMES_DASHBOARD_BASIC_AUTH_USERNAME"` |  |
| hermes-agent.extraEnv[5].valueFrom.secretKeyRef.name | string | `"openagent-secrets"` |  |
| hermes-agent.extraEnv[6].name | string | `"HERMES_DASHBOARD_BASIC_AUTH_PASSWORD"` |  |
| hermes-agent.extraEnv[6].valueFrom.secretKeyRef.key | string | `"HERMES_DASHBOARD_BASIC_AUTH_PASSWORD"` |  |
| hermes-agent.extraEnv[6].valueFrom.secretKeyRef.name | string | `"openagent-secrets"` |  |
| hermes-agent.extraEnv[7].name | string | `"HERMES_DASHBOARD_BASIC_AUTH_SECRET"` |  |
| hermes-agent.extraEnv[7].valueFrom.secretKeyRef.key | string | `"HERMES_DASHBOARD_BASIC_AUTH_SECRET"` |  |
| hermes-agent.extraEnv[7].valueFrom.secretKeyRef.name | string | `"openagent-secrets"` |  |
| hermes-agent.extraEnv[8].name | string | `"DISCORD_ALLOW_ALL_USERS"` |  |
| hermes-agent.extraEnv[8].value | string | `"true"` |  |
| hermes-agent.extraEnv[9].name | string | `"API_SERVER_ENABLED"` |  |
| hermes-agent.extraEnv[9].value | string | `"true"` |  |
| hermes-agent.extraVolumeMounts[0].mountPath | string | `"/opt/data/hooks/discord-session-link"` |  |
| hermes-agent.extraVolumeMounts[0].name | string | `"hermes-hooks"` |  |
| hermes-agent.extraVolumeMounts[0].readOnly | bool | `true` |  |
| hermes-agent.extraVolumeMounts[1].mountPath | string | `"/opt/data/memories/k8s-gitops-context.md"` |  |
| hermes-agent.extraVolumeMounts[1].name | string | `"k8s-gitops-context"` |  |
| hermes-agent.extraVolumeMounts[1].readOnly | bool | `true` |  |
| hermes-agent.extraVolumeMounts[1].subPath | string | `"k8s-gitops-context.md"` |  |
| hermes-agent.extraVolumes[0].configMap.name | string | `"openagent-hermes-hooks"` |  |
| hermes-agent.extraVolumes[0].name | string | `"hermes-hooks"` |  |
| hermes-agent.extraVolumes[1].configMap.name | string | `"openagent-k8s-gitops-context"` |  |
| hermes-agent.extraVolumes[1].name | string | `"k8s-gitops-context"` |  |
| hermes-agent.service.enabled | bool | `true` |  |
| hermes-agent.service.port | int | `9119` |  |
| hermes-workspace.enabled | bool | `true` |  |
| hermes.enabled | bool | `true` |  |
| litellm.db.database | string | `"litellm"` |  |
| litellm.db.deployStandalone | bool | `false` |  |
| litellm.db.endpoint | string | `"openagent-pg.openagent.svc.cluster.local"` |  |
| litellm.db.secret.name | string | `"openagent-litellm-secrets"` |  |
| litellm.db.secret.passwordKey | string | `"OPENAGENT_PG_PASSWORD"` |  |
| litellm.db.secret.usernameKey | string | `"OPENAGENT_PG_USER"` |  |
| litellm.db.useExisting | bool | `true` |  |
| litellm.enabled | bool | `true` |  |
| litellm.environmentSecrets[0] | string | `"openagent-secrets"` |  |
| litellm.fullnameOverride | string | `"openagent-litellm"` |  |
| litellm.image.pullPolicy | string | `"IfNotPresent"` |  |
| litellm.image.repository | string | `"ghcr.io/berriai/litellm"` |  |
| litellm.image.tag | string | `"main-stable"` |  |
| litellm.masterkeySecretKey | string | `"masterkey"` |  |
| litellm.masterkeySecretName | string | `"openagent-litellm-masterkey"` |  |
| litellm.proxy_config.fallbacks[0].opencode/big-pickle[0] | string | `"deepseek-v4-flash"` |  |
| litellm.proxy_config.fallbacks[10].minimax/M3[0] | string | `"deepseek-v4-pro"` |  |
| litellm.proxy_config.fallbacks[10].minimax/M3[1] | string | `"zai/glm-4.7"` |  |
| litellm.proxy_config.fallbacks[11]."minimax/M2.7"[0] | string | `"deepseek-v4-flash"` |  |
| litellm.proxy_config.fallbacks[11]."minimax/M2.7"[1] | string | `"zai/glm-4.7-flash"` |  |
| litellm.proxy_config.fallbacks[12].claude/sonnet-5[0] | string | `"deepseek-v4-pro"` |  |
| litellm.proxy_config.fallbacks[12].claude/sonnet-5[1] | string | `"zai/glm-4.7"` |  |
| litellm.proxy_config.fallbacks[13].claude-sonnet-5[0] | string | `"deepseek-v4-pro"` |  |
| litellm.proxy_config.fallbacks[13].claude-sonnet-5[1] | string | `"zai/glm-4.7"` |  |
| litellm.proxy_config.fallbacks[14].claude/opus-4[0] | string | `"claude/sonnet-5"` |  |
| litellm.proxy_config.fallbacks[14].claude/opus-4[1] | string | `"deepseek-v4-pro"` |  |
| litellm.proxy_config.fallbacks[1].opencode/north-mini-code-free[0] | string | `"deepseek-v4-flash"` |  |
| litellm.proxy_config.fallbacks[2].opencode/deepseek-v4-flash-free[0] | string | `"deepseek-v4-flash"` |  |
| litellm.proxy_config.fallbacks[3]."opencode/mimo-v2.5-free"[0] | string | `"deepseek-v4-flash"` |  |
| litellm.proxy_config.fallbacks[4].anthropic/claude-opus-4-7[0] | string | `"claude/opus-4"` |  |
| litellm.proxy_config.fallbacks[4].anthropic/claude-opus-4-7[1] | string | `"deepseek-v4-pro"` |  |
| litellm.proxy_config.fallbacks[5]."moonshotai/kimi-k2.6"[0] | string | `"deepseek-v4-pro"` |  |
| litellm.proxy_config.fallbacks[5]."moonshotai/kimi-k2.6"[1] | string | `"zai/glm-4.7"` |  |
| litellm.proxy_config.fallbacks[6].deepseek-v4-flash[0] | string | `"zai/glm-4.7-flash"` |  |
| litellm.proxy_config.fallbacks[6].deepseek-v4-flash[1] | string | `"opencode/deepseek-v4-flash-free"` |  |
| litellm.proxy_config.fallbacks[7].deepseek-v4-pro[0] | string | `"zai/glm-4.7"` |  |
| litellm.proxy_config.fallbacks[7].deepseek-v4-pro[1] | string | `"deepseek-v4-flash"` |  |
| litellm.proxy_config.fallbacks[8]."zai/glm-4.7"[0] | string | `"deepseek-v4-pro"` |  |
| litellm.proxy_config.fallbacks[8]."zai/glm-4.7"[1] | string | `"zai/glm-4.7-flash"` |  |
| litellm.proxy_config.fallbacks[9]."zai/glm-4.7-flash"[0] | string | `"deepseek-v4-flash"` |  |
| litellm.proxy_config.fallbacks[9]."zai/glm-4.7-flash"[1] | string | `"opencode/deepseek-v4-flash-free"` |  |
| litellm.proxy_config.general_settings.master_key | string | `"os.environ/LITELLM_MASTER_KEY"` |  |
| litellm.proxy_config.litellm_settings.drop_params | bool | `true` |  |
| litellm.proxy_config.model_list[0].litellm_params.api_base | string | `"os.environ/OPENCODE_API_BASE"` |  |
| litellm.proxy_config.model_list[0].litellm_params.api_key | string | `"os.environ/OPENCODE_API_KEY"` |  |
| litellm.proxy_config.model_list[0].litellm_params.model | string | `"openai/big-pickle"` |  |
| litellm.proxy_config.model_list[0].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[0].model_name | string | `"opencode/big-pickle"` |  |
| litellm.proxy_config.model_list[10].litellm_params.api_base | string | `"os.environ/MINIMAX_API_BASE"` |  |
| litellm.proxy_config.model_list[10].litellm_params.api_key | string | `"os.environ/MINIMAX_API_KEY"` |  |
| litellm.proxy_config.model_list[10].litellm_params.model | string | `"openai/MiniMax-M3"` |  |
| litellm.proxy_config.model_list[10].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[10].model_name | string | `"minimax/M3"` |  |
| litellm.proxy_config.model_list[11].litellm_params.api_base | string | `"os.environ/MINIMAX_API_BASE"` |  |
| litellm.proxy_config.model_list[11].litellm_params.api_key | string | `"os.environ/MINIMAX_API_KEY"` |  |
| litellm.proxy_config.model_list[11].litellm_params.model | string | `"openai/MiniMax-M2.7"` |  |
| litellm.proxy_config.model_list[11].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[11].model_name | string | `"minimax/M2.7"` |  |
| litellm.proxy_config.model_list[12].litellm_params.api_base | string | `"http://192.168.1.64:8317/v1"` |  |
| litellm.proxy_config.model_list[12].litellm_params.api_key | string | `"sk-auth2api-claude-proxy-key"` |  |
| litellm.proxy_config.model_list[12].litellm_params.model | string | `"openai/claude-sonnet-4-6"` |  |
| litellm.proxy_config.model_list[12].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[12].litellm_params.rpm | int | `20` |  |
| litellm.proxy_config.model_list[12].model_name | string | `"claude/sonnet-5"` |  |
| litellm.proxy_config.model_list[13].litellm_params.api_base | string | `"http://192.168.1.64:8317/v1"` |  |
| litellm.proxy_config.model_list[13].litellm_params.api_key | string | `"sk-auth2api-claude-proxy-key"` |  |
| litellm.proxy_config.model_list[13].litellm_params.model | string | `"openai/claude-sonnet-4-6"` |  |
| litellm.proxy_config.model_list[13].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[13].litellm_params.rpm | int | `20` |  |
| litellm.proxy_config.model_list[13].model_name | string | `"claude-sonnet-5"` |  |
| litellm.proxy_config.model_list[14].litellm_params.api_base | string | `"http://192.168.1.64:8317/v1"` |  |
| litellm.proxy_config.model_list[14].litellm_params.api_key | string | `"sk-auth2api-claude-proxy-key"` |  |
| litellm.proxy_config.model_list[14].litellm_params.model | string | `"openai/claude-sonnet-4-6"` |  |
| litellm.proxy_config.model_list[14].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[14].litellm_params.rpm | int | `20` |  |
| litellm.proxy_config.model_list[14].model_name | string | `"gpt-5.5"` |  |
| litellm.proxy_config.model_list[15].litellm_params.api_base | string | `"http://192.168.1.64:8317/v1"` |  |
| litellm.proxy_config.model_list[15].litellm_params.api_key | string | `"sk-auth2api-claude-proxy-key"` |  |
| litellm.proxy_config.model_list[15].litellm_params.model | string | `"openai/claude-opus-4-7"` |  |
| litellm.proxy_config.model_list[15].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[15].litellm_params.rpm | int | `10` |  |
| litellm.proxy_config.model_list[15].model_name | string | `"claude/opus-4"` |  |
| litellm.proxy_config.model_list[16].litellm_params.api_base | string | `"http://192.168.1.64:8317/v1"` |  |
| litellm.proxy_config.model_list[16].litellm_params.api_key | string | `"sk-auth2api-claude-proxy-key"` |  |
| litellm.proxy_config.model_list[16].litellm_params.model | string | `"openai/claude-haiku-4-5-20251001"` |  |
| litellm.proxy_config.model_list[16].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[16].litellm_params.rpm | int | `30` |  |
| litellm.proxy_config.model_list[16].model_name | string | `"claude/haiku-4"` |  |
| litellm.proxy_config.model_list[1].litellm_params.api_base | string | `"os.environ/OPENCODE_API_BASE"` |  |
| litellm.proxy_config.model_list[1].litellm_params.api_key | string | `"os.environ/OPENCODE_API_KEY"` |  |
| litellm.proxy_config.model_list[1].litellm_params.model | string | `"openai/north-mini-code-free"` |  |
| litellm.proxy_config.model_list[1].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[1].litellm_params.use_chat_completions_api | bool | `true` |  |
| litellm.proxy_config.model_list[1].model_name | string | `"opencode/north-mini-code-free"` |  |
| litellm.proxy_config.model_list[2].litellm_params.api_base | string | `"os.environ/OPENCODE_API_BASE"` |  |
| litellm.proxy_config.model_list[2].litellm_params.api_key | string | `"os.environ/OPENCODE_API_KEY"` |  |
| litellm.proxy_config.model_list[2].litellm_params.model | string | `"openai/deepseek-v4-flash-free"` |  |
| litellm.proxy_config.model_list[2].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[2].litellm_params.use_chat_completions_api | bool | `true` |  |
| litellm.proxy_config.model_list[2].model_name | string | `"opencode/deepseek-v4-flash-free"` |  |
| litellm.proxy_config.model_list[3].litellm_params.api_base | string | `"os.environ/OPENCODE_API_BASE"` |  |
| litellm.proxy_config.model_list[3].litellm_params.api_key | string | `"os.environ/OPENCODE_API_KEY"` |  |
| litellm.proxy_config.model_list[3].litellm_params.model | string | `"openai/mimo-v2.5-free"` |  |
| litellm.proxy_config.model_list[3].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[3].model_name | string | `"opencode/mimo-v2.5-free"` |  |
| litellm.proxy_config.model_list[4].litellm_params.api_key | string | `"os.environ/ANTHROPIC_API_KEY"` |  |
| litellm.proxy_config.model_list[4].litellm_params.model | string | `"anthropic/claude-opus-4-7"` |  |
| litellm.proxy_config.model_list[4].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[4].model_name | string | `"anthropic/claude-opus-4-7"` |  |
| litellm.proxy_config.model_list[5].litellm_params.api_key | string | `"os.environ/MOONSHOT_API_KEY"` |  |
| litellm.proxy_config.model_list[5].litellm_params.model | string | `"moonshot/kimi-k2.6"` |  |
| litellm.proxy_config.model_list[5].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[5].model_name | string | `"moonshotai/kimi-k2.6"` |  |
| litellm.proxy_config.model_list[6].litellm_params.api_key | string | `"os.environ/DEEPSEEK_API_KEY"` |  |
| litellm.proxy_config.model_list[6].litellm_params.model | string | `"deepseek/deepseek-v4-flash"` |  |
| litellm.proxy_config.model_list[6].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[6].litellm_params.use_chat_completions_api | bool | `true` |  |
| litellm.proxy_config.model_list[6].model_name | string | `"deepseek-v4-flash"` |  |
| litellm.proxy_config.model_list[7].litellm_params.api_key | string | `"os.environ/DEEPSEEK_API_KEY"` |  |
| litellm.proxy_config.model_list[7].litellm_params.cache | bool | `true` |  |
| litellm.proxy_config.model_list[7].litellm_params.model | string | `"deepseek/deepseek-v4-pro"` |  |
| litellm.proxy_config.model_list[7].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[7].litellm_params.use_chat_completions_api | bool | `true` |  |
| litellm.proxy_config.model_list[7].model_name | string | `"deepseek-v4-pro"` |  |
| litellm.proxy_config.model_list[8].litellm_params.api_key | string | `"os.environ/ZAI_API_KEY"` |  |
| litellm.proxy_config.model_list[8].litellm_params.model | string | `"zai/glm-4.7"` |  |
| litellm.proxy_config.model_list[8].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[8].litellm_params.use_chat_completions_api | bool | `true` |  |
| litellm.proxy_config.model_list[8].model_name | string | `"zai/glm-4.7"` |  |
| litellm.proxy_config.model_list[9].litellm_params.api_key | string | `"os.environ/ZAI_API_KEY"` |  |
| litellm.proxy_config.model_list[9].litellm_params.model | string | `"zai/glm-4.7-flash"` |  |
| litellm.proxy_config.model_list[9].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[9].litellm_params.use_chat_completions_api | bool | `true` |  |
| litellm.proxy_config.model_list[9].model_name | string | `"zai/glm-4.7-flash"` |  |
| litellm.proxy_config.router_settings.allowed_fails | int | `100` |  |
| litellm.proxy_config.router_settings.cooldown_time | int | `0` |  |
| litellm.proxy_config.router_settings.disable_cooldowns | bool | `true` |  |
| litellm.proxy_config.router_settings.num_retries | int | `2` |  |
| litellm.proxy_config.router_settings.request_timeout | int | `180` |  |
| litellm.proxy_config.router_settings.routing_strategy | string | `"least-busy"` |  |
| litellm.resources.limits.cpu | string | `"2000m"` |  |
| litellm.resources.limits.memory | string | `"2Gi"` |  |
| litellm.resources.requests.cpu | string | `"100m"` |  |
| litellm.resources.requests.memory | string | `"256Mi"` |  |
| litellm.service.port | int | `4000` |  |
| litellmVirtualService.destination.host | string | `"openagent-litellm.openagent.svc.cluster.local"` |  |
| litellmVirtualService.destination.port | int | `4000` |  |
| litellmVirtualService.host | string | `"litellm.maklab.net"` |  |
| namespace | string | `"openagent"` |  |
| postgres.clusterName | string | `"openagent-pg"` |  |
| postgres.configName | string | `"openagent-pg-config"` |  |
| postgres.enabled | bool | `true` |  |
| postgres.instances | int | `1` |  |
| postgres.poolingConfigName | string | `"openagent-pg-pooling"` |  |
| postgres.profile | string | `"development"` |  |
| postgres.storage | string | `"5Gi"` |  |
| postgres.version | string | `"18"` |  |
| runtimeMode | string | `"cluster"` |  |
| storageClass | string | `"local-path"` |  |
| vpa.enabled | bool | `true` |  |
