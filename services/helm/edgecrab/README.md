# edgecrab

![Version: 0.3.0](https://img.shields.io/badge/Version-0.3.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.10.0](https://img.shields.io/badge/AppVersion-0.10.0-informational?style=flat-square)

OpenClaw AI orchestrator — EdgeCrab-powered, combining the best of OpenClaw and Hermes agent in a resource-light Rust binary (~49MB).

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| local |  |  |

## Under the hood

This chart deploys [EdgeCrab](https://github.com/raphaelmansuy/edgecrab) — a Rust-native AI orchestrator that combines:

- **OpenClaw** — 17 messaging gateways, always-on presence
- **Hermes Agent** — autonomous reasoning, persistent memory, user-first alignment

### Key Features

- **16 LLM Providers** — Anthropic, OpenAI, z.ai, DeepSeek, Moonshot, OpenCode, and more
- **17 Messaging Gateways** — WhatsApp, Telegram, Discord, Slack, Signal, Matrix, and more
- **Sub-agent Delegation** — Spawn specialized agents for parallel task execution
- **Persistent Memory** — Cross-session learning via MEMORY.md and Honcho integration
- **Skills Library** — Reusable agent procedures with bundled helper files
- **LSP Integration** — Semantic code intelligence for file operations
- **VPA Scaling** — Vertical Pod Autoscaler for single-pod resource optimization

### Doppler config

The ExternalSecret pulls the entire `svc_edgecrab` Doppler config. Required keys:

| Secret | Purpose |
|--------|---------|
| `ZAI_API_KEY` | z.ai provider (ultrabrain) |
| `ANTHROPIC_API_KEY` | Claude provider (fallback) |
| `DEEPSEEK_API_KEY` | DeepSeek provider |
| `MOONSHOT_API_KEY` | Moonshot / Kimi provider |
| `OPENCODE_API_KEY` | OpenCode Zen provider |
| `WHATSAPP_ALLOW_FROM` | Allowed WhatsApp sender |

### WhatsApp Setup

After the pod starts, exec in and run:

```bash
edgecrab whatsapp      # launches QR code scanner wizard
# Scan with your phone — session persists across restarts
```

The WhatsApp gateway uses Baileys bridge (local Node subprocess) for message delivery.

### Access

EdgeCrab runs as a headless gateway service — no web UI. Interaction happens via messaging platforms (WhatsApp, Telegram, etc.). The gateway binds to `0.0.0.0:8642` internally for health probes and optional API access.

### VPA (Vertical Pod Autoscaler)

This chart uses VPA instead of HPA for single-pod scaling. VPA automatically adjusts CPU and memory requests based on actual workload.

**Prerequisites**: VPA must be installed on the cluster. Install via:

```bash
kubectl apply -f https://github.com/kubernetes/autoscaler/releases/latest/download/vertical-pod-autoscaler.yaml
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| config.agent.system_prompt | string | (see values.yaml) | Agent system prompt |
| config.checkpoints.enabled | bool | `true` | Enable conversation checkpoints |
| config.checkpoints.max_snapshots | int | `50` | Maximum checkpoints to retain |
| config.compression.protect_last_n | int | `20` | Messages to protect from compression |
| config.compression.threshold | float | `0.5` | Context usage threshold for compression |
| config.delegation.enabled | bool | `true` | Enable sub-agent delegation |
| config.delegation.max_iterations | int | `50` | Max iterations for sub-agents |
| config.delegation.max_subagents | int | `3` | Max concurrent sub-agents |
| config.display.activity_shelf | bool | `true` | Show live activity in gateway |
| config.gateway.api_server.enabled | bool | `true` | Enable HTTP API server |
| config.gateway.api_server.port | int | `8642` | API server port |
| config.gateway.host | string | `"0.0.0.0"` | Gateway bind host |
| config.gateway.homeassistant.enabled | bool | `false` | Enable Home Assistant integration |
| config.gateway.port | int | `8642` | Gateway port |
| config.gateway.whatsapp.allowed_users | list | `["${WHATSAPP_ALLOW_FROM}"]` | Allowed WhatsApp users |
| config.gateway.whatsapp.enabled | bool | `true` | Enable WhatsApp gateway |
| config.gateway.whatsapp.mode | string | `"bridge"` | WhatsApp mode (bridge) |
| config.gateway.whatsapp.reply_prefix | string | `null` | Optional reply prefix |
| config.gateway.whatsapp.self_chat_mode | bool | `true` | Allow self-chat |
| config.gateway.whatsapp.send_read_receipts | bool | `true` | Send read receipts |
| config.lsp.enabled | bool | `true` | Enable LSP integration |
| config.logging.level | string | `"info"` | Log level |
| config.memory.enabled | bool | `true` | Enable persistent memory |
| config.memory.write_approval | bool | `false` | Require approval for memory writes |
| config.model.default | string | `"zai/glm-5.2"` | Default model (ultrabrain) |
| config.model.fallback.api_key_env | string | `"ANTHROPIC_API_KEY"` | Fallback provider API key env var |
| config.model.fallback.model | string | `"claude-opus-4-6"` | Fallback model |
| config.model.fallback.provider | string | `"anthropic"` | Fallback provider |
| config.model.max_iterations | int | `90` | Max ReAct loop iterations |
| config.model.prompt_caching | bool | `true` | Enable prompt caching |
| config.model.streaming | bool | `true` | Enable streaming responses |
| config.skills.inline_shell | bool | `false` | Allow inline shell in skills |
| config.skills.write_approval | bool | `false` | Require approval for skill writes |
| config.tools.max_parallel_workers | int | `8` | Max parallel tool workers |
| config.tools.parallel_execution | bool | `true` | Enable parallel tool execution |
| dopplerConfig | string | `""` | Doppler config name; set by ArgoCD appset |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy |
| image.repository | string | `"ghcr.io/raphaelmansuy/edgecrab"` | Image repository |
| image.tag | string | `"0.10.0"` | Image tag |
| resources.limits.cpu | string | `"2000m"` | CPU limit |
| resources.limits.memory | string | `"2Gi"` | Memory limit |
| resources.requests.cpu | string | `"200m"` | CPU request |
| resources.requests.memory | string | `"512Mi"` | Memory request |
| service.port | int | `8642` | Service port |
| storage.accessMode | string | `"ReadWriteOnce"` | PVC access mode |
| storage.size | string | `"5Gi"` | PVC size |
| storage.storageClass | string | `""` | Storage class (empty = default) |
| vpa.controlledResources | list | `["cpu","memory"]` | Resources VPA controls |
| vpa.enabled | bool | `true` | Enable VPA |
| vpa.maxAllowed.cpu | string | `"4000m"` | Max CPU VPA can allocate |
| vpa.maxAllowed.memory | string | `"4Gi"` | Max memory VPA can allocate |
| vpa.minAllowed.cpu | string | `"100m"` | Min CPU VPA can allocate |
| vpa.minAllowed.memory | string | `"256Mi"` | Min memory VPA can allocate |
| vpa.updatePolicy.updateMode | string | `"Auto"` | VPA update mode |
