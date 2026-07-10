# Discord Agent Bot

Framework-agnostic Discord bot that routes messages to an agent backend.
Supports two modes: A2A protocol (HTTP/SSE) and Kubernetes CRD-based agents.

## Image

`ghcr.io/jomakori/gke_gitops/openagent-discord-bot`

Scratch-based (~7MB). Uses in-cluster service account for K8s API access (no external auth needed).
Requires `ghcr-pull-secret` for private registry pull (see chart's Externalsecret).

## Modes

One of the two backends must be configured. If both are set, A2A takes priority.

### A2A Protocol Mode

For orchestrators that expose an A2A (Agent-to-Agent) endpoint over HTTP/SSE (e.g. kagent).

Env: `AGENT_A2A_URL` — full URL of the A2A endpoint.

```yaml
config:
  agentA2aUrl: "http://agent-api.namespace.svc.cluster.local:8080"
```

### Kubernetes API Mode

For CRD-based orchestrators (e.g. Sympozium). Bot creates AgentRun resources directly via the Kubernetes API
using its service account token. Responses are read from the completed job pod's logs.

| Env | Description |
|-----|-------------|
| `AGENT_REF` | Agent instance name (e.g. `omo-loop-engineering-sisyphus`) |
| `AGENT_ID` | Agent identifier — format depends on orchestrator |
| `AGENT_MODEL` | Model name for agent runs |
| `AGENT_MODEL_PROVIDER` | Provider (e.g. `deepseek`, `openai`) |
| `AGENT_NAMESPACE` | Namespace for CRD creation (default: `default`) |
| `AGENT_SKILLS` | Comma-separated skill pack refs (optional) |

```yaml
config:
  agentRef: "omo-loop-engineering-sisyphus"
  agentId: "stimulus-omo-loop-engineering"
  agentModel: "deepseek-v4-flash"
  agentModelProvider: "deepseek"
  agentNamespace: "sympozium-system"
  agentSkills: "k8s-ops,omo-core-skills,hashline-editor,memory"
```

#### RBAC

The chart includes a ClusterRole + ClusterRoleBinding when `agentRef` is set:

```yaml
rules:
  - apiGroups: ["sympozium.ai"]
    resources: ["agentruns"]
    verbs: ["create", "get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get"]
```

#### Polling

Bot polls AgentRun status every 5 seconds (max 5 minutes). On `Succeeded` it reads the job pod logs.
On `Failed` it returns the error from `status.error`.

## Discord Config

| Env | Default | Description |
|-----|---------|-------------|
| `DISCORD_BOT_TOKEN` | — | Bot token (required) |
| `DISCORD_BOT_CLIENT_ID` | — | Application client ID |
| `DISCORD_MENTION_ONLY` | `true` | Only respond to @mentions |
| `DISCORD_CONVERSATION_MODE` | `threaded` | `threaded` (guild threads) or `dm` (direct messages) |
| `DISCORD_CHANNEL_ONLY` | — | Restrict to channel IDs (comma-separated) |
| `DISCORD_STARTUP_CHANNEL` | — | Channel name or ID for online announcement |
| `DISCORD_PHASE_UPDATES` | `true` | Post status updates during processing |
| `DISCORD_POLL_UI` | `true` | Render polls for ambiguous questions |

Defaults shown are the Helm chart defaults. The Go binary defaults to `false` for
`PHASE_UPDATES` and `POLL_UI` when env is unset.

## Conversation Model

Messages from the same user in the same thread share a `sessionKey`. In K8s API mode
the thread ID is passed as `sessionKey` so the orchestrator can maintain context across
turns. In A2A mode, the thread ID is sent as `contextId`.

Bot creates a new Discord thread per conversation (in `threaded` mode) and keeps track
of thread→user mappings for the `/notify` endpoint.

## Agent Status Updates

Agents can POST status updates to `http://<bot>:8080/notify`:

```json
{
  "thread_id": "...",
  "status": "working",
  "agent": "sisyphus",
  "message": "classifying intent"
}
```

The bot forwards these as formatted messages to the Discord thread.

## Build

```bash
make build              # local binary
make image              # Docker image (linux/arm64)
make push               # push to ghcr.io
make bump TYPE=fix      # bump version + update chart values.yaml
make release TYPE=feat  # bump + build + push
```

## Flow

```
Discord message → bot → [A2A endpoint] or [K8s API AgentRun] → agent → response → bot → Discord thread
                                                                      └─ /notify ←─ status updates
```
