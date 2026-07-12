# openagent-discord

![Version: 0.3.1](https://img.shields.io/badge/Version-0.3.1-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.3.1](https://img.shields.io/badge/AppVersion-0.3.1-informational?style=flat-square)

Framework-agnostic Discord bot that routes messages to an OpenAI-compatible chat completions API. Supports two modes: chat completions (default, OpenAI-compatible) and direct Kubernetes CRD creation.

## Under the hood

### Chart structure

Custom chart — no upstream dependencies. All templates are local.

| Template | Purpose |
|----------|---------|
| `deployment.yaml` | Bot Deployment — all secrets via `envFrom: secretRef` |
| `serviceaccount.yaml` | ServiceAccount for K8s API access |
| `clusterrole.yaml` | RBAC for AgentRun CRUD + pod log access (only when `agentRef` is set) |
| `externalsecret.yaml` | Pulls all keys from Doppler `svc_openagent` into `openagent-discord-secrets` |
| `service.yaml` | ClusterIP for `/healthz` probes and `/notify` agent callbacks |

### Agent backend selection

The bot selects its backend at startup based on which env vars are set:

```
AGENT_API_URL set?      → OpenAI-compatible chat completions mode (POST /v1/chat/completions)
AGENT_REF set?          → K8s API mode (creates AgentRun CRDs, e.g. Sympozium)
Neither set?            → fatal: one backend required
Both set?               → chat completions takes priority
```

In **chat completions mode**, the bot sends OpenAI-compatible `POST /v1/chat/completions` requests and parses JSON responses. No RBAC needed.

In **K8s API mode**, the bot creates `AgentRun` CRs via the Kubernetes API using its service account token. It polls every 5 seconds (max 5 minutes) and reads the completed job pod's logs for the response. Requires the ClusterRole granted by `clusterrole.yaml`.

### Secrets chain

```
Doppler (svc_openagent)
  → ExternalSecret (ghcr-pull-secret)      ← GITHUB_TOKEN for image pull
  → ExternalSecret (openagent-discord-secrets)  ← pulls ALL keys including:
      DISCORD_BOT_TOKEN      → explicit secretKeyRef env
      DISCORD_BOT_CLIENT_ID  → explicit secretKeyRef env
      AGENT_API_URL          → through envFrom (chat completions mode)
      AGENT_API_KEY          → through envFrom (chat completions API auth)
```

### Setup

| Aspect | Detail |
|--------|--------|
| **Namespace** | `openagent` |
| **Sync wave** | 3 (after external-secrets at wave 1, openagent services at wave 2) |
| **Doppler config** | `svc_openagent` — must contain `DISCORD_BOT_TOKEN`, `DISCORD_BOT_CLIENT_ID`, `AGENT_API_URL`, `AGENT_API_KEY` |
| **Image** | `ghcr.io/jomakori/gke_gitops/openagent-discord-bot:0.3.1` (arm64) |
| **Ingress** | None — bot connects outbound to Discord, no inbound traffic needed |
| **RBAC** | ClusterRole auto-created when `agentRef` is set |

### Discord requirements

- Bot must be invited to the target guild with `309237645920` permissions (send messages, create threads, read message history, embed links)
- `DISCORD_MENTION_ONLY=true` — users must @mention the bot (strip mention prefix before forwarding to agent)
- `DISCORD_CONVERSATION_MODE=threaded` — creates a new thread per user conversation in guild channels

### Agent notify endpoint

Agents can POST status updates to the bot at `/notify`:

```
POST http://openagent-discord.openagent.svc.cluster.local:8080/notify
Content-Type: application/json

{"thread_id": "...", "status": "working", "agent": "sisyphus", "message": "planning approach"}
```

The bot forwards these as formatted Discord messages to the conversation thread.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `dopplerConfig` | string | `""` | Doppler config name. Set by ArgoCD appset. Creates ExternalSecrets when present. |
| `image.repository` | string | `ghcr.io/jomakori/gke_gitops/openagent-discord-bot` | Container image |
| `image.tag` | string | `"0.3.1"` | Image tag |
| `image.pullPolicy` | string | `IfNotPresent` | Image pull policy |
| `service.port` | int | `8080` | Healthz + notify port |
| `resources.requests.cpu` | string | `50m` | CPU request |
| `resources.requests.memory` | string | `128Mi` | Memory request |
| `resources.limits.cpu` | string | `500m` | CPU limit |
| `resources.limits.memory` | string | `512Mi` | Memory limit |
| `config.mentionOnly` | bool | `false` | Only respond to @mentions |
| `config.channelOnly` | string | `""` | Restrict to Discord channel IDs (comma-separated, empty = all) |
| `config.conversationMode` | string | `"threaded"` | `threaded` (guild threads) or `dm` |
| `config.phaseUpdates` | bool | `true` | Post status updates during agent processing |
| `config.pollUI` | bool | `true` | Render polls for ambiguous questions |
| `config.startupChannel` | string | `"chat"` | Channel name or ID for online announcement |
| `config.clientId` | string | `""` | Discord application client ID (for invite URL) |
