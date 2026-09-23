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
| file://charts/claude-proxy | claude-proxy | 0.2.0 |
| file://charts/hermes-webui | hermes-webui | 0.1.1 |
| oci://ghcr.io/berriai | litellm(litellm-helm) | 1.92.0 |
| oci://ghcr.io/jyje/hermes-agent-helm | hermes-agent | 1.15.0 |

## OpenAgent stack

The umbrella chart that runs the cluster's AI workforce: an LLM gateway, the Hermes Agent gateway, its web UI, and a Claude Pro proxy — plus the CRDs, secrets and routing that tie them together. Work is *loop-engineered*: tasks are decomposed, delegated to specialised personas, reviewed, and iterated rather than answered in a single pass.

### Components

| Component | Deployed Via | Purpose |
|-----------|-------------|---------|
| `openagent-litellm` | remote OCI dep (LiteLLM Helm chart) | Multi-provider LLM gateway — model access only, no fallbacks. |
| `openagent-hermes` | remote OCI dep (Hermes Agent Helm chart) | Hermes Agent gateway — Discord bot + MCP servers. |
| `hermes-webui` | local subchart (`charts/hermes-webui`) | Web dashboard — thin-client gateway mode, CF Access private. |
| `claude-proxy` | local subchart (`charts/claude-proxy`) | Claude Pro subscription proxy — OAuth-based, ClusterIP `:4523`. |
| umbrella templates | local (`templates/`) | OMO agent fleet, skills, StackGres, istio gateway, ExternalSecrets. |

> Chart and image versions are pinned in `Chart.yaml` / `values.yaml` and bumped by Renovate — deliberately not restated here, so this README cannot go stale on a version bump.

### Chart structure

```text
openagent/                       ← umbrella
├── charts/
│   ├── hermes-webui/            ← local subchart (web dashboard)
│   └── claude-proxy/            ← local subchart (Claude Pro proxy)
├── templates/                   ← flat manifests (condensed; no subdirs)
│   ├── apps.yaml                ← dashboard-auth, hermes API svc, litellm VS, responses-proxy
│   ├── db.yaml                  ← StackGres SGScript (cluster owned by postgres-operator chart)
│   ├── hermes.yaml              ← hermes mise config
│   ├── hooks.yaml               ← MCP manifest CM + preflight Job + drift CronJob
│   ├── k8s-gitops-context.yaml  ← skill ConfigMap
│   ├── omo-config.yaml          ← OMO agent fleet (roster, chains, categories)
│   ├── secrets.yaml             ← ExternalSecrets, GHCR pull secret, litellm/pg creds
│   └── vpa.yaml                 ← VerticalPodAutoscaler
├── values.yaml                  ← full config surface
└── Chart.yaml                   ← remote OCI + local subchart deps
```

### Runtime, toolchain & health

The gateway is **build-free in-cluster**: the agent tooling and the MCP verifier come from a prebuilt tools image instead of being compiled on the PVC. The old in-repo Go tree, boot shim and source ConfigMaps are retired; the hermes image and PVC remain, because MCP servers still need the toolchain the gateway pre-warms onto the volume.

- **Boot** — an initContainer copies the verifier binary out of the tools image into an `emptyDir`; the container then runs the boot command, which prepares the toolchain and hands over to the gateway.
- **Verification** — the MCP preflight Job and drift CronJob use the same initContainer + verifier pattern, so verification runs the identical binary and toolchain as the gateway (see [MCP Manifest Verification](#mcp-manifest-verification)).
- **Health** — startup and readiness probes gate the gateway on its dashboard, so the pod is not Ready — and Service endpoints stay empty — until it actually serves.
- **Configuration** — the chart seeds the agent config, including the schema version the pinned image expects, kept in lockstep by the same values that pin the image.
- **Drift signal** — the drift CronJob fails by design when it detects drift; an ArgoCD health override for that CronJob reports it as Healthy-with-message, so a real finding does not mark the app Degraded or block auto-sync. Drift stays visible in the Job logs.

### OMO agent fleet

The AI workforce is an OMO (Oh My OpenAgent) fleet implemented as the `hermes-omo-plugin`, which launches named subagents natively through Hermes' subagent lifecycle — no OpenCode server, no `opencode` tool, no `omo.jsonc`. The roster (agent → model → fallback chains, and category routing) is defined under `hermes-agent.config.plugins.entries.omo.settings` — the path the plugin reads through Hermes' `ctx.get_config`. Model IDs live only in `litellm.proxy_config.model_list`; a model appears exactly once. Chains are the `settings.chains` array (primary-first); runtime fallback on 429/5xx is owned by the plugin, not the gateway.

Agent roles: sisyphus (Sisyphus · Ultraworker, orchestrator), hephaestus (Hephaestus · Deep Agent), prometheus (Prometheus · Plan Builder), atlas (Atlas · Plan Executor), metis (Metis · Plan Consultant), momus (Momus · Plan Critic), oracle (Oracle · Architecture/Reasoning), librarian (Librarian · Research), explore (Explore · Repository Exploration), multimodal-looker (Multimodal-Looker · Multimodal Analysis), sisyphus-junior (Sisyphus-Junior · Specialized Execution Worker). Writing work routes via the `writing` category / sisyphus. Each agent and category carries a primary model plus a fallback chain; Claude is escalation-only.

### Skills

The gateway loads built-in skills, configured via `config.agent.environment_hint`:

| Skill | Source | Purpose |
|-------|--------|---------|
| **ponytail** | [DietrichGebert/ponytail](https://github.com/DietrichGebert/ponytail) | YAGNI ladder — write only what is needed; reuse > rewrite, stdlib > custom |
| **caveman** | [JuliusBrussee/caveman](https://github.com/JuliusBrussee/caveman) | Terse communication — fewer output tokens, drop filler, keep substance |

**Ponytail YAGNI ladder** (before writing code): does this need to exist? → already in the codebase? → stdlib? → native platform feature? → installed dependency? → one line? → only then, the minimum that works.

**Caveman rules**: drop articles, filler, pleasantries and hedging; fragments are fine; short synonyms; technical terms exact; code blocks unchanged.

### LLM routing

```text
Discord user
  → Hermes Agent (Discord bot, single agent)
    → OMO agent fleet (routing + fallbacks)
      → LiteLLM (model access)
        → provider APIs (incl. the Claude proxy)
```

All LLM traffic flows through LiteLLM; LiteLLM provides model access, while the OMO fleet (ConfigMap) owns routing and fallbacks.

### Web dashboard

```text
Browser
  → https://openagent.maklab.net
    → Cloudflare Tunnel
      → Istio Ingress Gateway
        → openagent-hermes-workspace.openagent:3000 (Web UI)
          → openagent-hermes-api.openagent:8642 (API, chat/sessions)
```

The web UI runs in gateway mode as a pure HTTP client of the API server (`HERMES_WEBUI_GATEWAY_BASE_URL` → `openagent-hermes-api:8642`). It does **not** talk to the Hermes built-in dashboard, which is disabled (`HERMES_DASHBOARD=0`): that dashboard runs as a second Hermes process and each process loads the whole MCP fleet, doubling memory and boot contention for an in-cluster-only admin UI. See the `k8s-gitops-context` skill for connectivity modes and troubleshooting.

### Namespaces & secrets

All application resources deploy to the `openagent` namespace, and the gateway pod runs MCP servers as stdio processes within the container. Secrets come from the `svc_openagent` Doppler config (provider keys, Discord, GHCR) and flow in via `envFrom: secretRef` — no Helm `--set` for secrets.

## Skill Single-Source: k8s-gitops-context

The `k8s-gitops-context` skill body is owned here, in `templates/k8s-gitops-context.yaml`, and rendered for two runtimes via the `runtimeMode` value.

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

## MCP Manifest Verification

`hermes-agent.config.mcp_servers` is the **single source of truth** for the MCP servers the gateway connects to. The `bootstrap` init container re-seeds `config.yaml` from this chart on every pod start (`overwrite: true`), so live config edits are reverted — change the manifest here. A bad package name, a missing env var, or a server that silently loses tools used to surface only when a feature was already broken (typically as `Failed to connect to MCP server '<name>'`). Three layers now catch that before it bites:

| Layer | What it does | Run by |
|-------|--------------|--------|
| **Static lint** — `gitopsctl mcp verify --mode validate` | No unpinned/`@latest` stdio package; every server has `connect_timeout`; every stdio server has `idle_timeout_seconds` + `max_lifetime_seconds`; every enabled server declares `resources`/`prompts` and a non-empty `tools.include`; parked servers stay present as `enabled: false`. | CI (`.github/workflows/helm_lint-test.yaml`) and the `mcp-manifest-validate` pre-commit hook via `.useful-scripts/validate_mcp_manifest.sh`. |
| **Preflight Job** — `templates/hooks.yaml` | For every **enabled** server it does a real JSON-RPC `initialize` + `tools/list` (stdio or Streamable HTTP), asserts every declared `tools.include` tool is present, and fails loudly with the server's own stderr on failure. Gated by `mcpVerification.preflight.enabled`. | Helm/ArgoCD **PostSync** hook Job (after the PVC + boot toolchain exist). |
| **Drift CronJob** — `templates/hooks.yaml` | Same handshake on `mcpVerification.cronjob.schedule` (default every 15 min); quiet when the live surface matches, prints + exits non-zero on drift. Gated by `mcpVerification.cronjob.enabled`. | In-cluster `CronJob`. |

The server definitions are rendered once into the `openagent-mcp-manifest` ConfigMap (from these same values) and consumed by both Jobs and by the gateway's boot pre-warm — there is no second server list to drift. The gateway never *starts* a server at boot to warm caches: the `gitopsctl` `mcp prewarm` command (prebuilt tools image, installed into the pod's `tools` emptyDir by the `install-gitopsctl` initContainer) materialises each `npx`/`uvx` cache with a package-resolution command (`npm exec --package=<pkg> -- true`, `uv tool install`), runs every child in its own process group and hard-kills the group on timeout, and sweeps `_npx` orphans left by older boots.

Run the pieces locally:

```bash
.useful-scripts/validate_mcp_manifest.sh
# handshake the enabled servers (built from .useful-scripts/gitopsctl, same binary the cluster jobs run):
go run ./.useful-scripts/gitopsctl/cmd/gitopsctl mcp verify \
  --manifest <(helm template openagent services/helm/openagent --skip-schema-validation | yq 'select(.kind=="ConfigMap" and .metadata.name=="openagent-mcp-manifest" ).data."mcp-manifest"') \
  --mode preflight
```

### Known limitations

- **A hanging server is only bounded where verification can bound it.** The preflight/CronJob enforce a per-server hard timeout and kill the process group, but the gateway's own connect path still relies on Hermes' `connect_timeout`; a server that hangs mid-session is not detected until the next CronJob run.
- **The client's error surface stays opaque.** Verification recovers the *server's* stderr, which is what Hermes discards; the gateway still reports only its own terse client-side error at runtime.
- **Recycle timings trade availability for leak prevention.** `idle_timeout_seconds` / `max_lifetime_seconds` (30 min / 6 h) recycle a wedged server, but a recycle drops that server's tools until it reconnects. Loosen them in `values.yaml` if a server's startup is expensive.
- **The Jobs reuse the gateway's `ReadWriteOnce` PVC.** On this single-node cluster that is fine; on a multi-node cluster a Job and the gateway pod can land on different nodes and the Job will not schedule while the volume is attached.
- **The CronJob starts a second copy of each server.** Servers that hold exclusive local resources (browser profiles, file locks) can conflict with the gateway's live instance.
- **Public HTTP servers are third-party and can change or rate-limit.** Drift is reported, never auto-fixed; the declared `tools.include` lists for `skiplagged`/`kiwi`/`ferryhopper` are the surface observed when they were pinned into the manifest.
- **Only enabled servers are handshaked.** Parked servers are still checked statically (pin, policy, presence) but are not connected to.
- **helm-unittest needs the patched hermes-agent schema.** The fetched `hermes-agent` chart's `values.schema.json` sets root `additionalProperties: false` and has no `global` property, so any schema-validating tool rejects Helm's injected `global` key. `.useful-scripts/patch_hermes_schema.sh` edits the fetched tgz in place (idempotent, version-agnostic: adds `properties.global`) and is wired into `ct_check.sh`, the pre-commit render/kubeconform + helm-unittest hooks, and CI. `helm lint` and `helm unittest` on this umbrella require it; plain `helm template` renders do not.

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
| hermes-agent.args | list | `[]` |  |
| hermes-agent.command[0] | string | `"/opt/tools/gitopsctl"` |  |
| hermes-agent.command[1] | string | `"boot"` |  |
| hermes-agent.config._config_version | int | `42` |  |
| hermes-agent.config.agent.environment_hint | string | `"# Skills: Ponytail + Caveman + k8s-gitops-context\n\n## K8s GitOps Context\n\nTHIS IS THE CLUSTER SOURCE OF TRUTH. Read @/opt/data/memories/k8s-gitops-context.md\nbefore ANY cluster-related task. Contains:\n- Repo paths, secrets chain (Doppler → ESO → pods)\n- Sync wave order, helm chart patterns, service registration\n- Istio networking, Cloudflare tunnel, Terraform execution order\n- OpenAgent architecture (umbrella chart, native OMO orchestration, Claude proxy)\n- Critical gotchas (SNI, ExternalSecret patterns, storage limitations)\n\nNEVER operate on cluster resources without reading the context first.\n\n## Ponytail — YAGNI Ladder (before writing code)\n\nBefore writing code, stop at the first rung that holds:\n1. Does this need to exist? → no: skip it (YAGNI)\n2. Already in this codebase? → reuse it, don't rewrite\n3. Stdlib does it? → use it\n4. Native platform feature? → use it\n5. Installed dependency? → use it\n6. One line? → one line\n7. Only then: the minimum that works\n\nThe ladder runs after understanding the problem, not instead of it.\nLazy about the solution, never about reading.\n\nLazy, not negligent: trust-boundary validation, data-loss handling,\nsecurity, and accessibility are never on the chopping block.\n\nSource: github.com/DietrichGebert/ponytail\n\n## Caveman — Terse Communication\n\nRespond terse like smart caveman. All technical substance stay.\nOnly fluff die.\n\nRules:\n- Drop: articles (a/an/the), filler (just/really/basically),\n  pleasantries (sure/certainly/of course), hedging\n- Fragments OK. Short synonyms. Technical terms exact.\n- Code blocks unchanged. Errors quoted exact.\n- Pattern: [thing] [action] [reason]. [next step].\n\nNot: \"Sure! I'd be happy to help you with that.\"\nYes: \"Bug in auth middleware. Token expiry check use `<` not `<=`. Fix:\"\n\nAuto-Clarity: Drop caveman for security warnings, irreversible\nactions, multi-step sequences where fragments risk misread.\nResume after clear part.\n\nSource: github.com/JuliusBrussee/caveman\n\n## Pre-commit — Validate Before Commit\n\nBEFORE EVERY COMMIT: run pre-commit hooks.\n\n```bash\npre-commit run --files $(git diff --cached --name-only)\n```\n\nHooks in this repo:\n- yamllint: YAML syntax, indentation\n- check-merge-conflict: unresolved merge markers\n- trailing-whitespace: trailing spaces\n- gitleaks: API keys, tokens\n- helm-docs: Helm chart docs sync\n\nFailure flow:\n1. pre-commit fails → read error\n2. Fix issue (usually indentation)\n3. git add fixed file\n4. Re-run pre-commit\n5. Green → commit\n"` |  |
| hermes-agent.config.agent.max_turns | int | `90` |  |
| hermes-agent.config.agent.system_prompt | string | `"You are Hermes — the communicator. You field ALL prompts and are the\nSOLE user-facing persona. You never act as a worker yourself, and a\nworker (Sisyphus, Hephaestus, Oracle, …) never addresses the user\ndirectly — you synthesize its results and speak for it. Classify every\nrequest BEFORE acting.\n\n## Classification\n\n- TRIVIAL (typo, single config, known pattern): Answer directly. No delegation.\n- STANDARD (new feature, refactor, multi-file): Route through planning pipeline.\n- COMPLEX (architecture, cross-system, security): Full pipeline with review gates.\n\n## Delegation\n\n1. Assess context: is the request clear and unambiguous?\n   → NO: Ask ONE clarifying question first.\n   → YES: Proceed.\n\n2. Standard: Build a plan → present for USER APPROVAL → wait for \"go\" / \"approved\".\n   Complex: Analyze (Metis) → Architect (Oracle) → Plan (Prometheus) → Review (Momus)\n   → present for USER APPROVAL.\n\n3. NEVER execute Standard/Complex work without explicit user sign-off.\n\n4. On approval: spin up Plane kanban tickets via the plane-ticket-sync\n   skill (project per board, [Spec] parent ticket + child tickets per\n   work item, Risks/gotchas as comments) unless the user declines.\n\n## Tool routing\n\n- Quick work (< 3 tool calls): do it yourself (read_file, terminal, web).\n- Simple focused subtask / non-coding: `delegate_task`.\n- Real engineering (multi-file, refactor, bugfix, tests): `omo`\n  (action=\"dispatch\"). The OMO orchestrator runs the agent fleet\n  internally (Sisyphus, Hephaestus, Oracle, … — see the OMO roster).\n  Inject project conventions + memory context into the prompt.\n- After every omo dispatch: check returned `status` (completed /\n  error / timeout), read the result summary, verify changes against\n  the request, update memory/todos, then synthesize and report to the\n  user as Hermes.\n- Max concurrency: 8 for omo dispatches. Do not fire parallel omo\n  dispatches against the same directory/repo — serialize those.\n\n## Identity\n\n- You are always Hermes to the user. Never present a worker persona as\n  the assistant or let raw worker output reach the user — synthesize it\n  first, then speak.\n\n## Approval Gates (BLOCK these without asking)\n\n- merge / commit\n- publish / deploy / push\n- destructive (delete, teardown, drop)\n- external-send (email, API, webhook)\n\n## Style\n\n- Terse. Caveman mode. Drop articles and filler.\n- ALWAYS verbalize your classification: \"Classified as [tier].\"\n- Show your work. Tell user what you're doing.\n- When delegating: \"Delegating to [agent] for [task].\"\n"` |  |
| hermes-agent.config.auxiliary.vision.model | string | `"claude/sonnet-5"` |  |
| hermes-agent.config.auxiliary.vision.provider | string | `"litellm"` |  |
| hermes-agent.config.compression.enabled | bool | `true` |  |
| hermes-agent.config.compression.target_ratio | float | `0.2` |  |
| hermes-agent.config.compression.threshold | float | `0.35` |  |
| hermes-agent.config.delegation.api_key | string | `"${LITELLM_MASTER_KEY}"` |  |
| hermes-agent.config.delegation.base_url | string | `"http://openagent-litellm.openagent.svc.cluster.local:4000/v1"` |  |
| hermes-agent.config.delegation.fallback_providers[0].model | string | `"deepseek-v4-pro"` |  |
| hermes-agent.config.delegation.fallback_providers[0].provider | string | `"litellm"` |  |
| hermes-agent.config.delegation.fallback_providers[1].model | string | `"claude/sonnet-5"` |  |
| hermes-agent.config.delegation.fallback_providers[1].provider | string | `"litellm"` |  |
| hermes-agent.config.delegation.fallback_providers[2].model | string | `"bytedance-glm-5.2"` |  |
| hermes-agent.config.delegation.fallback_providers[2].provider | string | `"litellm"` |  |
| hermes-agent.config.delegation.max_concurrent_children | int | `8` |  |
| hermes-agent.config.delegation.max_iterations | int | `30` |  |
| hermes-agent.config.delegation.max_spawn_depth | int | `3` |  |
| hermes-agent.config.delegation.model | string | `"minimax-m3"` |  |
| hermes-agent.config.delegation.provider | string | `"litellm"` |  |
| hermes-agent.config.delegation.reasoning_effort | string | `"medium"` |  |
| hermes-agent.config.delegation.subagent_auto_approve | bool | `true` |  |
| hermes-agent.config.fallback_providers[0].model | string | `"deepseek-v4-pro"` |  |
| hermes-agent.config.fallback_providers[0].provider | string | `"litellm"` |  |
| hermes-agent.config.fallback_providers[1].model | string | `"claude/sonnet-5"` |  |
| hermes-agent.config.fallback_providers[1].provider | string | `"litellm"` |  |
| hermes-agent.config.fallback_providers[2].model | string | `"bytedance-glm-5.2"` |  |
| hermes-agent.config.fallback_providers[2].provider | string | `"litellm"` |  |
| hermes-agent.config.mcp_servers.arctic-shift.args[0] | string | `"-y"` |  |
| hermes-agent.config.mcp_servers.arctic-shift.args[1] | string | `"arctic-shift-mcp@1.0.2"` |  |
| hermes-agent.config.mcp_servers.arctic-shift.command | string | `"npx"` |  |
| hermes-agent.config.mcp_servers.arctic-shift.connect_timeout | int | `180` |  |
| hermes-agent.config.mcp_servers.arctic-shift.env.npm_config_cache | string | `"/opt/data/.npm-mcp/arctic-shift"` |  |
| hermes-agent.config.mcp_servers.arctic-shift.idle_timeout_seconds | int | `1800` |  |
| hermes-agent.config.mcp_servers.arctic-shift.lazy | bool | `true` |  |
| hermes-agent.config.mcp_servers.arctic-shift.max_lifetime_seconds | int | `21600` |  |
| hermes-agent.config.mcp_servers.arctic-shift.tools.include[0] | string | `"search_submissions"` |  |
| hermes-agent.config.mcp_servers.arctic-shift.tools.include[1] | string | `"search_comments"` |  |
| hermes-agent.config.mcp_servers.arctic-shift.tools.include[2] | string | `"get_post_comments"` |  |
| hermes-agent.config.mcp_servers.arctic-shift.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.arctic-shift.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.argocd.args[0] | string | `"-y"` |  |
| hermes-agent.config.mcp_servers.argocd.args[1] | string | `"argocd-mcp@0.9.0"` |  |
| hermes-agent.config.mcp_servers.argocd.args[2] | string | `"stdio"` |  |
| hermes-agent.config.mcp_servers.argocd.auth_probe.args.limit | int | `1` |  |
| hermes-agent.config.mcp_servers.argocd.auth_probe.tool | string | `"list_applications"` |  |
| hermes-agent.config.mcp_servers.argocd.command | string | `"npx"` |  |
| hermes-agent.config.mcp_servers.argocd.connect_timeout | int | `180` |  |
| hermes-agent.config.mcp_servers.argocd.env.ARGOCD_API_TOKEN | string | `"${MCP_ARGOCD_TOKEN}"` |  |
| hermes-agent.config.mcp_servers.argocd.env.ARGOCD_BASE_URL | string | `"${MCP_ARGOCD_URL}"` |  |
| hermes-agent.config.mcp_servers.argocd.env.npm_config_cache | string | `"/opt/data/.npm-mcp/argocd"` |  |
| hermes-agent.config.mcp_servers.argocd.idle_timeout_seconds | int | `1800` |  |
| hermes-agent.config.mcp_servers.argocd.lazy | bool | `true` |  |
| hermes-agent.config.mcp_servers.argocd.max_lifetime_seconds | int | `21600` |  |
| hermes-agent.config.mcp_servers.argocd.tools.include[0] | string | `"list_applications"` |  |
| hermes-agent.config.mcp_servers.argocd.tools.include[1] | string | `"get_application"` |  |
| hermes-agent.config.mcp_servers.argocd.tools.include[2] | string | `"get_application_managed_resources"` |  |
| hermes-agent.config.mcp_servers.argocd.tools.include[3] | string | `"get_application_resource_tree"` |  |
| hermes-agent.config.mcp_servers.argocd.tools.include[4] | string | `"get_application_events"` |  |
| hermes-agent.config.mcp_servers.argocd.tools.include[5] | string | `"sync_application"` |  |
| hermes-agent.config.mcp_servers.argocd.tools.include[6] | string | `"get_appproject"` |  |
| hermes-agent.config.mcp_servers.argocd.tools.include[7] | string | `"get_resources"` |  |
| hermes-agent.config.mcp_servers.argocd.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.argocd.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.bitwarden.args[0] | string | `"-c"` |  |
| hermes-agent.config.mcp_servers.bitwarden.args[1] | string | `"BW_CLIENTID=$BW_CLIENTID BW_CLIENTSECRET=$BW_CLIENTSECRET bw login --apikey 2>/dev/null\nexport BW_SESSION=$(BW_PASSWORD=$BW_PASSWORD bw unlock --passwordenv BW_PASSWORD 2>/dev/null | grep \"BW_SESSION=\" | sed \"s/.*BW_SESSION=\\\"//;s/\\\".*//\")\nexec npx -y @bitwarden/mcp-server@2026.7.0"` |  |
| hermes-agent.config.mcp_servers.bitwarden.auth_probe.tool | string | `"status"` |  |
| hermes-agent.config.mcp_servers.bitwarden.command | string | `"sh"` |  |
| hermes-agent.config.mcp_servers.bitwarden.connect_timeout | int | `180` |  |
| hermes-agent.config.mcp_servers.bitwarden.env.BW_CLIENTID | string | `"${BW_CLIENTID}"` |  |
| hermes-agent.config.mcp_servers.bitwarden.env.BW_CLIENTSECRET | string | `"${BW_CLIENTSECRET}"` |  |
| hermes-agent.config.mcp_servers.bitwarden.env.BW_PASSWORD | string | `"${BW_PASSWORD}"` |  |
| hermes-agent.config.mcp_servers.bitwarden.env.npm_config_cache | string | `"/opt/data/.npm-mcp/bitwarden"` |  |
| hermes-agent.config.mcp_servers.bitwarden.idle_timeout_seconds | int | `1800` |  |
| hermes-agent.config.mcp_servers.bitwarden.lazy | bool | `true` |  |
| hermes-agent.config.mcp_servers.bitwarden.max_lifetime_seconds | int | `21600` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.include[0] | string | `"status"` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.include[10] | string | `"create_folder"` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.include[11] | string | `"edit_folder"` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.include[12] | string | `"create_attachment"` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.include[13] | string | `"create_text_send"` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.include[1] | string | `"sync"` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.include[2] | string | `"list"` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.include[3] | string | `"get"` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.include[4] | string | `"create_item"` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.include[5] | string | `"edit_item"` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.include[6] | string | `"delete"` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.include[7] | string | `"restore"` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.include[8] | string | `"move"` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.include[9] | string | `"generate"` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.bitwarden.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.codegraph.args[0] | string | `"-y"` |  |
| hermes-agent.config.mcp_servers.codegraph.args[1] | string | `"@colbymchenry/codegraph@1.6.0"` |  |
| hermes-agent.config.mcp_servers.codegraph.args[2] | string | `"serve"` |  |
| hermes-agent.config.mcp_servers.codegraph.args[3] | string | `"--mcp"` |  |
| hermes-agent.config.mcp_servers.codegraph.command | string | `"npx"` |  |
| hermes-agent.config.mcp_servers.codegraph.connect_timeout | int | `180` |  |
| hermes-agent.config.mcp_servers.codegraph.env.CODEGRAPH_TELEMETRY | string | `"0"` |  |
| hermes-agent.config.mcp_servers.codegraph.env.npm_config_cache | string | `"/opt/data/.npm-mcp/codegraph"` |  |
| hermes-agent.config.mcp_servers.codegraph.env.npm_config_loglevel | string | `"error"` |  |
| hermes-agent.config.mcp_servers.codegraph.idle_timeout_seconds | int | `1800` |  |
| hermes-agent.config.mcp_servers.codegraph.lazy | bool | `true` |  |
| hermes-agent.config.mcp_servers.codegraph.max_lifetime_seconds | int | `21600` |  |
| hermes-agent.config.mcp_servers.codegraph.tools.include[0] | string | `"codegraph_explore"` |  |
| hermes-agent.config.mcp_servers.codegraph.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.codegraph.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.doppler.args[0] | string | `"-y"` |  |
| hermes-agent.config.mcp_servers.doppler.args[1] | string | `"@dopplerhq/mcp-server@1.0.5"` |  |
| hermes-agent.config.mcp_servers.doppler.auth_probe.tool | string | `"workplace_get"` |  |
| hermes-agent.config.mcp_servers.doppler.command | string | `"npx"` |  |
| hermes-agent.config.mcp_servers.doppler.connect_timeout | int | `180` |  |
| hermes-agent.config.mcp_servers.doppler.env.DOPPLER_TOKEN | string | `"${MCP_DOPPLER_TOKEN}"` |  |
| hermes-agent.config.mcp_servers.doppler.env.npm_config_cache | string | `"/opt/data/.npm-mcp/doppler"` |  |
| hermes-agent.config.mcp_servers.doppler.idle_timeout_seconds | int | `1800` |  |
| hermes-agent.config.mcp_servers.doppler.lazy | bool | `true` |  |
| hermes-agent.config.mcp_servers.doppler.max_lifetime_seconds | int | `21600` |  |
| hermes-agent.config.mcp_servers.doppler.tools.include[0] | string | `"secrets_get"` |  |
| hermes-agent.config.mcp_servers.doppler.tools.include[10] | string | `"activity_logs_list"` |  |
| hermes-agent.config.mcp_servers.doppler.tools.include[11] | string | `"workplace_get"` |  |
| hermes-agent.config.mcp_servers.doppler.tools.include[1] | string | `"secrets_list"` |  |
| hermes-agent.config.mcp_servers.doppler.tools.include[2] | string | `"secrets_names"` |  |
| hermes-agent.config.mcp_servers.doppler.tools.include[3] | string | `"secrets_update"` |  |
| hermes-agent.config.mcp_servers.doppler.tools.include[4] | string | `"secrets_delete"` |  |
| hermes-agent.config.mcp_servers.doppler.tools.include[5] | string | `"secrets_download"` |  |
| hermes-agent.config.mcp_servers.doppler.tools.include[6] | string | `"configs_list"` |  |
| hermes-agent.config.mcp_servers.doppler.tools.include[7] | string | `"configs_get"` |  |
| hermes-agent.config.mcp_servers.doppler.tools.include[8] | string | `"projects_list"` |  |
| hermes-agent.config.mcp_servers.doppler.tools.include[9] | string | `"projects_get"` |  |
| hermes-agent.config.mcp_servers.doppler.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.doppler.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.drawio.args[0] | string | `"-c"` |  |
| hermes-agent.config.mcp_servers.drawio.args[1] | string | `"cd /opt/data/drawio-mcp-server && exec /opt/data/bin/deno run --config /opt/data/drawio-mcp-server/deno.json -P --allow-read --allow-env --allow-net src/index.ts --transport stdio"` |  |
| hermes-agent.config.mcp_servers.drawio.command | string | `"sh"` |  |
| hermes-agent.config.mcp_servers.drawio.connect_timeout | int | `120` |  |
| hermes-agent.config.mcp_servers.drawio.idle_timeout_seconds | int | `1800` |  |
| hermes-agent.config.mcp_servers.drawio.lazy | bool | `true` |  |
| hermes-agent.config.mcp_servers.drawio.max_lifetime_seconds | int | `21600` |  |
| hermes-agent.config.mcp_servers.drawio.timeout | int | `120` |  |
| hermes-agent.config.mcp_servers.drawio.tools.include[0] | string | `"search-shapes"` |  |
| hermes-agent.config.mcp_servers.drawio.tools.include[10] | string | `"get-diagram-stats"` |  |
| hermes-agent.config.mcp_servers.drawio.tools.include[11] | string | `"export-diagram"` |  |
| hermes-agent.config.mcp_servers.drawio.tools.include[12] | string | `"import-diagram"` |  |
| hermes-agent.config.mcp_servers.drawio.tools.include[13] | string | `"clear-diagram"` |  |
| hermes-agent.config.mcp_servers.drawio.tools.include[1] | string | `"get-shape-categories"` |  |
| hermes-agent.config.mcp_servers.drawio.tools.include[2] | string | `"get-shapes-in-category"` |  |
| hermes-agent.config.mcp_servers.drawio.tools.include[3] | string | `"get-style-presets"` |  |
| hermes-agent.config.mcp_servers.drawio.tools.include[4] | string | `"add-cells"` |  |
| hermes-agent.config.mcp_servers.drawio.tools.include[5] | string | `"edit-cells"` |  |
| hermes-agent.config.mcp_servers.drawio.tools.include[6] | string | `"edit-edges"` |  |
| hermes-agent.config.mcp_servers.drawio.tools.include[7] | string | `"set-cell-shape"` |  |
| hermes-agent.config.mcp_servers.drawio.tools.include[8] | string | `"delete-cell-by-id"` |  |
| hermes-agent.config.mcp_servers.drawio.tools.include[9] | string | `"list-paged-model"` |  |
| hermes-agent.config.mcp_servers.drawio.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.drawio.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.ferryhopper.connect_timeout | int | `60` |  |
| hermes-agent.config.mcp_servers.ferryhopper.lazy | bool | `true` |  |
| hermes-agent.config.mcp_servers.ferryhopper.timeout | int | `60` |  |
| hermes-agent.config.mcp_servers.ferryhopper.tools.include[0] | string | `"get_ports"` |  |
| hermes-agent.config.mcp_servers.ferryhopper.tools.include[1] | string | `"get_disruptions"` |  |
| hermes-agent.config.mcp_servers.ferryhopper.tools.include[2] | string | `"get_direct_connections_for_ports"` |  |
| hermes-agent.config.mcp_servers.ferryhopper.tools.include[3] | string | `"search_trips"` |  |
| hermes-agent.config.mcp_servers.ferryhopper.tools.include[4] | string | `"search_trips_v2"` |  |
| hermes-agent.config.mcp_servers.ferryhopper.tools.include[5] | string | `"trip_details"` |  |
| hermes-agent.config.mcp_servers.ferryhopper.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.ferryhopper.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.ferryhopper.url | string | `"https://mcp.ferryhopper.com/mcp"` |  |
| hermes-agent.config.mcp_servers.gistpad.args[0] | string | `"-y"` |  |
| hermes-agent.config.mcp_servers.gistpad.args[1] | string | `"gistpad-mcp@0.5.0"` |  |
| hermes-agent.config.mcp_servers.gistpad.command | string | `"npx"` |  |
| hermes-agent.config.mcp_servers.gistpad.connect_timeout | int | `180` |  |
| hermes-agent.config.mcp_servers.gistpad.enabled | bool | `false` |  |
| hermes-agent.config.mcp_servers.gistpad.env.GITHUB_TOKEN | string | `"${MCP_GITHUB_TOKEN}"` |  |
| hermes-agent.config.mcp_servers.gistpad.env.npm_config_cache | string | `"/opt/data/.npm-mcp/gistpad"` |  |
| hermes-agent.config.mcp_servers.gistpad.idle_timeout_seconds | int | `1800` |  |
| hermes-agent.config.mcp_servers.gistpad.lazy | bool | `true` |  |
| hermes-agent.config.mcp_servers.gistpad.max_lifetime_seconds | int | `21600` |  |
| hermes-agent.config.mcp_servers.gistpad.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.gistpad.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.github.args[0] | string | `"-y"` |  |
| hermes-agent.config.mcp_servers.github.args[1] | string | `"@modelcontextprotocol/server-github@2025.4.8"` |  |
| hermes-agent.config.mcp_servers.github.command | string | `"npx"` |  |
| hermes-agent.config.mcp_servers.github.connect_timeout | int | `180` |  |
| hermes-agent.config.mcp_servers.github.enabled | bool | `false` |  |
| hermes-agent.config.mcp_servers.github.env.GITHUB_PERSONAL_ACCESS_TOKEN | string | `"${MCP_GITHUB_TOKEN}"` |  |
| hermes-agent.config.mcp_servers.github.env.npm_config_cache | string | `"/opt/data/.npm-mcp/github"` |  |
| hermes-agent.config.mcp_servers.github.env.npm_config_loglevel | string | `"error"` |  |
| hermes-agent.config.mcp_servers.github.idle_timeout_seconds | int | `1800` |  |
| hermes-agent.config.mcp_servers.github.lazy | bool | `true` |  |
| hermes-agent.config.mcp_servers.github.max_lifetime_seconds | int | `21600` |  |
| hermes-agent.config.mcp_servers.github.tools.include[0] | string | `"list_issues"` |  |
| hermes-agent.config.mcp_servers.github.tools.include[1] | string | `"create_issue"` |  |
| hermes-agent.config.mcp_servers.github.tools.include[2] | string | `"update_issue"` |  |
| hermes-agent.config.mcp_servers.github.tools.include[3] | string | `"search_code"` |  |
| hermes-agent.config.mcp_servers.github.tools.include[4] | string | `"search_repositories"` |  |
| hermes-agent.config.mcp_servers.github.tools.include[5] | string | `"get_file_contents"` |  |
| hermes-agent.config.mcp_servers.github.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.github.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.google-workspace.args[0] | string | `"workspace-mcp==1.26.1"` |  |
| hermes-agent.config.mcp_servers.google-workspace.args[1] | string | `"--tool-tier"` |  |
| hermes-agent.config.mcp_servers.google-workspace.args[2] | string | `"complete"` |  |
| hermes-agent.config.mcp_servers.google-workspace.auth_probe.args.user_google_email | string | `"joe3rdwash@gmail.com"` |  |
| hermes-agent.config.mcp_servers.google-workspace.auth_probe.tool | string | `"list_calendars"` |  |
| hermes-agent.config.mcp_servers.google-workspace.command | string | `"uvx"` |  |
| hermes-agent.config.mcp_servers.google-workspace.connect_timeout | int | `180` |  |
| hermes-agent.config.mcp_servers.google-workspace.env.ALLOWED_FILE_DIRS | string | `"/opt/data/tmp"` |  |
| hermes-agent.config.mcp_servers.google-workspace.env.GOOGLE_OAUTH_CLIENT_ID | string | `"${GOOGLE_OAUTH_CLIENT_ID}"` |  |
| hermes-agent.config.mcp_servers.google-workspace.env.GOOGLE_OAUTH_CLIENT_SECRET | string | `"${GOOGLE_OAUTH_CLIENT_SECRET}"` |  |
| hermes-agent.config.mcp_servers.google-workspace.env.HOME | string | `"/opt/data/home"` |  |
| hermes-agent.config.mcp_servers.google-workspace.env.MISE_CACHE_DIR | string | `"/opt/data/mise-cache"` |  |
| hermes-agent.config.mcp_servers.google-workspace.env.MISE_CONFIG_FILE | string | `"/mise/mise.toml"` |  |
| hermes-agent.config.mcp_servers.google-workspace.env.MISE_DATA_DIR | string | `"/opt/data/mise"` |  |
| hermes-agent.config.mcp_servers.google-workspace.env.NPM_CONFIG_CACHE | string | `"/opt/data/home/.npm"` |  |
| hermes-agent.config.mcp_servers.google-workspace.env.PATH | string | `"/opt/data/bin:/opt/data/mise/shims:/opt/data/home/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"` |  |
| hermes-agent.config.mcp_servers.google-workspace.env.UV_CACHE_DIR | string | `"/opt/data/home/.cache/uv"` |  |
| hermes-agent.config.mcp_servers.google-workspace.env.UV_TOOL_BIN_DIR | string | `"/opt/data/home/.local/bin"` |  |
| hermes-agent.config.mcp_servers.google-workspace.env.UV_TOOL_DIR | string | `"/opt/data/home/.local/share/uv/tools"` |  |
| hermes-agent.config.mcp_servers.google-workspace.idle_timeout_seconds | int | `1800` |  |
| hermes-agent.config.mcp_servers.google-workspace.lazy | bool | `true` |  |
| hermes-agent.config.mcp_servers.google-workspace.max_lifetime_seconds | int | `21600` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[0] | string | `"create_doc"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[10] | string | `"update_doc_headers_footers"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[11] | string | `"export_doc_to_pdf"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[12] | string | `"list_docs_in_folder"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[13] | string | `"create_drive_file"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[14] | string | `"create_drive_folder"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[15] | string | `"list_drive_items"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[16] | string | `"search_drive_files"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[17] | string | `"get_drive_file_permissions"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[18] | string | `"get_drive_shareable_link"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[19] | string | `"manage_drive_access"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[1] | string | `"get_doc_content"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[20] | string | `"import_to_google_doc"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[21] | string | `"update_drive_file"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[22] | string | `"copy_drive_file"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[23] | string | `"get_drive_file_content"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[24] | string | `"create_spreadsheet"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[25] | string | `"read_sheet_values"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[26] | string | `"modify_sheet_values"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[27] | string | `"get_spreadsheet_info"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[28] | string | `"search_gmail_messages"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[29] | string | `"get_gmail_message_content"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[2] | string | `"get_doc_as_markdown"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[30] | string | `"send_gmail_message"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[31] | string | `"draft_gmail_message"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[32] | string | `"get_events"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[33] | string | `"manage_event"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[34] | string | `"list_calendars"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[3] | string | `"inspect_doc_structure"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[4] | string | `"batch_update_doc"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[5] | string | `"modify_doc_text"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[6] | string | `"find_and_replace_doc"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[7] | string | `"create_table_with_data"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[8] | string | `"debug_table_structure"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.include[9] | string | `"update_paragraph_style"` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.google-workspace.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.grafana.args[0] | string | `"-c"` |  |
| hermes-agent.config.mcp_servers.grafana.args[1] | string | `"exec npx -y @leval/mcp-grafana@1.1.7 | grep --line-buffered jsonrpc"` |  |
| hermes-agent.config.mcp_servers.grafana.auth_probe.args.query | string | `""` |  |
| hermes-agent.config.mcp_servers.grafana.auth_probe.tool | string | `"search_dashboards"` |  |
| hermes-agent.config.mcp_servers.grafana.command | string | `"sh"` |  |
| hermes-agent.config.mcp_servers.grafana.connect_timeout | int | `180` |  |
| hermes-agent.config.mcp_servers.grafana.env.GRAFANA_SERVICE_ACCOUNT_TOKEN | string | `"${MCP_GRAFANA_TOKEN}"` |  |
| hermes-agent.config.mcp_servers.grafana.env.GRAFANA_URL | string | `"${MCP_GRAFANA_URL}"` |  |
| hermes-agent.config.mcp_servers.grafana.env.npm_config_cache | string | `"/opt/data/.npm-mcp/grafana"` |  |
| hermes-agent.config.mcp_servers.grafana.idle_timeout_seconds | int | `1800` |  |
| hermes-agent.config.mcp_servers.grafana.lazy | bool | `true` |  |
| hermes-agent.config.mcp_servers.grafana.max_lifetime_seconds | int | `21600` |  |
| hermes-agent.config.mcp_servers.grafana.tools.include[0] | string | `"search_dashboards"` |  |
| hermes-agent.config.mcp_servers.grafana.tools.include[10] | string | `"list_prometheus_label_values"` |  |
| hermes-agent.config.mcp_servers.grafana.tools.include[11] | string | `"list_loki_label_names"` |  |
| hermes-agent.config.mcp_servers.grafana.tools.include[1] | string | `"get_dashboard_by_uid"` |  |
| hermes-agent.config.mcp_servers.grafana.tools.include[2] | string | `"get_dashboard_summary"` |  |
| hermes-agent.config.mcp_servers.grafana.tools.include[3] | string | `"get_dashboard_panel_queries"` |  |
| hermes-agent.config.mcp_servers.grafana.tools.include[4] | string | `"list_datasources"` |  |
| hermes-agent.config.mcp_servers.grafana.tools.include[5] | string | `"get_datasource_by_uid"` |  |
| hermes-agent.config.mcp_servers.grafana.tools.include[6] | string | `"get_datasource_by_name"` |  |
| hermes-agent.config.mcp_servers.grafana.tools.include[7] | string | `"query_prometheus"` |  |
| hermes-agent.config.mcp_servers.grafana.tools.include[8] | string | `"list_prometheus_metric_names"` |  |
| hermes-agent.config.mcp_servers.grafana.tools.include[9] | string | `"list_prometheus_label_names"` |  |
| hermes-agent.config.mcp_servers.grafana.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.grafana.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.kiwi.connect_timeout | int | `60` |  |
| hermes-agent.config.mcp_servers.kiwi.lazy | bool | `true` |  |
| hermes-agent.config.mcp_servers.kiwi.timeout | int | `60` |  |
| hermes-agent.config.mcp_servers.kiwi.tools.include[0] | string | `"search-flight"` |  |
| hermes-agent.config.mcp_servers.kiwi.tools.include[1] | string | `"feedback-to-devs"` |  |
| hermes-agent.config.mcp_servers.kiwi.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.kiwi.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.kiwi.url | string | `"https://mcp.kiwi.com"` |  |
| hermes-agent.config.mcp_servers.obscura.args[0] | string | `"mcp"` |  |
| hermes-agent.config.mcp_servers.obscura.args[1] | string | `"--stealth"` |  |
| hermes-agent.config.mcp_servers.obscura.command | string | `"/opt/data/bin/obscura"` |  |
| hermes-agent.config.mcp_servers.obscura.connect_timeout | int | `60` |  |
| hermes-agent.config.mcp_servers.obscura.idle_timeout_seconds | int | `1800` |  |
| hermes-agent.config.mcp_servers.obscura.lazy | bool | `true` |  |
| hermes-agent.config.mcp_servers.obscura.max_lifetime_seconds | int | `21600` |  |
| hermes-agent.config.mcp_servers.obscura.timeout | int | `120` |  |
| hermes-agent.config.mcp_servers.obscura.tools.include[0] | string | `"browser_navigate"` |  |
| hermes-agent.config.mcp_servers.obscura.tools.include[10] | string | `"browser_screenshot"` |  |
| hermes-agent.config.mcp_servers.obscura.tools.include[11] | string | `"browser_scroll"` |  |
| hermes-agent.config.mcp_servers.obscura.tools.include[12] | string | `"browser_search"` |  |
| hermes-agent.config.mcp_servers.obscura.tools.include[13] | string | `"browser_evaluate"` |  |
| hermes-agent.config.mcp_servers.obscura.tools.include[14] | string | `"browser_wait_for_text"` |  |
| hermes-agent.config.mcp_servers.obscura.tools.include[1] | string | `"browser_snapshot"` |  |
| hermes-agent.config.mcp_servers.obscura.tools.include[2] | string | `"browser_markdown"` |  |
| hermes-agent.config.mcp_servers.obscura.tools.include[3] | string | `"browser_click"` |  |
| hermes-agent.config.mcp_servers.obscura.tools.include[4] | string | `"browser_type"` |  |
| hermes-agent.config.mcp_servers.obscura.tools.include[5] | string | `"browser_fill"` |  |
| hermes-agent.config.mcp_servers.obscura.tools.include[6] | string | `"browser_fill_form"` |  |
| hermes-agent.config.mcp_servers.obscura.tools.include[7] | string | `"browser_extract"` |  |
| hermes-agent.config.mcp_servers.obscura.tools.include[8] | string | `"browser_interactive_elements"` |  |
| hermes-agent.config.mcp_servers.obscura.tools.include[9] | string | `"browser_links"` |  |
| hermes-agent.config.mcp_servers.obscura.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.obscura.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.plane.args[0] | string | `"plane-mcp-server==0.3.2"` |  |
| hermes-agent.config.mcp_servers.plane.args[1] | string | `"stdio"` |  |
| hermes-agent.config.mcp_servers.plane.auth_probe.args.action | string | `"list"` |  |
| hermes-agent.config.mcp_servers.plane.auth_probe.tool | string | `"project"` |  |
| hermes-agent.config.mcp_servers.plane.command | string | `"uvx"` |  |
| hermes-agent.config.mcp_servers.plane.connect_timeout | int | `180` |  |
| hermes-agent.config.mcp_servers.plane.env.HOME | string | `"/opt/data/home"` |  |
| hermes-agent.config.mcp_servers.plane.env.MISE_CACHE_DIR | string | `"/opt/data/mise-cache"` |  |
| hermes-agent.config.mcp_servers.plane.env.MISE_CONFIG_FILE | string | `"/mise/mise.toml"` |  |
| hermes-agent.config.mcp_servers.plane.env.MISE_DATA_DIR | string | `"/opt/data/mise"` |  |
| hermes-agent.config.mcp_servers.plane.env.NPM_CONFIG_CACHE | string | `"/opt/data/home/.npm"` |  |
| hermes-agent.config.mcp_servers.plane.env.PATH | string | `"/opt/data/bin:/opt/data/mise/shims:/opt/data/home/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"` |  |
| hermes-agent.config.mcp_servers.plane.env.PLANE_API_KEY | string | `"${PLANE_API_KEY}"` |  |
| hermes-agent.config.mcp_servers.plane.env.PLANE_BASE_URL | string | `"https://plane.maklab.net"` |  |
| hermes-agent.config.mcp_servers.plane.env.PLANE_INTERNAL_BASE_URL | string | `"http://plane-api.plane.svc.cluster.local:8000"` |  |
| hermes-agent.config.mcp_servers.plane.env.PLANE_WORKSPACE_SLUG | string | `"${PLANE_WORKSPACE_SLUG}"` |  |
| hermes-agent.config.mcp_servers.plane.env.UV_CACHE_DIR | string | `"/opt/data/home/.cache/uv"` |  |
| hermes-agent.config.mcp_servers.plane.env.UV_TOOL_BIN_DIR | string | `"/opt/data/home/.local/bin"` |  |
| hermes-agent.config.mcp_servers.plane.env.UV_TOOL_DIR | string | `"/opt/data/home/.local/share/uv/tools"` |  |
| hermes-agent.config.mcp_servers.plane.idle_timeout_seconds | int | `1800` |  |
| hermes-agent.config.mcp_servers.plane.lazy | bool | `true` |  |
| hermes-agent.config.mcp_servers.plane.max_lifetime_seconds | int | `21600` |  |
| hermes-agent.config.mcp_servers.plane.timeout | int | `120` |  |
| hermes-agent.config.mcp_servers.plane.tools.include[0] | string | `"project"` |  |
| hermes-agent.config.mcp_servers.plane.tools.include[1] | string | `"workitem"` |  |
| hermes-agent.config.mcp_servers.plane.tools.include[2] | string | `"member"` |  |
| hermes-agent.config.mcp_servers.plane.tools.include[3] | string | `"state"` |  |
| hermes-agent.config.mcp_servers.plane.tools.include[4] | string | `"label"` |  |
| hermes-agent.config.mcp_servers.plane.tools.include[5] | string | `"workitem_comment"` |  |
| hermes-agent.config.mcp_servers.plane.tools.include[6] | string | `"workitem_attachment"` |  |
| hermes-agent.config.mcp_servers.plane.tools.include[7] | string | `"workitem_relation"` |  |
| hermes-agent.config.mcp_servers.plane.tools.include[8] | string | `"page"` |  |
| hermes-agent.config.mcp_servers.plane.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.plane.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.skiplagged.connect_timeout | int | `60` |  |
| hermes-agent.config.mcp_servers.skiplagged.lazy | bool | `true` |  |
| hermes-agent.config.mcp_servers.skiplagged.timeout | int | `60` |  |
| hermes-agent.config.mcp_servers.skiplagged.tools.include[0] | string | `"sk_flights_search"` |  |
| hermes-agent.config.mcp_servers.skiplagged.tools.include[1] | string | `"sk_destinations_anywhere"` |  |
| hermes-agent.config.mcp_servers.skiplagged.tools.include[2] | string | `"sk_flex_departure_calendar"` |  |
| hermes-agent.config.mcp_servers.skiplagged.tools.include[3] | string | `"sk_flex_return_calendar"` |  |
| hermes-agent.config.mcp_servers.skiplagged.tools.include[4] | string | `"sk_hotels_search"` |  |
| hermes-agent.config.mcp_servers.skiplagged.tools.include[5] | string | `"sk_cars_search"` |  |
| hermes-agent.config.mcp_servers.skiplagged.tools.include[6] | string | `"sk_hotel_details"` |  |
| hermes-agent.config.mcp_servers.skiplagged.tools.include[7] | string | `"sk_faq_search"` |  |
| hermes-agent.config.mcp_servers.skiplagged.tools.include[8] | string | `"sk_resolve_location"` |  |
| hermes-agent.config.mcp_servers.skiplagged.tools.include[9] | string | `"sk_resolve_iata"` |  |
| hermes-agent.config.mcp_servers.skiplagged.tools.prompts | bool | `false` |  |
| hermes-agent.config.mcp_servers.skiplagged.tools.resources | bool | `false` |  |
| hermes-agent.config.mcp_servers.skiplagged.url | string | `"https://mcp.skiplagged.com/mcp"` |  |
| hermes-agent.config.model.default | string | `"deepseek-v4-flash-direct"` |  |
| hermes-agent.config.model.provider | string | `"litellm"` |  |
| hermes-agent.config.plugins.enabled[0] | string | `"discord-platform"` |  |
| hermes-agent.config.plugins.enabled[1] | string | `"omo"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.deep[0] | string | `"litellm/minimax-m3"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.deep[1] | string | `"litellm/glm-5.3"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.deep[2] | string | `"litellm/claude-sonnet-5"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.deep[3] | string | `"litellm/bytedance-dola-seed-2.0-pro"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.deep[4] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.quick[0] | string | `"litellm/deepseek-v4-flash"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.quick[1] | string | `"litellm/minimax-m3"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.quick[2] | string | `"litellm/claude-haiku-4-5"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.quick[3] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.ultrabrain[0] | string | `"litellm/claude-opus-5"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.ultrabrain[1] | string | `"litellm/glm-5.3"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.ultrabrain[2] | string | `"litellm/bytedance-seed-code"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.ultrabrain[3] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.visual-engineering[0] | string | `"litellm/gemini-3.6-flash"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.visual-engineering[1] | string | `"litellm/qwen3-6-plus"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.visual-engineering[2] | string | `"litellm/claude-sonnet-5"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.visual-engineering[3] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.writing[0] | string | `"litellm/minimax-m3"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.writing[1] | string | `"litellm/claude-sonnet-5"` |  |
| hermes-agent.config.plugins.entries.omo.settings.categories.writing[2] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.atlas[0] | string | `"litellm/minimax-m3"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.atlas[1] | string | `"litellm/deepseek-v4-pro"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.atlas[2] | string | `"litellm/claude-sonnet-5"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.atlas[3] | string | `"litellm/bytedance-dola-seed-2.0-pro"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.atlas[4] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.explore[0] | string | `"litellm/deepseek-v4-flash"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.explore[1] | string | `"litellm/minimax-m3"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.explore[2] | string | `"litellm/claude-haiku-4-5"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.explore[3] | string | `"litellm/bytedance-deepseek-v4-flash"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.explore[4] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.hephaestus[0] | string | `"litellm/minimax-m3"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.hephaestus[1] | string | `"litellm/glm-5.3"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.hephaestus[2] | string | `"litellm/claude-sonnet-5"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.hephaestus[3] | string | `"litellm/bytedance-dola-seed-2.0-code"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.hephaestus[4] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.librarian[0] | string | `"litellm/deepseek-v4-flash"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.librarian[1] | string | `"litellm/minimax-m3"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.librarian[2] | string | `"litellm/claude-haiku-4-5"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.librarian[3] | string | `"litellm/bytedance-deepseek-v4-flash"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.librarian[4] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.metis[0] | string | `"litellm/minimax-m3"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.metis[1] | string | `"litellm/deepseek-v4-pro"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.metis[2] | string | `"litellm/claude-sonnet-5"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.metis[3] | string | `"litellm/bytedance-glm-5.2"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.metis[4] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.momus[0] | string | `"litellm/minimax-m3"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.momus[1] | string | `"litellm/glm-5.3"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.momus[2] | string | `"litellm/claude-sonnet-5"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.momus[3] | string | `"litellm/bytedance-dola-seed-2.0-pro"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.momus[4] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.multimodal-looker[0] | string | `"litellm/gemini-3.6-flash"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.multimodal-looker[1] | string | `"litellm/qwen3-6-plus"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.multimodal-looker[2] | string | `"litellm/claude-sonnet-5"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.multimodal-looker[3] | string | `"litellm/bytedance-seed-code"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.multimodal-looker[4] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.oracle[0] | string | `"litellm/claude-opus-5"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.oracle[1] | string | `"litellm/glm-5.3"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.oracle[2] | string | `"litellm/bytedance-seed-code"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.oracle[3] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.prometheus[0] | string | `"litellm/minimax-m3"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.prometheus[1] | string | `"litellm/deepseek-v4-pro"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.prometheus[2] | string | `"litellm/claude-sonnet-5"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.prometheus[3] | string | `"litellm/bytedance-glm-5.2"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.prometheus[4] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.sisyphus-junior[0] | string | `"litellm/deepseek-v4-flash"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.sisyphus-junior[1] | string | `"litellm/minimax-m3"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.sisyphus-junior[2] | string | `"litellm/claude-haiku-4-5"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.sisyphus-junior[3] | string | `"litellm/bytedance-deepseek-v4-flash"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.sisyphus-junior[4] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.sisyphus[0] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.sisyphus[1] | string | `"litellm/claude-sonnet-5"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.sisyphus[2] | string | `"litellm/bytedance-glm-5.2"` |  |
| hermes-agent.config.plugins.entries.omo.settings.chains.sisyphus[3] | string | `"litellm/deepseek-v4-flash-direct"` |  |
| hermes-agent.config.plugins.entries.omo.settings.runtime_fallback.cooldown_seconds | int | `30` |  |
| hermes-agent.config.plugins.entries.omo.settings.runtime_fallback.enabled | bool | `true` |  |
| hermes-agent.config.plugins.entries.omo.settings.runtime_fallback.max_fallback_attempts | int | `6` |  |
| hermes-agent.config.plugins.entries.omo.settings.runtime_fallback.restore_primary_after_cooldown | bool | `true` |  |
| hermes-agent.config.plugins.entries.omo.settings.runtime_fallback.retry_on_errors[0] | int | `400` |  |
| hermes-agent.config.plugins.entries.omo.settings.runtime_fallback.retry_on_errors[10] | int | `529` |  |
| hermes-agent.config.plugins.entries.omo.settings.runtime_fallback.retry_on_errors[1] | int | `401` |  |
| hermes-agent.config.plugins.entries.omo.settings.runtime_fallback.retry_on_errors[2] | int | `402` |  |
| hermes-agent.config.plugins.entries.omo.settings.runtime_fallback.retry_on_errors[3] | int | `403` |  |
| hermes-agent.config.plugins.entries.omo.settings.runtime_fallback.retry_on_errors[4] | int | `404` |  |
| hermes-agent.config.plugins.entries.omo.settings.runtime_fallback.retry_on_errors[5] | int | `429` |  |
| hermes-agent.config.plugins.entries.omo.settings.runtime_fallback.retry_on_errors[6] | int | `500` |  |
| hermes-agent.config.plugins.entries.omo.settings.runtime_fallback.retry_on_errors[7] | int | `502` |  |
| hermes-agent.config.plugins.entries.omo.settings.runtime_fallback.retry_on_errors[8] | int | `503` |  |
| hermes-agent.config.plugins.entries.omo.settings.runtime_fallback.retry_on_errors[9] | int | `504` |  |
| hermes-agent.config.providers.litellm.base_url | string | `"http://openagent-litellm.openagent.svc.cluster.local:4000/v1"` |  |
| hermes-agent.config.providers.litellm.discover_models | bool | `true` |  |
| hermes-agent.config.providers.litellm.key_env | string | `"LITELLM_MASTER_KEY"` |  |
| hermes-agent.config.tools.tool_search.enabled | string | `"on"` |  |
| hermes-agent.config.tools.tool_search.max_search_limit | int | `20` |  |
| hermes-agent.config.tools.tool_search.search_default_limit | int | `5` |  |
| hermes-agent.config.tools.tool_search.threshold_pct | int | `10` |  |
| hermes-agent.env | object | `{}` |  |
| hermes-agent.extraEnvFrom[0].secretRef.name | string | `"openagent-secrets"` |  |
| hermes-agent.extraEnv[0].name | string | `"DISCORD_BOT_TOKEN"` |  |
| hermes-agent.extraEnv[0].valueFrom.secretKeyRef.key | string | `"DISCORD_BOT_TOKEN"` |  |
| hermes-agent.extraEnv[0].valueFrom.secretKeyRef.name | string | `"openagent-secrets"` |  |
| hermes-agent.extraEnv[10].name | string | `"WEBUI_BASE_URL"` |  |
| hermes-agent.extraEnv[10].value | string | `"https://openagent.maklab.net"` |  |
| hermes-agent.extraEnv[1].name | string | `"DISCORD_BOT_CLIENT_ID"` |  |
| hermes-agent.extraEnv[1].valueFrom.secretKeyRef.key | string | `"DISCORD_BOT_CLIENT_ID"` |  |
| hermes-agent.extraEnv[1].valueFrom.secretKeyRef.name | string | `"openagent-secrets"` |  |
| hermes-agent.extraEnv[2].name | string | `"HERMES_DASHBOARD"` |  |
| hermes-agent.extraEnv[2].value | string | `"0"` |  |
| hermes-agent.extraEnv[3].name | string | `"DISCORD_ALLOW_ALL_USERS"` |  |
| hermes-agent.extraEnv[3].value | string | `"true"` |  |
| hermes-agent.extraEnv[4].name | string | `"API_SERVER_ENABLED"` |  |
| hermes-agent.extraEnv[4].value | string | `"true"` |  |
| hermes-agent.extraEnv[5].name | string | `"API_SERVER_KEY"` |  |
| hermes-agent.extraEnv[5].valueFrom.secretKeyRef.key | string | `"API_SERVER_KEY"` |  |
| hermes-agent.extraEnv[5].valueFrom.secretKeyRef.name | string | `"openagent-secrets"` |  |
| hermes-agent.extraEnv[6].name | string | `"API_SERVER_HOST"` |  |
| hermes-agent.extraEnv[6].value | string | `"0.0.0.0"` |  |
| hermes-agent.extraEnv[7].name | string | `"API_SERVER_PORT"` |  |
| hermes-agent.extraEnv[7].value | string | `"8642"` |  |
| hermes-agent.extraEnv[8].name | string | `"API_SERVER_CORS_ORIGINS"` |  |
| hermes-agent.extraEnv[8].value | string | `"https://openagent.maklab.net"` |  |
| hermes-agent.extraEnv[9].name | string | `"CLAUDE_PROXY_API_KEY"` |  |
| hermes-agent.extraEnv[9].valueFrom.secretKeyRef.key | string | `"CLAUDE_PROXY_API_KEY"` |  |
| hermes-agent.extraEnv[9].valueFrom.secretKeyRef.name | string | `"openagent-secrets"` |  |
| hermes-agent.extraInitContainers[0].args[0] | string | `"install"` |  |
| hermes-agent.extraInitContainers[0].args[1] | string | `"/opt/tools/gitopsctl"` |  |
| hermes-agent.extraInitContainers[0].image | string | `"ghcr.io/jomakori/gitopsctl:dab8bebe51b7f03127f2cc7d145faa35c5bcea4f"` |  |
| hermes-agent.extraInitContainers[0].imagePullPolicy | string | `"IfNotPresent"` |  |
| hermes-agent.extraInitContainers[0].name | string | `"install-gitopsctl"` |  |
| hermes-agent.extraInitContainers[0].securityContext.runAsGroup | int | `10000` |  |
| hermes-agent.extraInitContainers[0].securityContext.runAsUser | int | `10000` |  |
| hermes-agent.extraInitContainers[0].volumeMounts[0].mountPath | string | `"/opt/tools"` |  |
| hermes-agent.extraInitContainers[0].volumeMounts[0].name | string | `"tools"` |  |
| hermes-agent.extraInitContainers[1].command[0] | string | `"sh"` |  |
| hermes-agent.extraInitContainers[1].command[1] | string | `"-c"` |  |
| hermes-agent.extraInitContainers[1].command[2] | string | `"set -eu\nif [ \"${OMO_PROVISION_ENABLED}\" = \"true\" ]; then\n  mkdir -p /opt/data/plugins\n  rm -rf /opt/data/plugins/omo\n  git clone --depth 1 --branch \"${OMO_PROVISION_REF}\" \"${OMO_PROVISION_REPOSITORY}\" /opt/data/plugins/omo\n  chown -R 10000:10000 /opt/data/plugins/omo\nfi\n"` |  |
| hermes-agent.extraInitContainers[1].env[0].name | string | `"OMO_PROVISION_ENABLED"` |  |
| hermes-agent.extraInitContainers[1].env[0].value | string | `"true"` |  |
| hermes-agent.extraInitContainers[1].env[1].name | string | `"OMO_PROVISION_REPOSITORY"` |  |
| hermes-agent.extraInitContainers[1].env[1].value | string | `"https://github.com/jomakori/hermes-omo-plugin.git"` |  |
| hermes-agent.extraInitContainers[1].env[2].name | string | `"OMO_PROVISION_REF"` |  |
| hermes-agent.extraInitContainers[1].env[2].value | string | `"main"` |  |
| hermes-agent.extraInitContainers[1].image | string | `"alpine/git:2.45.2"` |  |
| hermes-agent.extraInitContainers[1].imagePullPolicy | string | `"IfNotPresent"` |  |
| hermes-agent.extraInitContainers[1].name | string | `"provision-omo-plugin"` |  |
| hermes-agent.extraInitContainers[1].securityContext.runAsGroup | int | `10000` |  |
| hermes-agent.extraInitContainers[1].securityContext.runAsUser | int | `10000` |  |
| hermes-agent.extraInitContainers[1].volumeMounts[0].mountPath | string | `"/opt/data"` |  |
| hermes-agent.extraInitContainers[1].volumeMounts[0].name | string | `"data"` |  |
| hermes-agent.extraVolumeMounts[0].mountPath | string | `"/opt/data/hooks/discord-session-link"` |  |
| hermes-agent.extraVolumeMounts[0].name | string | `"hermes-hooks"` |  |
| hermes-agent.extraVolumeMounts[0].readOnly | bool | `true` |  |
| hermes-agent.extraVolumeMounts[1].mountPath | string | `"/opt/data/memories/k8s-gitops-context.md"` |  |
| hermes-agent.extraVolumeMounts[1].name | string | `"k8s-gitops-context"` |  |
| hermes-agent.extraVolumeMounts[1].readOnly | bool | `true` |  |
| hermes-agent.extraVolumeMounts[1].subPath | string | `"k8s-gitops-context.md"` |  |
| hermes-agent.extraVolumeMounts[2].mountPath | string | `"/mise"` |  |
| hermes-agent.extraVolumeMounts[2].name | string | `"hermes-mise-config"` |  |
| hermes-agent.extraVolumeMounts[2].readOnly | bool | `true` |  |
| hermes-agent.extraVolumeMounts[3].mountPath | string | `"/opt/tools"` |  |
| hermes-agent.extraVolumeMounts[3].name | string | `"tools"` |  |
| hermes-agent.extraVolumeMounts[3].readOnly | bool | `true` |  |
| hermes-agent.extraVolumeMounts[4].mountPath | string | `"/opt/data/mcp-verify"` |  |
| hermes-agent.extraVolumeMounts[4].name | string | `"mcp-verify"` |  |
| hermes-agent.extraVolumeMounts[4].readOnly | bool | `true` |  |
| hermes-agent.extraVolumes[0].configMap.name | string | `"openagent-hermes-hooks"` |  |
| hermes-agent.extraVolumes[0].name | string | `"hermes-hooks"` |  |
| hermes-agent.extraVolumes[1].configMap.name | string | `"openagent-k8s-gitops-context"` |  |
| hermes-agent.extraVolumes[1].name | string | `"k8s-gitops-context"` |  |
| hermes-agent.extraVolumes[2].configMap.name | string | `"openagent-hermes-mise-config"` |  |
| hermes-agent.extraVolumes[2].name | string | `"hermes-mise-config"` |  |
| hermes-agent.extraVolumes[3].emptyDir | object | `{}` |  |
| hermes-agent.extraVolumes[3].name | string | `"tools"` |  |
| hermes-agent.extraVolumes[4].configMap.name | string | `"openagent-mcp-manifest"` |  |
| hermes-agent.extraVolumes[4].name | string | `"mcp-verify"` |  |
| hermes-agent.image.pullPolicy | string | `"IfNotPresent"` |  |
| hermes-agent.image.repository | string | `"nousresearch/hermes-agent"` |  |
| hermes-agent.image.tag | string | `"v2026.9.11"` |  |
| hermes-agent.probes.readiness.failureThreshold | int | `3` |  |
| hermes-agent.probes.readiness.httpGet.path | string | `"/health"` |  |
| hermes-agent.probes.readiness.httpGet.port | int | `8642` |  |
| hermes-agent.probes.readiness.periodSeconds | int | `10` |  |
| hermes-agent.probes.readiness.timeoutSeconds | int | `3` |  |
| hermes-agent.probes.startup.failureThreshold | int | `120` |  |
| hermes-agent.probes.startup.httpGet.path | string | `"/health"` |  |
| hermes-agent.probes.startup.httpGet.port | int | `8642` |  |
| hermes-agent.probes.startup.periodSeconds | int | `10` |  |
| hermes-agent.probes.startup.timeoutSeconds | int | `3` |  |
| hermes-agent.resources.limits.cpu | string | `"2"` |  |
| hermes-agent.resources.limits.memory | string | `"3Gi"` |  |
| hermes-agent.resources.requests.cpu | string | `"2"` |  |
| hermes-agent.resources.requests.memory | string | `"3Gi"` |  |
| hermes-agent.service.enabled | bool | `false` |  |
| hermes-agent.service.port | int | `9119` |  |
| hermes-webui.enabled | bool | `true` |  |
| hermes.enabled | bool | `true` |  |
| litellm.autoscaling.enabled | bool | `true` |  |
| litellm.autoscaling.maxReplicas | int | `4` |  |
| litellm.autoscaling.minReplicas | int | `2` |  |
| litellm.autoscaling.targetCPUUtilizationPercentage | int | `70` |  |
| litellm.autoscaling.targetMemoryUtilizationPercentage | int | `70` |  |
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
| litellm.migrationJob.enabled | bool | `false` |  |
| litellm.migrationJob.resources.limits.cpu | string | `"500m"` |  |
| litellm.migrationJob.resources.limits.memory | string | `"4Gi"` |  |
| litellm.migrationJob.resources.requests.cpu | string | `"50m"` |  |
| litellm.migrationJob.resources.requests.memory | string | `"64Mi"` |  |
| litellm.proxy_config.general_settings.master_key | string | `"os.environ/LITELLM_MASTER_KEY"` |  |
| litellm.proxy_config.general_settings.pass_through_endpoints[0].forward_headers | bool | `true` |  |
| litellm.proxy_config.general_settings.pass_through_endpoints[0].include_subpath | bool | `true` |  |
| litellm.proxy_config.general_settings.pass_through_endpoints[0].path | string | `"/claude-pipe"` |  |
| litellm.proxy_config.general_settings.pass_through_endpoints[0].target | string | `"http://claude-proxy.openagent.svc.cluster.local:4523"` |  |
| litellm.proxy_config.general_settings.pass_through_endpoints[0].timeout | int | `300` |  |
| litellm.proxy_config.litellm_settings.drop_params | bool | `true` |  |
| litellm.proxy_config.model_list[0].litellm_params.api_key | string | `"os.environ/ANTHROPIC_API_KEY"` |  |
| litellm.proxy_config.model_list[0].litellm_params.model | string | `"anthropic/claude-opus-4-7"` |  |
| litellm.proxy_config.model_list[0].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[0].model_name | string | `"anthropic/claude-opus-4-7"` |  |
| litellm.proxy_config.model_list[10].litellm_params.api_base | string | `"http://claude-proxy.openagent.svc.cluster.local:4523/v1"` |  |
| litellm.proxy_config.model_list[10].litellm_params.api_key | string | `"os.environ/CLAUDE_PROXY_API_KEY"` |  |
| litellm.proxy_config.model_list[10].litellm_params.model | string | `"openai/claude-sonnet-4-6"` |  |
| litellm.proxy_config.model_list[10].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[10].litellm_params.rpm | int | `120` |  |
| litellm.proxy_config.model_list[10].model_info.cache_read_input_token_cost | float | `3e-7` |  |
| litellm.proxy_config.model_list[10].model_info.input_cost_per_token | float | `0.000003` |  |
| litellm.proxy_config.model_list[10].model_info.output_cost_per_token | float | `0.000015` |  |
| litellm.proxy_config.model_list[10].model_name | string | `"claude/sonnet-5"` |  |
| litellm.proxy_config.model_list[11].litellm_params.api_base | string | `"http://claude-proxy.openagent.svc.cluster.local:4523/v1"` |  |
| litellm.proxy_config.model_list[11].litellm_params.api_key | string | `"os.environ/CLAUDE_PROXY_API_KEY"` |  |
| litellm.proxy_config.model_list[11].litellm_params.model | string | `"openai/claude-opus-4-7"` |  |
| litellm.proxy_config.model_list[11].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[11].litellm_params.rpm | int | `30` |  |
| litellm.proxy_config.model_list[11].model_info.cache_read_input_token_cost | float | `5e-7` |  |
| litellm.proxy_config.model_list[11].model_info.input_cost_per_token | float | `0.000005` |  |
| litellm.proxy_config.model_list[11].model_info.output_cost_per_token | float | `0.000025` |  |
| litellm.proxy_config.model_list[11].model_name | string | `"claude/opus-4"` |  |
| litellm.proxy_config.model_list[12].litellm_params.api_base | string | `"http://claude-proxy.openagent.svc.cluster.local:4523/v1"` |  |
| litellm.proxy_config.model_list[12].litellm_params.api_key | string | `"os.environ/CLAUDE_PROXY_API_KEY"` |  |
| litellm.proxy_config.model_list[12].litellm_params.model | string | `"openai/claude-opus-5"` |  |
| litellm.proxy_config.model_list[12].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[12].litellm_params.rpm | int | `30` |  |
| litellm.proxy_config.model_list[12].model_info.cache_read_input_token_cost | float | `5e-7` |  |
| litellm.proxy_config.model_list[12].model_info.input_cost_per_token | float | `0.000005` |  |
| litellm.proxy_config.model_list[12].model_info.output_cost_per_token | float | `0.000025` |  |
| litellm.proxy_config.model_list[12].model_name | string | `"claude/opus-5"` |  |
| litellm.proxy_config.model_list[13].litellm_params.api_base | string | `"http://claude-proxy.openagent.svc.cluster.local:4523/v1"` |  |
| litellm.proxy_config.model_list[13].litellm_params.api_key | string | `"os.environ/CLAUDE_PROXY_API_KEY"` |  |
| litellm.proxy_config.model_list[13].litellm_params.model | string | `"openai/claude-haiku-4-5"` |  |
| litellm.proxy_config.model_list[13].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[13].litellm_params.rpm | int | `60` |  |
| litellm.proxy_config.model_list[13].model_info.cache_read_input_token_cost | float | `1e-7` |  |
| litellm.proxy_config.model_list[13].model_info.input_cost_per_token | float | `0.000001` |  |
| litellm.proxy_config.model_list[13].model_info.output_cost_per_token | float | `0.000005` |  |
| litellm.proxy_config.model_list[13].model_name | string | `"claude/haiku-4-5"` |  |
| litellm.proxy_config.model_list[14].litellm_params.api_base | string | `"http://claude-proxy.openagent.svc.cluster.local:4523/v1"` |  |
| litellm.proxy_config.model_list[14].litellm_params.api_key | string | `"os.environ/CLAUDE_PROXY_API_KEY"` |  |
| litellm.proxy_config.model_list[14].litellm_params.model | string | `"openai/claude-opus-5"` |  |
| litellm.proxy_config.model_list[14].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[14].litellm_params.rpm | int | `30` |  |
| litellm.proxy_config.model_list[14].model_info.cache_read_input_token_cost | float | `5e-7` |  |
| litellm.proxy_config.model_list[14].model_info.input_cost_per_token | float | `0.000005` |  |
| litellm.proxy_config.model_list[14].model_info.output_cost_per_token | float | `0.000025` |  |
| litellm.proxy_config.model_list[14].model_name | string | `"claude-opus-5"` |  |
| litellm.proxy_config.model_list[15].litellm_params.api_base | string | `"http://claude-proxy.openagent.svc.cluster.local:4523/v1"` |  |
| litellm.proxy_config.model_list[15].litellm_params.api_key | string | `"os.environ/CLAUDE_PROXY_API_KEY"` |  |
| litellm.proxy_config.model_list[15].litellm_params.model | string | `"openai/claude-sonnet-4-6"` |  |
| litellm.proxy_config.model_list[15].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[15].litellm_params.rpm | int | `120` |  |
| litellm.proxy_config.model_list[15].model_info.cache_read_input_token_cost | float | `3e-7` |  |
| litellm.proxy_config.model_list[15].model_info.input_cost_per_token | float | `0.000003` |  |
| litellm.proxy_config.model_list[15].model_info.output_cost_per_token | float | `0.000015` |  |
| litellm.proxy_config.model_list[15].model_name | string | `"claude-sonnet-5"` |  |
| litellm.proxy_config.model_list[16].litellm_params.api_base | string | `"http://claude-proxy.openagent.svc.cluster.local:4523/v1"` |  |
| litellm.proxy_config.model_list[16].litellm_params.api_key | string | `"os.environ/CLAUDE_PROXY_API_KEY"` |  |
| litellm.proxy_config.model_list[16].litellm_params.model | string | `"openai/claude-haiku-4-5"` |  |
| litellm.proxy_config.model_list[16].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[16].litellm_params.rpm | int | `60` |  |
| litellm.proxy_config.model_list[16].model_info.cache_read_input_token_cost | float | `1e-7` |  |
| litellm.proxy_config.model_list[16].model_info.input_cost_per_token | float | `0.000001` |  |
| litellm.proxy_config.model_list[16].model_info.output_cost_per_token | float | `0.000005` |  |
| litellm.proxy_config.model_list[16].model_name | string | `"claude-haiku-4-5"` |  |
| litellm.proxy_config.model_list[17].litellm_params.api_base | string | `"https://api.kilo.ai/api/gateway/v1"` |  |
| litellm.proxy_config.model_list[17].litellm_params.api_key | string | `"os.environ/KILO_API_KEY"` |  |
| litellm.proxy_config.model_list[17].litellm_params.model | string | `"openai/byteplus-coding/bytedance-seed-code"` |  |
| litellm.proxy_config.model_list[17].litellm_params.num_retries | int | `0` |  |
| litellm.proxy_config.model_list[17].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[17].litellm_params.rpm | int | `5` |  |
| litellm.proxy_config.model_list[17].model_name | string | `"bytedance-seed-code"` |  |
| litellm.proxy_config.model_list[18].litellm_params.api_base | string | `"https://api.kilo.ai/api/gateway/v1"` |  |
| litellm.proxy_config.model_list[18].litellm_params.api_key | string | `"os.environ/KILO_API_KEY"` |  |
| litellm.proxy_config.model_list[18].litellm_params.merge_reasoning_content_in_choices | bool | `true` |  |
| litellm.proxy_config.model_list[18].litellm_params.model | string | `"openai/byteplus-coding/dola-seed-2.0-code"` |  |
| litellm.proxy_config.model_list[18].litellm_params.num_retries | int | `0` |  |
| litellm.proxy_config.model_list[18].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[18].litellm_params.rpm | int | `5` |  |
| litellm.proxy_config.model_list[18].model_name | string | `"bytedance-dola-seed-2.0-code"` |  |
| litellm.proxy_config.model_list[19].litellm_params.api_base | string | `"https://api.kilo.ai/api/gateway/v1"` |  |
| litellm.proxy_config.model_list[19].litellm_params.api_key | string | `"os.environ/KILO_API_KEY"` |  |
| litellm.proxy_config.model_list[19].litellm_params.merge_reasoning_content_in_choices | bool | `true` |  |
| litellm.proxy_config.model_list[19].litellm_params.model | string | `"openai/byteplus-coding/dola-seed-2.0-pro"` |  |
| litellm.proxy_config.model_list[19].litellm_params.num_retries | int | `0` |  |
| litellm.proxy_config.model_list[19].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[19].litellm_params.rpm | int | `5` |  |
| litellm.proxy_config.model_list[19].model_name | string | `"bytedance-dola-seed-2.0-pro"` |  |
| litellm.proxy_config.model_list[1].litellm_params.api_base | string | `"https://api.kilo.ai/api/gateway/v1"` |  |
| litellm.proxy_config.model_list[1].litellm_params.api_key | string | `"os.environ/KILO_API_KEY"` |  |
| litellm.proxy_config.model_list[1].litellm_params.merge_reasoning_content_in_choices | bool | `true` |  |
| litellm.proxy_config.model_list[1].litellm_params.model | string | `"openai/moonshotai/kimi-k3"` |  |
| litellm.proxy_config.model_list[1].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[1].model_info.cache_read_input_token_cost | float | `3e-7` |  |
| litellm.proxy_config.model_list[1].model_info.input_cost_per_token | float | `0.000003` |  |
| litellm.proxy_config.model_list[1].model_info.output_cost_per_token | float | `0.000015` |  |
| litellm.proxy_config.model_list[1].model_name | string | `"kimi-k3"` |  |
| litellm.proxy_config.model_list[20].litellm_params.api_base | string | `"https://api.kilo.ai/api/gateway/v1"` |  |
| litellm.proxy_config.model_list[20].litellm_params.api_key | string | `"os.environ/KILO_API_KEY"` |  |
| litellm.proxy_config.model_list[20].litellm_params.merge_reasoning_content_in_choices | bool | `true` |  |
| litellm.proxy_config.model_list[20].litellm_params.model | string | `"openai/byteplus-coding/glm-5.2"` |  |
| litellm.proxy_config.model_list[20].litellm_params.num_retries | int | `0` |  |
| litellm.proxy_config.model_list[20].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[20].litellm_params.rpm | int | `5` |  |
| litellm.proxy_config.model_list[20].model_name | string | `"bytedance-glm-5.2"` |  |
| litellm.proxy_config.model_list[21].litellm_params.api_base | string | `"https://api.kilo.ai/api/gateway/v1"` |  |
| litellm.proxy_config.model_list[21].litellm_params.api_key | string | `"os.environ/KILO_API_KEY"` |  |
| litellm.proxy_config.model_list[21].litellm_params.merge_reasoning_content_in_choices | bool | `true` |  |
| litellm.proxy_config.model_list[21].litellm_params.model | string | `"openai/byteplus-coding/deepseek-v4-flash"` |  |
| litellm.proxy_config.model_list[21].litellm_params.num_retries | int | `0` |  |
| litellm.proxy_config.model_list[21].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[21].litellm_params.rpm | int | `5` |  |
| litellm.proxy_config.model_list[21].model_name | string | `"bytedance-deepseek-v4-flash"` |  |
| litellm.proxy_config.model_list[2].litellm_params.api_base | string | `"https://api.kilo.ai/api/gateway/v1"` |  |
| litellm.proxy_config.model_list[2].litellm_params.api_key | string | `"os.environ/KILO_API_KEY"` |  |
| litellm.proxy_config.model_list[2].litellm_params.merge_reasoning_content_in_choices | bool | `true` |  |
| litellm.proxy_config.model_list[2].litellm_params.model | string | `"openai/deepseek/deepseek-v4-flash"` |  |
| litellm.proxy_config.model_list[2].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[2].model_info.cache_read_input_token_cost | float | `3e-8` |  |
| litellm.proxy_config.model_list[2].model_info.input_cost_per_token | float | `1.4e-7` |  |
| litellm.proxy_config.model_list[2].model_info.output_cost_per_token | float | `2.8e-7` |  |
| litellm.proxy_config.model_list[2].model_name | string | `"deepseek-v4-flash"` |  |
| litellm.proxy_config.model_list[3].litellm_params.api_key | string | `"os.environ/DEEPSEEK_API_KEY"` |  |
| litellm.proxy_config.model_list[3].litellm_params.cache | bool | `true` |  |
| litellm.proxy_config.model_list[3].litellm_params.model | string | `"deepseek/deepseek-v4-flash"` |  |
| litellm.proxy_config.model_list[3].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[3].litellm_params.use_chat_completions_api | bool | `true` |  |
| litellm.proxy_config.model_list[3].model_name | string | `"deepseek-v4-flash-direct"` |  |
| litellm.proxy_config.model_list[4].litellm_params.api_base | string | `"https://api.kilo.ai/api/gateway/v1"` |  |
| litellm.proxy_config.model_list[4].litellm_params.api_key | string | `"os.environ/KILO_API_KEY"` |  |
| litellm.proxy_config.model_list[4].litellm_params.merge_reasoning_content_in_choices | bool | `true` |  |
| litellm.proxy_config.model_list[4].litellm_params.model | string | `"openai/deepseek/deepseek-v4-pro"` |  |
| litellm.proxy_config.model_list[4].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[4].model_info.cache_read_input_token_cost | float | `1.4e-7` |  |
| litellm.proxy_config.model_list[4].model_info.input_cost_per_token | float | `0.0000016` |  |
| litellm.proxy_config.model_list[4].model_info.output_cost_per_token | float | `0.0000032` |  |
| litellm.proxy_config.model_list[4].model_name | string | `"deepseek-v4-pro"` |  |
| litellm.proxy_config.model_list[5].litellm_params.api_key | string | `"os.environ/DEEPSEEK_API_KEY"` |  |
| litellm.proxy_config.model_list[5].litellm_params.cache | bool | `true` |  |
| litellm.proxy_config.model_list[5].litellm_params.model | string | `"deepseek/deepseek-v4-pro"` |  |
| litellm.proxy_config.model_list[5].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[5].litellm_params.use_chat_completions_api | bool | `true` |  |
| litellm.proxy_config.model_list[5].model_name | string | `"deepseek-v4-pro-direct"` |  |
| litellm.proxy_config.model_list[6].litellm_params.api_base | string | `"https://api.kilo.ai/api/gateway/v1"` |  |
| litellm.proxy_config.model_list[6].litellm_params.api_key | string | `"os.environ/KILO_API_KEY"` |  |
| litellm.proxy_config.model_list[6].litellm_params.merge_reasoning_content_in_choices | bool | `true` |  |
| litellm.proxy_config.model_list[6].litellm_params.model | string | `"openai/z-ai/glm-5.3"` |  |
| litellm.proxy_config.model_list[6].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[6].model_info.cache_read_input_token_cost | float | `2.6e-7` |  |
| litellm.proxy_config.model_list[6].model_info.input_cost_per_token | float | `0.0000014` |  |
| litellm.proxy_config.model_list[6].model_info.output_cost_per_token | float | `0.0000044` |  |
| litellm.proxy_config.model_list[6].model_name | string | `"glm-5.3"` |  |
| litellm.proxy_config.model_list[7].litellm_params.api_base | string | `"https://api.kilo.ai/api/gateway/v1"` |  |
| litellm.proxy_config.model_list[7].litellm_params.api_key | string | `"os.environ/KILO_API_KEY"` |  |
| litellm.proxy_config.model_list[7].litellm_params.model | string | `"openai/minimax/minimax-m3"` |  |
| litellm.proxy_config.model_list[7].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[7].model_info.cache_read_input_token_cost | float | `6e-8` |  |
| litellm.proxy_config.model_list[7].model_info.input_cost_per_token | float | `3e-7` |  |
| litellm.proxy_config.model_list[7].model_info.output_cost_per_token | float | `0.0000012` |  |
| litellm.proxy_config.model_list[7].model_name | string | `"minimax-m3"` |  |
| litellm.proxy_config.model_list[8].litellm_params.api_base | string | `"https://api.kilo.ai/api/gateway/v1"` |  |
| litellm.proxy_config.model_list[8].litellm_params.api_key | string | `"os.environ/KILO_API_KEY"` |  |
| litellm.proxy_config.model_list[8].litellm_params.model | string | `"openai/google/gemini-3.6-flash"` |  |
| litellm.proxy_config.model_list[8].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[8].model_info.cache_read_input_token_cost | float | `7e-8` |  |
| litellm.proxy_config.model_list[8].model_info.input_cost_per_token | float | `7.5e-7` |  |
| litellm.proxy_config.model_list[8].model_info.output_cost_per_token | float | `0.00000375` |  |
| litellm.proxy_config.model_list[8].model_name | string | `"gemini-3.6-flash"` |  |
| litellm.proxy_config.model_list[9].litellm_params.api_base | string | `"https://api.kilo.ai/api/gateway/v1"` |  |
| litellm.proxy_config.model_list[9].litellm_params.api_key | string | `"os.environ/KILO_API_KEY"` |  |
| litellm.proxy_config.model_list[9].litellm_params.model | string | `"openai/qwen/qwen3.6-plus"` |  |
| litellm.proxy_config.model_list[9].litellm_params.order | int | `1` |  |
| litellm.proxy_config.model_list[9].model_info.input_cost_per_token | float | `3.25e-7` |  |
| litellm.proxy_config.model_list[9].model_info.output_cost_per_token | float | `0.00000195` |  |
| litellm.proxy_config.model_list[9].model_name | string | `"qwen3-6-plus"` |  |
| litellm.proxy_config.router_settings.disable_cooldowns | bool | `true` |  |
| litellm.proxy_config.router_settings.num_retries | int | `0` |  |
| litellm.proxy_config.router_settings.request_timeout | int | `180` |  |
| litellm.proxy_config.router_settings.routing_strategy | string | `"least-busy"` |  |
| litellm.resources.limits.cpu | string | `"2000m"` |  |
| litellm.resources.limits.memory | string | `"4Gi"` |  |
| litellm.resources.requests.cpu | string | `"100m"` |  |
| litellm.resources.requests.memory | string | `"1.5Gi"` |  |
| litellm.service.port | int | `4000` |  |
| litellmVirtualService.destination.host | string | `"openagent-litellm.openagent.svc.cluster.local"` |  |
| litellmVirtualService.destination.port | int | `4000` |  |
| litellmVirtualService.host | string | `"litellm.maklab.net"` |  |
| mcpVerification.cronjob.activeDeadlineSeconds | int | `900` |  |
| mcpVerification.cronjob.backoffLimit | int | `0` |  |
| mcpVerification.cronjob.enabled | bool | `true` |  |
| mcpVerification.cronjob.historyLimit | int | `3` |  |
| mcpVerification.cronjob.schedule | string | `"*/15 * * * *"` |  |
| mcpVerification.preflight.activeDeadlineSeconds | int | `1800` |  |
| mcpVerification.preflight.backoffLimit | int | `0` |  |
| mcpVerification.preflight.enabled | bool | `true` |  |
| mcpVerification.preflight.toolchainWaitSeconds | int | `300` |  |
| namespace | string | `"openagent"` |  |
| postgres.clusterName | string | `"openagent-pg"` |  |
| postgres.enabled | bool | `true` |  |
| postgres.userName | string | `"openagent"` |  |
| runtimeMode | string | `"cluster"` |  |
| storageClass | string | `"local-path"` |  |
| tools.image.pullPolicy | string | `"IfNotPresent"` |  |
| tools.image.repository | string | `"ghcr.io/jomakori/gitopsctl"` |  |
| tools.image.tag | string | `"dab8bebe51b7f03127f2cc7d145faa35c5bcea4f"` |  |
| vpa.enabled | bool | `true` |  |
| vpa.targets[0].containers[0].controlledValues | string | `"RequestsOnly"` |  |
| vpa.targets[0].containers[0].maxCpu | string | `"2"` |  |
| vpa.targets[0].containers[0].maxMemory | string | `"8Gi"` |  |
| vpa.targets[0].containers[0].minCpu | string | `"50m"` |  |
| vpa.targets[0].containers[0].minMemory | string | `"512Mi"` |  |
| vpa.targets[0].containers[0].name | string | `"hermes-agent"` |  |
| vpa.targets[0].name | string | `"openagent-hermes-agent"` |  |
| vpa.updateMode | string | `"Auto"` |  |
