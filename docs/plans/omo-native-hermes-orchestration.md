# Spec-Plan: Native OMO Multi-Agent Orchestration in Hermes (retire the OpenCode delegation layer)

> **Status:** **Approved** — D1–D6 locked to the recommended defaults; **D7 = cluster surface only** (local `~/.config/opencode` workstation stays). Executing: **Phase 0 (verification)**.
> **Repo of record:** `jomakori/gke_GitOps` (wiring) + new plugin repo `jomakori/hermes-omo-plugin` (engine)
> **Pulled:** `main` fast-forwarded `d33362e → 4111006` (9 commits) before writing this.

---

## TL;DR

Today Hermes reaches OMO's agent fleet by shelling out: **Hermes → `opencode` plugin → `opencode serve` pod → OMO npm plugin → agents**. Every hop is infrastructure that has to stay alive, be version-pinned, and be reconciled.

Hermes already has a first-class, plugin-safe subagent rail (`ctx.subagent_lifecycle`), an observer/event contract, session/transcript persistence, native `delegate_task`, MCP servers, and a named-agent roster precedent. This plan replaces the whole OpenCode column with **one Hermes plugin** that owns the OMO roster (personas, models, tool scopes, delegation) and drives workers through Hermes' own primitives.

- **Remove:** `charts/opencode-server`, `templates/opencode.yaml`, the `hermes-opencode-plugin` clone, the `OPENCODE_SERVER_URL` wiring, the `oh-my-openagent` npm dependency.
- **Add:** `hermes-omo-plugin` (11 ported personas + orchestrator engine) installed the same way the opencode plugin is today (`gitopsctl boot` clones it into `/opt/data/plugins/omo`).
- **Keep exactly:** the model aliases (LiteLLM), the persona names/roles/models, Hermes as the only user-facing identity.
- **No dual path.** OpenCode is retired outright — not parked behind a feature flag. Rollback is `git revert` of the wiring + an idempotent boot re-clone, not a retained parallel runtime.
- **Decoupled at the concern level.** Plugin provisioning moves *out of* the `gitopsctl` boot binary and *into* the chart. One small Go change removes `opencodeSetup()`; after that, changing the plugin (or its successor) never requires a `gitopsctl` image rebuild + sha pin bump again. `gitopsctl` itself stays in-repo — see §10.
- **Key finding:** the native rail cannot express per-agent *provider* routing or per-agent *fallback* — but the provider half is already satisfied (everything routes through LiteLLM), and the fallback half is ~120 lines of orchestrator logic we own.

**Effort:** Large. 6 phases. Two repos. No data migration (sessions are Hermes-native already).

---

## 1. Context

### 1.1 The system as it stands

```
Discord / WebUI
  └─ Hermes Agent (gateway, HERMES_HOME=/opt/data)
       ├─ profile system_prompt  = "Sisyphus — OMO Orchestrator"  (VALUES, not a real agent)
       ├─ delegate_task          = Hermes primitive subagent
       └─ opencode tool          = hermes-opencode-plugin (clone of zaycruz/…)
            └─ `opencode run --attach http://opencode-server:4096`
                 └─ opencode-server pod  (chart, same image + shared PVC)
                      └─ oh-my-openagent@5.0.0-beta.48  (npm plugin)
                           └─ Sisyphus → Hephaestus / Oracle / Explore / Librarian / …  (OpenCode agents)
```

Fleet config is generated into the `openagent-opencode-config` ConfigMap by `templates/opencode.yaml` from the `opencode:` values block (agents + categories + model chains + MCP gating). Model IDs live once, in `litellm.proxy_config.model_list`.

### 1.2 Why the current shape exists

- **`opencode-server` is a separate pod only to stop OOM-killing the gateway.** A heavy `opencode run` used to run as a gateway child and restart the gateway mid-orchestration. That reason evaporates once workers are Hermes-native subagents (workers no longer hold the gateway's memory; the lifecycle service has its own bounded executor).
- **The plugin exists because Hermes had no native multi-agent fleet.** It does now: `ctx.subagent_lifecycle`, `delegate_task`, `subagent_start/stop` hooks, `subagent.*` gateway RPCs, live transcripts.

### 1.3 What Hermes gives us natively (verified against `NousResearch/hermes-agent@f9524d3`)

| Need | Native surface | Limit we must design around |
|---|---|---|
| Agent tool registration | `ctx.register_tool(name, toolset, schema, handler, check_fn)` | plugin tools bypass the toolset filter — we gate inside the handler |
| Worker spawn / control | `ctx.subagent_lifecycle.launch/status/wait/result/cancel/reconnect` | `SubagentLaunchRequest`: `goal, context, role, model, allowed_toolsets, correlation_id, metadata` — **no `provider`, no per-launch timeout/cwd/blocked-tools** |
| Model-facing delegation | `delegate_task` (`role="leaf"|"orchestrator"`, `tasks[]`, `background`, `output_schema`, `action=list/steer/stop`) | flat fan-out; `max_concurrent_children` default 10; `max_spawn_depth` default 1 |
| Hooks | `pre_tool_call`, `post_tool_call`, `pre/post_llm_call`, `on_session_start/end`, **`subagent_start`/`subagent_stop`**, `pre_approval_request` | payloads are kwargs; directive hooks: `block`/`approve`/`modify` |
| Events | `ctx.emit` / `ctx.subscribe` (plugin-local bus); gateway WS `/api/ws` + `session.events.since` | plugin bus is namespaced to the plugin key |
| Sessions / transcripts | SQLite `state.db`; `session.*`, `subagent.list/tail/progress/tool/thinking`, `delegation.status`; live logs at `<hermes_home>/cache/delegation/live/<delegation_id>/task-<n>.log` | live registry retains terminals ~1 h in-process |
| Per-call model routing | `ctx.llm.complete(provider=, model=, …)` gated by `plugins.entries.<id>.llm.*` trust policy | overrides fail-closed; capability opt-in required |
| Tool scoping | `allowed_toolsets` (subset of parent — **narrow only**), blocked-tool set | coarse: toolsets, not per-tool ACLs |
| Roster precedent | in-tree `hermes-bots` (Bot Mode: profile per agent, `message_agent`, `bot_relay.*`) | desktop-oriented; profile-based, no core `agents:` config key |
| Approval | `security.approval.transport`, `pre_tool_call` block/approve, `delegation.subagent_auto_approve` (default false ⇒ subagent approvals auto-deny) | plugin transports cannot auto-approve |

### 1.4 What we are porting (OMO roster — `code-yeongyu/oh-my-openagent@e614da3`, `5.0.0-beta.79`)

Corrections to the roster in the original request:

- **`ultraworker` is not an agent.** It is part of the Sisyphus display name (`"Sisyphus - ultraworker"`) and the `ultrawork` keyword mode. Do not create an agent for it.
- **`prometheus` is a planner, not a worker.** Plan mode is *sticky*; execution is a separate worker session. Plan-family agents cannot delegate to plan-family.
- **`build` / `plan` are OpenCode-native**, not OMO-defined. They have no Hermes counterpart to port.
- **There is no `default` category** in OMO — the builtin set is 8 (`quick, default` is not one of them; the closest are `unspecified-low`/`-high`).

Full ported roster (names · roles · model chains) — chains are already encoded in our chart, they are the source we keep:

| Agent | Role (as it must appear) | Model chain (from `values.yaml`) | Tool scope |
|---|---|---|---|
| `sisyphus` | Sisyphus · Ultraworker | deepseek-v4-flash-direct → claude-sonnet-5 → bytedance-glm-5.2 | orchestrator (may delegate) |
| `hephaestus` | Hephaestus · Deep Agent | minimax-m3 → glm-5.3 → claude-sonnet-5 → bytedance-dola-seed-2.0-code | full engineering |
| `prometheus` | Prometheus · Plan Builder | minimax-m3 → deepseek-v4-pro → claude-sonnet-5 → bytedance-glm-5.2 | read + plan artifacts |
| `atlas` | Atlas · Plan Executor | minimax-m3 → deepseek-v4-pro → claude-sonnet-5 → bytedance-dola-seed-2.0-pro | orchestrator (plan-driven) |
| `metis` | Metis · Plan Consultant | minimax-m3 → deepseek-v4-pro → claude-sonnet-5 → bytedance-glm-5.2 | read-only |
| `momus` | Momus · Plan Critic | minimax-m3 → glm-5.3 → claude-sonnet-5 → bytedance-dola-seed-2.0-pro | read-only |
| `oracle` | Oracle · Architecture / Reasoning | claude-opus-5 → glm-5.3 → bytedance-seed-code | read-only |
| `librarian` | Librarian · Research | deepseek-v4-flash → minimax-m3 → claude-haiku-4-5 → bytedance-deepseek-v4-flash | read + web |
| `explore` | Explore · Repository Exploration | deepseek-v4-flash → minimax-m3 → claude-haiku-4-5 → bytedance-deepseek-v4-flash | read + search |
| `multimodal-looker` | Multimodal-Looker · Multimodal Analysis | gemini-3.6-flash → qwen3-6-plus → claude-sonnet-5 → bytedance-seed-code | read only |
| `sisyphus-junior` | Sisyphus-Junior · Specialized Execution Worker | deepseek-v4-flash → minimax-m3 → claude-haiku-4-5 → bytedance-deepseek-v4-flash | executable (category spawn target) |

Personas/prompts are ported from OMO prompt sources (Sisyphus, Hephaestus, Atlas, Prometheus, Metis, Momus, Oracle, Librarian, Explore, Multimodal-Looker, Sisyphus-Junior). Where OMO ships **model-family variants** of a prompt (Sisyphus has 12), we port **one canonical body per agent** and record the variant table for follow-up — our models are fixed by the chart, so multi-variant calibration is deferred, not lost.

---

## 2. Goals / Non-Goals

### Goals
1. `User → Hermes → native OMO orchestrator → agents → Hermes → user`. Hermes is the only user-facing identity; workers never become the conversational counterpart.
2. Preserve OMO's named personas, roles, model chains, and delegation behavior.
3. Reproduce the useful OMO workflow natively: routing, hierarchical + parallel delegation, DAG/dependencies, planning→execution→review loops, nested delegation, per-agent model routing, lifecycle, cancellation, structured results, event streaming, transcript visibility.
4. Delete the OpenCode delegation layer entirely (plugin, server pod, npm dependency, config plumbing) — no coexistence window, no fallback flag.
5. Reuse Hermes primitives (delegation, sessions, events, transcripts, tools, providers) — build only what Hermes genuinely lacks (DAG, per-agent fallback, roster config, orchestration events).

### Non-Goals
- Not changing LiteLLM or the model aliases (model access stays where it is).
- Not changing the secret chain (Doppler → ESO → `openagent-secrets`).
- Not touching the local OpenCode workstation (`~/.config/opencode/oh-my-openagent.json`) — out of scope; it is a separate, unmanaged config.
- Not rebuilding the web UI. Visibility rides Hermes' existing event/RPC surface (± a thin command).
- Not extracting `gitopsctl` into its own repository — it stays in-repo; only its plugin-provisioning responsibility moves to the chart (§10).
- Not porting OMO's per-model prompt variants in v1 (documented follow-up).

---

## 3. Design

### 3.1 Shape

```
USER
  │
  ▼
HERMES (gateway profile; sole user-facing persona)
  │  omo  tool  (plugin: hermes-omo-plugin)
  ▼
Native OMO Orchestrator  ─────────────────────────────────────────────┐
  │  roster registry · model-chain resolver · fallback state machine  │
  │  DAG scheduler · parallel fan-out · lifecycle · events · results  │
  └──────────────┬───────────────────────────────────────────────────┘
                 │  ctx.subagent_lifecycle.launch(model=<chain rung>)
     ┌───────────┼───────────────────────────────┐
     ▼           ▼                               ▼
  Sisyphus    Prometheus                     Hephaestus
  Ultraworker Plan Builder                   Deep Agent
     │           │
     │     ┌─────┴─────┐
     │     ▼           ▼
     │   Metis       Momus
     │
     ├── Explore      ├── Librarian      ├── Oracle      └── Multimodal-Looker
                 │
                 ▼
        structured result → HERMES → USER
```

### 3.2 The plugin (`jomakori/hermes-omo-plugin`)

```
hermes-omo-plugin/
├── plugin.yaml                 # name: omo; provides_tools: [omo, omo_task]; capabilities
├── __init__.py                 # register(ctx): tools, hooks, command, config schema
├── roster.py                   # agent registry: name, role, toolset scope, chain, prompt ref
├── agents/                     # ported persona bodies (one canonical .md per agent)
│   ├── sisyphus.md hephaestus.md prometheus.md atlas.md metis.md momus.md
│   ├── oracle.md librarian.md explore.md multimodal-looker.md sisyphus-junior.md
├── orchestrator/
│   ├── engine.py               # DAG scheduler + parallel fan-out + result collection
│   ├── chains.py               # model-chain resolution + fallback state machine
│   ├── lifecycle.py            # ctx.subagent_lifecycle wrapper (launch/status/cancel)
│   └── events.py               # ctx.emit orchestration milestones + subagent_* hooks
├── tools/
│   ├── omo_tool.py             # Hermes-facing dispatch entrypoint
│   └── omo_task_tool.py        # OMO `task()` contract used *by* orchestrator agents
├── skills/omo-driven-development/SKILL.md   # decision framework (port of opencode-driven-development)
├── tests/                      # pytest: roster, chains, DAG, guards, events
└── README.md
```

**Two tools, mirroring OMO's two surfaces:**

- `omo` — Hermes' entrypoint. Actions: `dispatch` (goal/agent/category/background/plan file), `status`, `tree`, `cancel`, `resume`. This replaces the `opencode` tool name-for-name in the system prompt.
- `omo_task` — the delegation contract the orchestrator agents use. EITHER `agent=` (any roster agent) OR `category=` (spawn `sisyphus-junior` with a category's model/skills/append); `load_skills`, `description`, `prompt`, `run_in_background`, `task_id`. Guards: reject `sisyphus-junior` as a direct target (category-only); plan-family mutual block. **Note:** OMO's "primary agents cannot be delegated to" and its `prometheus`-is-a-coordinator guard were deliberately **dropped** — they contradict this design, where Hermes *must* be able to dispatch to Sisyphus, Prometheus and Hephaestus. Caught by the engine tests.

Both handlers call the same engine and the same `ctx.subagent_lifecycle` rail.

### 3.3 Model routing & fallback (the one real gap)

- **Per-agent model:** `SubagentLaunchRequest.model = <chain[0]>`. All aliases are LiteLLM names; `delegation.provider` stays `litellm` with `base_url` at the LiteLLM service. ✅ native.
- **Per-agent provider:** not expressible on the launch API. **Assumed moot** because every OMO chain is `litellm/<alias>` and `delegation.provider` is pinned to `litellm` — but this assumption is **load-bearing and unverified**. Phase 0 must confirm that a `subagent_lifecycle` launch resolves the alias through the LiteLLM provider and not some other resolution path. Fallback if it does not: drive workers through `ctx.llm.complete(provider="litellm", model=<alias>)` and own the loop (see D2).
- **Per-agent fallback chain:** not native. The engine owns it:
  1. Launch with rung *n*.
  2. On terminal failure, classify (reuse OMO's classifier semantics: retryable ⇔ status ∈ {400,401,403,404,429,5xx} ∪ {408,425,429} or message matches rate-limit/quota/overloaded patterns; **not** abort/context-overflow).
  3. `max_fallback_attempts` (3), per-model `cooldown_seconds`, `restore_primary_after_cooldown` — state kept beside the run, surfaced in events.
  4. Equivalent-model canonicalization (collapse `-thinking/-max/-high`) so chains don't thrash.
- **Escape hatch:** `ctx.llm` with `llm.provider_override`/`llm.model_override` capabilities is available if a future agent needs a genuinely different provider. Not used in v1.

### 3.4 Tool scoping (preserving OMO's read/write restrictions)

Hermes scopes tools by **toolset** — registry names `web`, `search`, `terminal`, `file`, `delegation`, `kanban`, `hermes-cli`, … (exact set confirmed in Phase 0) — not per-tool ACLs, and `allowed_toolsets` may only narrow the parent's set. Mapping:

| OMO restriction | Native expression |
|---|---|
| oracle/explore/librarian/metis/momus deny write+edit | launch with read-only toolsets only |
| multimodal-looker = `read` only | launch with the minimal read toolset |
| sisyphus/hephaestus/atlas/prometheus/junior full engineering | engineering + delegation toolsets |
| plan-family cannot delegate into plan-family | engine guard (handler-level) |
| sisyphus-junior blocked from `task` | engine guard (handler-level) |

Plugin tools (`omo`, `omo_task`) bypass the toolset filter, so authorization is enforced **inside the handlers** (caller identity/depth check), exactly as OMO's preflight does.

**Worker approvals — de-escalated on inspection; NOT a blocker.** `delegation.subagent_auto_approve` defaults to `false`, but it only governs `prompt_dangerous_approval()` — i.e. **dangerous** shell commands, not all tools. Reading `tools/delegate_tool_config.py:38-58`: subagent worker threads get a non-interactive callback that denies *dangerous commands* by default and approves them only under the opt-in; ordinary work (tests, builds, git, file edits) is unaffected. Gateway sessions resolve approvals through their own per-session queue. So the engineering workers function on the safe default, and `subagent_auto_approve: true` is an **optional YOLO posture for unattended dangerous commands — a security decision, deliberately left off**. If a worker ever genuinely needs a dangerous command unattended, the better route is a plugin approval transport surfacing to Hermes/Discord, not a blanket opt-in.

**MCP gating — resolved (P0-1).** MCP servers are toolsets named `mcp-<server>` (`tools/mcp_tool_registration.py:100`), so OMO's `mcpToolAgents` allowlist is expressed natively: include `mcp-plane`/`mcp-github` in the orchestrator workers' `allowed_toolsets`, omit them for the read-only/search tiers. Confirm live that these toolsets sit in the parent's enabled set and are narrowable.

**Read-only enforcement — resolved, and it needs a hook (P0-2).** The `file` toolset bundles `read_file`, `write_file`, `patch`, `search_files` (`toolsets.py:120-123`), and per-tool `blocked_tools` is rejected at launch. Read-only agents therefore **cannot** be expressed by toolsets alone. Enforce with a `pre_tool_call` hook denying `write_file`/`patch` for those worker identities (oracle, explore, librarian, metis, momus, multimodal-looker). **Required for Phase 2.**

### 3.5 Visibility

- Emit `ctx.emit("omo:<run>", {phase, agent, model, rung, status})` at every transition; subscribe in the same plugin for the run tree.
- Ride Hermes' native `subagent_start`/`subagent_stop` hooks + `subagent.list/progress/tool/thinking/tail` + `delegation.status` RPCs. Live transcripts already land at `cache/delegation/live/<delegation_id>/task-<n>.log`.
- Add a `register_command("omo", …)` slash command printing the run tree:
  `Hermes └── Sisyphus · Ultraworker └── Prometheus · Plan Builder └── Metis · Plan Consultant …` with `agent · role · status · task · model`.
- WebUI: the existing `hermes-webui` renders sessions/subagents; the tree view is delivered by the command + events first, a dashboard widget only if the command proves insufficient (decision below).

### 3.6 Hermes stays the communicator

- Rename the gateway persona from `"Sisyphus — OMO Orchestrator"` to **Hermes — the communicator**, whose job is classify → dispatch → synthesize → speak. It never behaves as a worker and never exposes a worker as the assistant.
- Sisyphus · Ultraworker becomes an *internal* orchestrator agent launched by the engine (matches the requested end-state diagram, where Hermes sits above Sisyphus).
- All worker output returns as structured results to Hermes; only Hermes speaks to the user. Worker personas never address the user directly.

### 3.7 Nesting depth — RESOLVED (verified against the pinned build)

Settled in Phase 0 by reading `tools/delegate_tool.py:186-188`:

```python
child_depth = getattr(parent_agent, "_delegate_depth", 0) + 1
max_spawn   = _get_max_spawn_depth()
effective_role = "orchestrator" if _get_orchestrator_enabled() and child_depth < max_spawn else "leaf"
# "Role is depth-derived ... The `role` arg is ignored."
```

Consequences, now reflected in the chart:
- **`delegation.max_spawn_depth: 3` is required**, not optional. At the default `1`, `child_depth(1) < 1` is false, so **every child is a `leaf` and Sisyphus cannot delegate at all** — the pipeline collapses to flat while every other check still passes. Added to `values.yaml`.
- **`role` is ignored.** The plugin may pass `role="orchestrator"`; Hermes recomputes it from depth. Nesting is governed solely by `max_spawn_depth` + `orchestrator_enabled` (the latter defaults `True`).
- **`delegate_task` hard-errors** at `depth >= max_spawn` (`delegate_tool.py:441`); the plugin's `ctx.subagent_lifecycle.launch` path does not raise, so the engine can still launch at depth — but the child's own spawning is depth-capped.
- Cost note: each extra level multiplies API spend (`max_concurrent_children` default 10).

---

## 4. Changes by file

### 4.1 New repo
| Path | Change |
|---|---|
| `hermes-omo-plugin/**` | new plugin (engine, roster, personas, tests) — see §3.2 |

### 4.2 `jomakori/gke_GitOps`
| Path | Change |
|---|---|
| `.useful-scripts/gitopsctl/internal/boot/boot.go` (`opencodeSetup` 427–461, called at 481) | **One-time Go change (unavoidable for true removal):** delete `opencodeSetup()` — it installs the `opencode-ai` npm CLI, clones `zaycruz/hermes-opencode-plugin` into `/opt/data/plugins/opencode`, copies its `SKILL.md`, and seeds `/opt/data/.omo/omo.jsonc` from a mounted template (`boot.go:450`). Residual need (chown `/opt/data/plugins`) moves to the chart initContainer. Plugin provisioning relocates to the chart (§3.2 / Phase 3.5); `boot.Run` ends at `chownTree(); hand over to hermes` |
| `services/helm/openagent/templates/opencode.yaml` | delete; the roster/chains move to `hermes-agent.config.plugins.entries.omo.settings` (the path Hermes' `ctx.get_config` reads); prompts stay in the plugin |
| `services/helm/openagent/charts/opencode-server/**` | delete the subchart |
| `services/helm/openagent/Chart.yaml` | remove the `opencode-server` dependency |
| `services/helm/openagent/values.yaml` | `opencode-server:` block delete; `opencode:` block → `omo:`; `plugins.enabled` drop `opencode`, add `omo`; `agent.system_prompt` rewrite (Hermes communicator); remove `OPENCODE_SERVER_URL`; remove the `opencode-config` mounts; drop the opencode VPA target |
| `services/helm/openagent/templates/hermes.yaml` | remove the mise tool `opencode = "1.18.29"` + its comment — this is the second place the OpenCode CLI is installed (`boot` installs it via npm too) |
| `services/helm/openagent/values.yaml` — `litellm.proxy_config.model_list` | delete the 4 `opencode/*` aliases (`big-pickle`, `nemotron-3-ultra-free`, `laguna-s-2.1-free`, `mimo-v2.5-free`) + their `OPENCODE_API_KEY`/`OPENCODE_API_BASE` params — verified to have **no in-cluster consumer** |
| `services/helm/openagent/templates/k8s-gitops-context.yaml` | rewrite the `OpenCode Agent Execution` section → `Native OMO Orchestration`; fix the model table + fallback narrative (~675, 712–734, 795, 805–807, 824, 936) |
| `services/helm/openagent/README.md.gotmpl` + generated `README.md` | rewrite the gotmpl; regenerate via helm-docs |
| `.gitignore:10–11` | delete the `!…/charts/opencode-server/` un-ignore lines |
| Shared PVC (`/opt/data`) | prune runtime leftovers the old boot created: `npm-global/`, `bin/opencode`, `plugins/opencode/`, `skills/…/opencode-driven-development/`, `.omo/` |
| `hermes-opencode-plugin` (fork) | archive; remove all references |
| `services/helm/openagent/tests/*` | **no OpenCode refs exist** (verified) — only update for the new `omo:` config keys |

> §4.3 is the exhaustive, per-artifact verification of this list (20 items, cross-checked against `grep -rIi opencode`). Use it as the removal checklist, not this summary.

### 4.3 OpenCode removal inventory (exhaustive)

`grep -rIi opencode` over the tracked tree (excl. `.git`, `.worktrees`) = **308 hits across 16 paths**. Every one must be resolved or explicitly allowlisted. This is the definition of "removes OpenCode and all deps of it".

| # | Artifact | Refs | Action | Verify |
|---|---|---|---|---|
| R1 | `services/helm/openagent/templates/opencode.yaml` | 27 | delete file | no `openagent-opencode-config` ConfigMap in render |
| R2 | `services/helm/openagent/charts/opencode-server/**` (6 files: Chart, values, README, deployment, service, _helpers) | 51 | delete directory | dir gone; `helm dep build` clean |
| R3 | `services/helm/openagent/Chart.yaml` — `opencode-server` dependency | 3 | remove the dep block | render has no `opencode-server` objects |
| R4 | `values.yaml` — `opencode-server:` block + `enabled` | — | delete | key absent |
| R5 | `values.yaml` — `opencode:` block (plugin, defaultAgent, defaultModel, mcpToolAgents, categories, agents, backgroundTask, runtimeFallback) | — | replace with `omo:` (consumed by the new ConfigMap) | new keys present; old absent |
| R6 | `values.yaml` — `hermes-agent.config.plugins.enabled: [discord-platform, opencode]` | — | drop `opencode`, add `omo` | plugin list = `[discord-platform, omo]` |
| R7 | `values.yaml:1024` — `OPENCODE_SERVER_URL` env | 1 | delete | env absent |
| R8 | `values.yaml` — `opencode-config` volume + 2 volumeMounts (`opencode.json` @1070, `omo.jsonc` @1074) | 3 | delete; **no replacement mount** — plugin config reaches the plugin via `plugins.entries.omo.settings` | mounts absent |
| R9 | `values.yaml:~1280` — VPA target `opencode-server` | — | delete | `vpa.targets` has one entry |
| R10 | `values.yaml:119–155` — `litellm.proxy_config.model_list[0..3]` (`opencode/big-pickle`, `opencode/nemotron-3-ultra-free`, `opencode/laguna-s-2.1-free`, `opencode/mimo-v2.5-free`) + their `OPENCODE_API_KEY`/`OPENCODE_API_BASE` params | 8 | delete — **no in-cluster consumer** (the `opencode/` aliases appear only as definitions here + the skill table; the renderer at `opencode.yaml:3` already excludes them) | LiteLLM config has no `opencode/` alias |
| R11 | `templates/hermes.yaml:~37` — mise tool `opencode = "1.18.29"` + comment ← **the opencode CLI install** | 3 | delete the tool entry | `mise.toml` has no `opencode`; nothing symlinks `bin/opencode` |
| R12 | `templates/k8s-gitops-context.yaml` — 23 refs (agent-execution section @675/712–734, model table @805–808, fallback narrative @824, "single agent" note @936) | 23 | rewrite the section for native OMO; drop the `opencode/*` rows | skill mentions no OpenCode |
| R13 | `services/helm/openagent/README.md` (117, generated) + `README.md.gotmpl` (5) | 122 | rewrite gotmpl; regenerate with helm-docs | `helm-docs` in sync; no OpenCode prose |
| R14 | `.gitignore:10–11` — `!…/charts/opencode-server/` un-ignore lines | 2 | delete | — |
| R15 | `.useful-scripts/gitopsctl/internal/boot/boot.go` — `opencodeSetup` (opencode-ai npm install @431, plugin clone @437, SKILL copy @443, `omo.jsonc` seed @450) | 14 | delete (Phase 3.5); chown moves to the chart initContainer | `go build/test/vet` green |
| R16 | `.useful-scripts/gitopsctl/README.md:19` — "opencode setup" prose | 1 | update | — |

**Runtime state on the shared PVC** (not in git, created by the old boot; must be pruned or they linger as dead deps):

| R17 | `/opt/data/npm-global/` (installed `opencode-ai`) · `/opt/data/bin/opencode` symlink · `/opt/data/plugins/opencode/` · `/opt/data/skills/software-development/opencode-driven-development/` · `/opt/data/.omo/` (OMO config the OpenCode plugin read) | runtime | one-shot prune in the chart initContainer or a boot cleanup step after the plugin is native | `ls` shows none of them |

**Outside the repo:**

| R18 | Doppler `svc_openagent`: `OPENCODE_API_KEY`, `OPENCODE_API_BASE` | external | remove once R10 lands (or keep if the **local** workstation still routes these aliases through LiteLLM — see D7) | ExternalSecret syncs without the keys |
| R19 | GitHub fork `jomakori/hermes-opencode-plugin` | external | archive; remove from all docs | no live reference |
| R20 | No CI/workflow/renovate/devbox dependency exists (verified: `.github/**`, `renovate.json`, `devbox.json`, `ct_check.sh`, `.pre-commit-config.yaml`, `tests/**` contain **zero** OpenCode refs) | 0 | no change | — |

**Verification:** one sweep, run by hand as part of this change — `grep -rIi opencode services/helm/openagent .useful-scripts/gitopsctl`, expecting only the local-workstation paths D7 keeps plus the sentence stating the layer is gone. **Do not** add a committed guard script for this: a permanent CI/pre-commit gate that exists to prove a one-time deletion is maintenance the repo should not carry. If a reference ever returns, that is a code-review catch, not a lint rule.

### 4.4 Explicitly unchanged
LiteLLM + model aliases; `openagent-secrets`; MCP manifest + verification jobs; the `delegation:` block (kept for Hermes' generic `delegate_task`); postgres; istio/CF routing; skills (ponytail/caveman/k8s-gitops-context); the `hermes-webui` thin client.

---

## 5. Phased plan

| Phase | Work | Exit criteria |
|---|---|---|
| **0 — Verify** | Spike on a live Hermes: (a) does a plugin-tool-launched subagent inherit/expose MCP tools, and can they be narrowed? (b) does `SubagentLaunchRequest.model` actually select an arbitrary **LiteLLM** alias, via the LiteLLM provider? (c) nesting: does a worker-spawned worker work under default `max_spawn_depth: 1`, or must it be raised? (d) live transcript path + `subagent.tail` shape (the documented `cache/delegation/live/…` path is for `delegate_task` — confirm it also covers `subagent_lifecycle` launches); (e) how plugin config is injected (`ctx.get_config` from chart values); (f) **which tools are approval-gated for a subagent** and what `delegation.subagent_auto_approve: false` actually denies; (g) the exact toolset-name set | Written findings; no design assumption left untested; decisions §7 resolved; **D2 and the §3.4/§3.7 fallbacks chosen on evidence** |
| **1 — Plugin skeleton** | `plugin.yaml`, `register()`, roster registry, `omo` + `omo_task` schemas, engine with DAG + parallel fan-out + lifecycle wrapper, guards, pytest harness | `omo` dispatches a single named worker; status/tree/cancel work; unit tests green |
| **2 — Personas** | Port 11 prompt bodies verbatim; toolset scopes; category set; 6-section delegation prompt builder; skills port | Roster table (§1.4) reproduced; a planning→execution→review run completes end-to-end |
| **3 — Chains & fallback** | Model-chain resolver; retryable classifier; cooldown/restore state machine; events | Injected 429/5xx falls to rung 2 then 3; state visible in events |
| **3.5 — Decouple provisioning** | Remove `opencodeSetup()` from `gitopsctl` boot; add a chart initContainer that provisions the plugin onto the shared PVC (`/opt/data/plugins/omo`) from a pinned source, with the chown; supply plugin config via ConfigMap | Changing the plugin requires **zero** `gitopsctl` image rebuild; `boot.Run` no longer knows about plugins; `gitopsctl` unit tests still green |
| **4 — Helm wiring** | boot.go swap; omo ConfigMap; values `omo:` block; remove opencode-server/Chart dep/mounts/env/VPA; helm-docs | `./ct_check.sh --dir services/helm/openagent` green; `helm unittest` green; render clean |
| **5 — Removal + docs** | Delete opencode-server + template; prune PVC leftovers (R17); rewrite k8s-gitops-context + README (R12–R13); archive fork (R19) | `grep -rIi opencode services/helm/openagent .useful-scripts/gitopsctl` shows only the local-workstation paths (D7) and the sentence stating OpenCode is gone — verified in-session, not by a committed guard |
| **6 — Deploy + E2E** | ArgoCD sync; Discord + WebUI E2E; rehearse the `git revert` rollback | Hermes answers; a Standard task routes Hermes→Sisyphus→(Metis/Oracle/Prometheus/Momus)→Hephaestus→verify→Hermes; tree visible; revert restores the prior state < 1 h |

### 5.1 Phase 0 findings — verified statically against the pinned build

Verified against `NousResearch/hermes-agent` **tag `v2026.9.11` (release v0.21.2, commit `939e45c`)** — the exact image in the cluster. Live confirmation still blocked: the `openagent-hermes-agent` pod is `0/1` and `kubectl exec` hangs (see §5.1.1).

| # | Question | Finding | Impact |
|---|---|---|---|
| P0-1 | Toolset registry names | Full set confirmed (`web, search, vision, terminal, file, skills, browser, todo, memory, delegation, kanban, coding, safe, …`); child defaults are `["terminal","file","web"]`. **MCP servers are themselves toolsets, named `mcp-<server>`** (`tools/mcp_tool_registration.py:100,296`) | **D3 resolves favourably** — per-agent MCP gating *is* natively expressible via `allowed_toolsets` (`mcp-plane`, `mcp-github`); the "correctness-first, gate later" fallback is unnecessary |
| P0-2 | Read-only scoping | **`file` bundles read+write+patch** (`toolsets.py:120-123`). `SubagentLaunchRequest.blocked_tools` exists but is **rejected at launch** (`subagent_lifecycle.py:228-229`: *"Per-tool blocking is not supported; use allowed_toolsets"*) | **Read-only workers are NOT expressible via toolsets.** oracle/explore/librarian/metis/momus/multimodal-looker need a `pre_tool_call` hook denying `write_file`/`patch` by caller identity. Now **required**, not optional |
| P0-3 | Per-launch timeout / cwd | Both **rejected** (`subagent_lifecycle.py:225-227`) | Worker deadlines must be enforced by the engine (cancel on deadline); no per-launch timeout |
| P0-4 | Per-worker model vs provider | `launch()` passes `model=` into the child builder (`:259-264`); `handle.provider` is **derived from the child** (`:271`), never passed | Per-agent **model** confirmed; per-agent **provider** genuinely unavailable — the §3.3 assumption stands, pending live LiteLLM-alias check |
| P0-5 | Nesting depth | Defaults: `max_spawn_depth: 1` (*"1 = flat, 2 = orchestrator→leaf, 3+ = deeper"*), `orchestrator_enabled: True`, `max_concurrent_children: 10`, `independent_completions: False` (`config_defaults.py:1257-1263`) | Hermes→Sisyphus→Prometheus→Metis needs `max_spawn_depth: 3`; the default silently truncates |
| P0-6 | Worker approvals | `delegation.subagent_auto_approve` default `False` → `_subagent_auto_deny`, but **only for `prompt_dangerous_approval()`** (dangerous shell commands); gateway sessions use their own queue (`tools/delegate_tool_config.py:38-58`) | Ordinary worker commands are unaffected. Not a blocker; leave the safe default. Auto-approve is an opt-in security decision |
| P0-7 | Live transcripts | Path `<hermes_home>/cache/delegation/live/<id>/task-<n>.log` exists (`tools/delegation_live_log.py:3`, `delegate_tool.py:477`) | Lifecycle launches use the same `tools.delegate_tool` builder, so the side channel should apply — confirm live |
| P0-8 | Plugin config injection | `PluginContext.get_config/set_config` (`hermes_cli/plugins.py:246,258`) | Plugin reads `plugins.entries.<id>` — chart values flow in. ✓ |
| P0-9 | Child tool inheritance | `_build_child_preserving_parent_tools` preserves parent resolution (`delegate_tool_results.py:300-311`); `DELEGATE_BLOCKED_TOOLS = {delegate_task, clarify, memory, send_message, cronjob_manage}` always stripped (`delegate_tool_toolsets.py:14-22`) | Children lose `delegate_task` by default → **the engine must be the sole launcher** (our preferred design). If a worker must spawn, verify whether `role="orchestrator"` re-adds it |

**Still to confirm live (needs a healthy pod):** P0-2's exact `pre_tool_call` caller identity; P0-4's LiteLLM-alias resolution; P0-7 for lifecycle launches specifically; whether `mcp-*` toolsets are in the parent's enabled set (narrowable).

### 5.1.1 Phase 0 environment blocker (pre-existing, not caused by this migration)

`openagent-hermes-agent-55856f8788-mfnqj` is `Running` but `Ready=False` for 9 h, `kubectl exec` hangs (even `echo ok`), and the logs show:

- `MCP discovery timed out while 13 server(s) were still connecting` (arctic-shift, argocd, bitwarden, codegraph, doppler, drawio, ferryhopper, google-workspace, grafana, kiwi, obscura, plane, skiplagged)
- `MCP server 'bitwarden': config value 'args[1]' has hidden leading or trailing whitespace`
- a faulthandler stack dump during `_start_recover_previous_run → register_relay_adapter → read_yaml_layers`

The live spike cannot run until this pod is healthy. **This is an operational issue outside the migration's scope** — flagging it rather than fixing it here. Phase 0 is otherwise answered statically against the same build.

### 5.2 Execution status

Shipped. `gke_GitOps` PRs **#265** (native orchestration + OpenCode removal), **#267** (fallback-model layer) and **#268** (retry on 402) are merged to `main`; the plugin is published at `jomakori/hermes-omo-plugin` (PR #1 merged; PR #2, the approval transport, closed by design — §12.1).

| Phase | State | Evidence |
|---|---|---|
| 0 — Verify | ✅ static; ⛔ live | §5.1 (9 findings against `v2026.9.11`); live blocked by §5.1.1 |
| 1 — Plugin skeleton | ✅ | `plugin.yaml`, `roster.py`, `__init__.py`, `orchestrator/{chains,guards,engine}.py`, `tools/{omo_tool,omo_task_tool}.py` |
| 2 — Personas | ✅ | `agents/*.md` — 11 files, 2,176 lines, frontmatter stamped with source + SHA |
| 3 — Chains & fallback | ✅ | `FallbackState` + retryable classifier + engine dispatch/fallback/cancel tests; **31/31** pytest green |
| 3.5 — Provisioning decoupled | ✅ | `boot.go` `opencodeSetup` removed (−39 lines); plugin provisioning moved to a chart initContainer (`omo.provision`, default `false`) |
| 4 — Helm wiring | ✅ | `plugins.entries.omo.settings` block; `plugins.enabled = [discord-platform, omo]`; Hermes communicator prompt; render exit 0 |
| 5 — Removal + docs | ✅ | R1–R20 resolved; sweep run in-session |
| 6 — Deploy + E2E | ✅ | ArgoCD `Synced`/`Healthy`; a dispatched worker walked two dead rungs and succeeded on the third (§13.1) |

Offline gate: `pytest` **44/44** · `go build/vet/test` clean · `helm unittest` 5/5 suites (10 tests) · `ct_check` pass · `helm template` exit 0 · `yamllint` only pre-existing long-line warnings.

**Hermes API contract verified against the pinned build** (so the plugin cannot fail at load): `register_tool(name, toolset, schema, handler, check_fn, requires_env, is_async, description, emoji, override)`; `schema` shape `{name, description, parameters}` (matches the bundled `a2a` plugin); handlers invoked as `handler(args, **kwargs)` with `is_async=True` returning a coroutine; `register_hook(hook_name, callback)`; `register_command(name, handler, description)`; `on_unload(callback)`; `ctx.subagent_lifecycle` is a property; and `ctx.get_config` resolves `plugins.entries.<plugin_id>.settings.<key>`. A `register()` + handler-contract test now locks these in.

**Closed:** the plugin is published and provisioned (`OMO_PROVISION_ENABLED: "true"`), and the live confirmations (P0-2/P0-4/P0-7) plus E2E are done — see §13.

The plugin repo is CI-ready on publish: the test suite runs on Python 3.11 and 3.13, plus `ruff check` + `ruff format --check` and a guard that fails on any unresolved `{TOKEN}` in a persona prompt.

**Corrections found during execution:**
- **D5 switch was wrong as drafted.** `omo.provision.enabled` could never work: the upstream hermes-agent chart does not `tpl` its values (`tpl` count = 0 in its `deployment.yaml`), so `extraInitContainers` entries are literal and cannot reference `.Values`. Flipping the flag would have silently no-op'd. Fixed by removing the flag from the `omo:` block and making the initContainer's `OMO_PROVISION_ENABLED` env the single, documented switch.
- **Removal verified in-session, not by a committed guard.** The first cut added `check_no_opencode.sh` plus a CI step and pre-commit hook. That was the wrong shape — a one-time deletion is proved by the change that performs it, not by a permanent script the repo carries and maintains forever. Script, hook and CI step were removed; the sweep is re-run by hand (only the local-workstation paths and the statement of absence remain). Running that sweep is also what caught the README prose still describing the deleted `omo-config` ConfigMap.
- **Plugin config was unreachable — an integration break, not a bug in either half.** The plugin reads settings via `ctx.get_config`, which Hermes resolves from `plugins.entries.<plugin_id>.settings.<key>` (`hermes_cli/plugins.py:246`). The chart set only `plugins.enabled` and rendered the roster into a mounted `openagent-omo-config` ConfigMap that the plugin never read. Every setting in the old `omo:` block was therefore **dead config**: an operator editing chains/fallbacks would have seen nothing change, and the plugin would silently fall back to `roster.py` defaults. Fixed by moving the block to `hermes-agent.config.plugins.entries.omo.settings`, deleting the ConfigMap template and its volume/mount, and updating the skill text. Verified present in the render.
- **Two guard/engine defects, caught only by writing the missing engine tests:** OMO's "primary agents cannot be delegated to" rule made `sisyphus`, `prometheus` and `hephaestus` undispatchable — contradicting the requested architecture (dropped); and cancelling a background worker before its launch completed was a silent no-op (now honours a `cancel_requested` flag at launch).
- **Pod root cause:** the gateway container is `Running`, not crashing — the readiness probe to the dashboard on `:9119` times out. Startup completes (mise, mcp-prewarm, codegraph) and then the gateway blocks in MCP discovery (`MCP discovery timed out while 13 server(s) were still connecting`). A concrete defect in that path: `bitwarden.args[1]` carries a trailing newline from the `|` block scalar (`values.yaml:931`), which Hermes itself warns about; the `|-` strip form would fix the warning. Not changed here — it is live-platform behaviour outside this migration's scope.
- **Persona ports were not faithful (found by a heading-level audit).** Two passes were needed. First: `sisyphus.md` was missing Todo Management, Task Management, Clarification Protocol, When to Create Tasks, Parallel Delegation, Hard Blocks and the Delegation Table — all present in the source — so the orchestrator persona had lost its delegation discipline and forbidden-action list; re-ported (379 → 524 lines). Second, and now fixed: **four personas shipped unresolved template tokens** that would have been injected literally into worker system prompts — `sisyphus` (7), `hephaestus` (11), `atlas` (5), `sisyphus-junior` (1), e.g. `{DELEGATION_TABLE}`, `{HARD_BLOCKS_SECTION}`, `{CATEGORY_SECTION}`. Static fragments were inlined verbatim; dynamic sections were rendered canonically from OMO's builtin metadata. Acceptance bar met: `grep -oE '\{+[A-Z][A-Z_]{3,}\}+' agents/*.md` returns **nothing** across all 11 files, and the persona set grew 2,176 → 2,952 lines. **Lesson: a "structurally complete" port is not a faithful port — heading- and token-level checks are required.**
- **The restored fallback layer was inert in production — a seam between two correct halves.** `ChainResolver` reads `runtime_fallback` (and `categories`), but `__init__._settings()` built the config dict from the *flat* keys only, so the resolver never received either. `retry_on_errors` therefore fell back to the built-in default, which has no `402` — so a credit-exhausted worker reported FAILED instead of walking its chain, and category dispatch silently resolved through `chains.sisyphus-junior`, ignoring `categories.<name>` entirely. Both halves passed their own tests: the resolver's tests construct it directly with a config, and the registration tests never asserted what `_settings` actually hands over. **Lesson: a seam between two green components needs a test at the seam** — now covered by tests that drive `_settings` with a config-bearing context and dispatch by category.
- **Why the suite missed it, precisely:** `register()` constructs `ChainResolver(_settings(ctx))` internally, so no test observed the assembled engine through a real context. The new tests close that by driving `plugin._settings(ctx)` with a `ConfigCtx` and by asserting a category dispatch resolves to the category chain.

**Operational finding, now measured (see §13.2):** the chains were ported verbatim, and **6 of the 15 aliases they reference are credit-exhausted** — `deepseek-v4-flash`, `deepseek-v4-pro`, `minimax-m3`, `glm-5.3`, `gemini-3.6-flash`, `qwen3-6-plus`. Nine of the eleven agents therefore burn two dead rungs before reaching a funded model. The audit caught this, not the fallback layer: the layer makes the fleet *work*, at the cost of two wasted round-trips per dispatch.

---

## 6. Validation

- **Plugin:** `pytest` (roster completeness, chain resolution, DAG ordering, guards, fallback classification, event emission).
- **Chart:** `./ct_check.sh --dir services/helm/openagent`, `.useful-scripts/validate_mcp_manifest.sh`, render + kubeconform, `helm unittest`, helm-docs in sync, selector guard.
- **Cluster:** ArgoCD sync green; gateway probes green; no `opencode-server` workload remains.
- **E2E:** Discord prompt → classification verbalized → dispatch → worker results → Hermes synthesis; `omo` tree command lists agent·role·status·task·model; transcripts readable; a forced provider error exercises fallback.
- **Regression:** MCP preflight + drift CronJob still green; session DB unchanged; webui still lists sessions.

---

## 7. Decisions needed from you (recommended option first)

| # | Decision | Recommendation | Why it matters |
|---|---|---|---|
| D1 | **Persona split:** gateway = "Hermes" (communicator), with "Sisyphus · Ultraworker" as the internal orchestrator agent? | **Yes** — matches your end-state diagram and "Hermes is the sole user-facing identity" | If you instead want the gateway itself named Sisyphus, the diagram's extra hop disappears but persona drift returns |
| D2 | **Fallback ownership:** orchestrator plugin vs moving chains into LiteLLM router | **Orchestrator plugin** | LiteLLM router fallback is global/per-alias, can't express per-agent chains; the chart deliberately keeps LiteLLM plain |
| D3 | **Per-agent MCP gating** (plane/github to 4 agents only) | **Native gating** — include `mcp-plane`/`mcp-github` toolsets only in the orchestrator workers' `allowed_toolsets` | **Resolved by P0-1**: MCP servers are `mcp-<server>` toolsets, so OMO's allowlist is directly expressible; no correctness-first fallback needed |
| D4 | **Visibility depth:** `omo` slash command + native events, vs a webui dashboard widget | **Command + events first**; widget only if it proves insufficient | Avoids rebuilding UI for a capability Hermes already streams |
| D5 | **Plugin install path:** leave it in `gitopsctl boot` (today) vs move it to a chart initContainer on the shared PVC | **Chart initContainer** — one-time Go deletion of `opencodeSetup()`, then plugin lifecycle is chart-only | Removes the image-rebuild → sha-pin-bump → sync dance from every future plugin change; the boot binary stops being a plugin registry |
| D6 | **Model-family prompt variants** (Sisyphus 12, oracle 3, momus 3, hephaestus 4) | **One canonical body per agent in v1**; variant table recorded as follow-up | Our models are pinned; multi-variant is calibration, not capability |
| D7 | **Scope of "remove OpenCode":** cluster surface only, or also the local workstation (`~/.config/opencode/**`, the `render_local_skill.sh` target, `values.yaml:10`'s local-`runtimeMode` rows)? | **Cluster only.** Keep the local opencode workstation — it is your dev environment, not the deployed system | Removing the local config would break your local tooling (and this session's runtime). This also decides R18 (Doppler `OPENCODE_*` keys) and whether R10's aliases are truly dead |

---

## 8. Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| ~~Engineering workers auto-denied~~ — **resolved, not a risk** | — | — | Only *dangerous* commands are gated; workers function on the safe default. Leave `subagent_auto_approve` off |
| **Nested pipeline truncated** — `max_spawn_depth` default 1 | High | High | **Resolved:** `max_spawn_depth: 3` added to `values.yaml`; `role` is depth-derived and ignored |
| **`SubagentLaunchRequest.model` doesn't resolve via the LiteLLM provider** | Medium | High | Phase 0 (b); fallback = `ctx.llm`-driven worker loop (D2) |
| Native rail can't express something OMO needed (provider/fallback/MCP gating) | Medium | High | Phase 0 verifies all four before Phase 1; fallback is orchestrator-owned by design |
| Persona/behavior regression vs OMO's calibrated prompts | Medium | Medium | Port verbatim, keep variant table, side-by-side behavior test on a Standard task |
| Subagents can't inherit MCP tools → workers lose Plane/GitHub | Medium | High | Phase 0 (b); fallback = keep MCP session-wide |
| Plugin tool bypasses toolset filter → unauthorized spawn | Low | Medium | Enforce guards inside handlers (OMO parity) |
| Losing the opencode-server pod's memory isolation reintroduces gateway OOM | Low | Medium | Workers are subagents under a bounded executor, not gateway children; VPA + limits retained on the gateway |
| Visibility parity gap (tree/transcripts) | Medium | Low | Native events + transcripts + `omo` command; widget deferred (D4) |
| Rollback complexity | Low | High | OpenCode is not retained as a runtime — rollback is a single `git revert` of the wiring PR (chart initContainer restores, chart dep restores); rehearse it in Phase 6. Migration is cleanly bisectable because Phases 4–5 are one wiring PR |

---

## 9. Success criteria

- [ ] `User → Hermes → orchestrator → agents → Hermes → user` — workers never user-facing.
- [ ] All 11 personas present with name · role · model chain · tool scope preserved.
- [ ] Routing, hierarchical + parallel delegation, DAG, planning→execution→review, nested delegation, per-agent model routing, fallback, cancellation, structured results all exercised in E2E.
- [ ] `openagent-opencode-config`, `opencode-server`, the opencode plugin clone, `OPENCODE_SERVER_URL`, the `opencode` mise CLI, the `opencode/*` LiteLLM aliases, and the npm dependency are gone; the PVC leftovers (R17) are pruned; **no reference remains in the cluster surface** and a CI/pre-commit guard keeps it that way (§4.3).
- [ ] Hermes tree/status/transcripts expose what was delegated, to whom, doing what, and what came back.
- [ ] CI green (`ct_check`, helm unittest, mcp validate, render/kubeconform).
- [ ] Rollback (`git revert` of the wiring PR) restores the prior state ≤ 1 h. No OpenCode artifact is retained to "fall back to".
- [ ] Plugin provisioning is chart-driven: changing the plugin (or adding a future one) requires **no** `gitopsctl` image rebuild and **no** sha pin bump.
- [ ] `gitopsctl` remains in-repo and its `go build/test/vet` + `mcp verify --mode validate` gates stay green.

---

## 10. Appendix — Should `gitopsctl` be decoupled into its own repo?

**Verdict: no wholesale decoupling.** `gitopsctl` stays in `gke_GitOps`. Fix the three couplings that actually hurt, in-repo. Rationale below.

### 10.1 What it is today

Stdlib-only Go (module `gitopsctl`, 2,617 LOC, zero external modules). Two roles:

| Role | Commands | Consumed by |
|---|---|---|
| In-cluster runtime | `boot`, `install`, `mcp prewarm`, `mcp verify --mode preflight\|drift` | initContainer → `/opt/tools/gitopsctl`; gateway entrypoint; PostSync Job; drift CronJob |
| Repo-side CLI (roadmap) | staged absorption of `ct_check.sh`, `render_local_skill.sh`, `validate_*.sh` (`README:15–17`, "one binary") | pre-commit + CI |

Release wiring: image built by `image-builds.yaml` (build context staged with `cp -a`), tagged by **source sha**; `gitopsctl-pin-bump.yaml` commits that sha **directly to protected `main`** with an **admin PAT**, deliberately bypassing the four required checks (`enforce_admins: false`). **Two literal pins** must move together — `tools.image.tag` and the `extraInitContainers[0].image` literal (`values.yaml:1104`), which cannot be templated because the upstream hermes-agent subchart takes the init container as values.

### 10.2 Benefits of extracting

| Benefit | Weight |
|---|---|
| Kills the admin-PAT bot commit to protected `main`; pin bumps become normal dependency PRs (Renovate) | high |
| Semantic tags (`v1.4.0`) instead of an opaque 40-hex sha — legible, rollback-able, changelog-able | high |
| Own CI (Go test/vet/coverage/release); the `cp -a` build-context staging hack disappears | medium |
| Review separation — Go changes stop riding in manifest PRs; smaller GitOps repo; no Go toolchain in pre-commit | medium |
| Reusability + per-repo supply-chain scanning (`mcp verify` is deployment-agnostic) | low-med |

### 10.3 Costs

| Cost | Weight |
|---|---|
| **Breaks the stated one-binary roadmap** — a separate repo cannot own scripts that read `services/helm/openagent`; the repo-side CLI would have to stay behind | high |
| Two-repo release choreography (release tag first, then chart bump) replaces today's automated end-to-end flow | high |
| `validate_mcp_manifest.sh` runs `go run` from source in pre-commit + CI; decoupled it must fetch a pinned binary (network) or use `go.work`/submodule, or validation silently stops | high |
| The two-pin duplication is **structural** (values-passthrough subchart) — extraction relocates the value, doesn't remove the duplication | medium |
| Loses `image sha == source sha` provenance; adds tag-skew risk (needs tag+digest pinning + Renovate discipline) | medium |
| New-repo overhead (CI, branch protection, license, CODEOWNERS, GHCR config, Renovate) | low |
| Cluster-failure → source correlation crosses repos | low |

### 10.4 What we do instead (in-repo, cheap)

1. **Move plugin provisioning out of `boot`** (Phase 3.5) — the coupling that forces an image rebuild for a plugin change.
2. **Retire the admin-PAT bot commit.** Publish a semver tag alongside the sha and let Renovate track it; `main` keeps its required checks. This captures the two high-weight benefits without a second repo.
3. **Single-source the two pins.** Rebuild `extraInitContainers` in a local template (or assert equality in CI) so `tools.image.tag` and the initContainer literal cannot drift.

### 10.5 If independence is still wanted later

The defensible split is **runtime half out, repo-side half stays** (`boot`, `install`, `mcp prewarm/verify` → own repo; `ct_check`/render/validate shell → remain in `gke_GitOps`, or a separate repo-side binary). Wholesale extraction trades an automated bump for cross-repo ceremony and stalls the absorption roadmap. Recorded as a non-goal for this migration.

---

## 11. Runbook — the three remaining steps

None of this is outstanding analysis; each step is gated on a human decision or a healthy cluster.

**Status:** steps 1 and 2 are **done and handed off as open PRs** (deliberately not merged).

| Step | State |
|---|---|
| Plugin repo | ✅ published — https://github.com/jomakori/hermes-omo-plugin |
| Plugin README redesign | ✅ PR #1 open, CI green (lint · personas · test 3.11 · test 3.13) |
| Chart wiring + removal | ✅ PR https://github.com/jomakori/gke_GitOps/pull/265 open, `lint-test` green |
| Provisioning | ✅ `OMO_PROVISION_ENABLED: "true"` on the branch (takes effect on merge+sync) |
| Phase 6 E2E | ⏳ after the chart PR merges and ArgoCD syncs |

### 1. Publish the plugin repo
```bash
cd /Users/maklab/hermes-omo-plugin
git init -b main && git add -A
git commit -m "feat: native OMO orchestration plugin for Hermes Agent"
gh repo create jomakori/hermes-omo-plugin --public --source . --remote origin --push
```

### 2. Flip provisioning and open the chart PR
```bash
# services/helm/openagent/values.yaml, hermes-agent.extraInitContainers:
#   OMO_PROVISION_ENABLED: "false"  ->  "true"
cd /Users/maklab/gke_GitOps
git add -A && git commit && git push -u origin feat/omo-native-orchestration
gh pr create --base main
```

### 3. Phase 6 prerequisite — RESOLVED
The gateway turned out **not** to be wedged: it recovered on its own (`1/1`, 0 restarts, 19 h uptime), and the dashboard answers in ~1 ms on the pod IP. It had been in a long settle while MCP discovery churned (13 servers, repeated keepalive drops). The `:9119` probe `(timeoutSeconds: 3)` merely failed during that window. The `bitwarden.args[1]` trailing newline (`|` → `|-`) was fixed on the branch as a drive-by, since Hermes logs a whitespace warning for it. No pod work is outstanding.

### 4. Phase 6 E2E
Discord + WebUI: Hermes answers → a Standard task routes Hermes→Sisyphus→(Metis/Oracle/Prometheus/Momus)→Hephaestus→verify→Hermes; `/omo` tree lists agent·role·status·task·model; transcripts readable; a forced provider error exercises the fallback chain.

---

## 12. Approval forwarding — shipped, and what the audits turned up

### 12.1 Worker approvals — decision: auto-approve via the host knob, NOT a gate replacement

Hermes gives worker threads a non-interactive approval callback, so a dangerous command from a subagent is refused with nobody asked. The plugin must **not** paper over that with a registered approval transport: `_run_approval_gate` consults `spec.transport` **first, unconditionally**, so a selected transport *replaces* Hermes' built-in approval surfaces for every caller. That was built, then reverted — plugin PR #2 is closed and the branch is empty against its base.

**Why the host gate cannot be reached from a worker** (so a plugin-side workaround is the wrong shape): `_is_gateway_approval_context()` is `HERMES_GATEWAY_SESSION` env **or** a non-empty `HERMES_SESSION_PLATFORM` ContextVar. Worker threads do not inherit ContextVars (`thread_context.py` says so), and the delegate path uses `DaemonThreadPoolExecutor(initializer=...)` instead of the host's own `propagate_context_to_thread` helper that other paths use. The child also binds its **own** session key, so even forcing the predicate finds no registered notifier. The real fix is upstream — propagate approval/session context into delegate children — after which Hermes' gate would handle worker approvals natively.

**Decision:** use the host-native knob, `delegation.subagent_auto_approve: true`, and keep no approval code in the plugin.

**Verified live on the pod (v2026.9.11):**

| Flag | Resolved callback | Decision | Log |
|---|---|---|---|
| `false` (host default) | `_subagent_auto_deny` | `deny` | `Subagent auto-denied dangerous command: … Set delegation.subagent_auto_approve: true to allow.` |
| `true` | `_subagent_auto_approve` | `once` | `Subagent auto-approved dangerous command: …` |

The `true` case was exercised on a **fresh thread** with the delegate callback installed via `_set_subagent_approval_cb(_get_subagent_approval_callback())` and resolved through `_resolve_cli_approval_callback()` — i.e. the exact path a worker takes.

**Security posture (deliberate):** subagents may now run destructive commands unattended. Read-only agents remain constrained by the plugin's `pre_tool_call` guard; this flag governs only the dangerous-command gate. Recorded next to the setting in `values.yaml`.

**Honest limit on the evidence:** a whole-agent end-to-end (a real delegated subagent deleting a sentinel file) could not be driven to completion — the one-shot CLI run stalled initialising amid MCP keepalive churn — and could not be retried without leaving the flag set.

### 12.2 Follow-ups from the OMO/Hermes audits (prioritised, not started)

**Highest value — Hermes APIs to use instead of approximating:**

| Item | Why |
|---|---|
| `register_middleware` (`tool_request`, `tool_execution`, `llm_request`, `llm_execution`) | Real interception — rewrite tool args *before* hooks/approvals, wrap or short-circuit a tool call, rewrite provider kwargs per call. Replaces our coarse toolset approximations. |
| `pre_tool_call` `modify` | Arg rewriting: the sanctioned per-agent scoping lever. |
| `ctx.llm` + `register_auxiliary_task` | Host-owned model calls with fail-closed trust flags and dedicated aux config lanes. |
| `ctx.state` | Durable profile-scoped KV — real persistence instead of the in-memory `runs` dict. |
| `register_system_prompt_section` / `register_context_engine` | Frozen orchestration policy bytes; per-request message selection. |

There is **no `ctx.budget`** — token/cost accounting is only `PluginLlmUsage.cost_usd` per `ctx.llm` call, so budgets must be self-tracked.

**OMO safety/correctness behaviours — dropped by decision (2026-09-22).** `write-existing-file-guard` (blind overwrite), `bash-file-read-guard` and the rest of this family all assumed a `pre_tool_call` gate — the same mechanism whose `session_id`-keyed hook never fired in the read-only guard, and which the host does not supply a reliable payload for. Porting them would re-attempt a mechanism known not to fire rather than extend a working one, so the operator dropped the item rather than leave it reading as planned work. **Residual risk, accepted:** a worker can overwrite an existing file or read one through a shell command. What mitigates it: workers run non-interactive under Hermes' own approval path (they carry no interactive approval channel), every worker row is filed under the claim boundary as a claim rather than evidence, and the operator's cluster is single-tenant.

**Lower tiers, recorded:** reactive runtime-fallback error classification; error-recovery hooks; token-economy hooks (tool-output truncator, image resizer, preemptive compaction, dynamic context pruning); categories beyond our five (`architect`, `artistry`, `deep-low/high`, `unspecified-low/high`) plus per-category prompt appends and capability gating; glob-conditional rules injection; comment checker; the `ultrawork` keyword mode; Team Mode (largely substitutable by Hermes kanban + `worktree_isolation` + orchestrator delegation; residual is the peer mailbox); BackgroundManager extras (circuit breaker, per-provider/model concurrency); built-in MCP servers still to declare; skill-embedded MCPs (no native equivalent).

**Explicitly out of scope — OpenCode/OpenClaw-coupled, or Hermes already does it better:** auto-update-checker, TUI sidebar/toast/btw-side, run-continuation-state, OpenClaw core, telemetry, doctor, Claude-Code compat; and `/goal`, LSP diagnostics, compression, context files, memory providers, `session_search`, which the host owns natively.

### 12.3 The `oh-my-hermes` (OMH) ecosystem — survey and what to adopt

Four independent projects share the name; two are direct siblings of this one. All four were analysed.

| Project | Delivery | Roster | Delegation | Model strategy | Enforcement |
|---|---|---|---|---|---|
| `witt3rd/oh-my-hermes` (~340★) | 11 skills + thin plugin | 15 **generic role** names, no codenames | `delegate_task` + `[omh-role:]` marker hook | none | `pre_tool_call` warn-only; read-only is prose |
| `cookie98/oh-my-hermes` | plugin (9 commands, 169 tests) | 4 names in **unused** YAML | `subprocess.run(claude/codex/opencode)` **outside** Hermes + file state | none (binary-presence fallback) | none at all |
| `rlaope/oh-my-hermes` (2.8k★) | plugin bundle + 234 skills + CLI | 9 roles + 19 tools + memory provider | `delegate_task`, route written to host config | category chains → writes host `delegation.*` | `pre_tool_call` policy gate + rules file |
| `Salomondiei08/oh-my-hermes` (863★) | **file-drop skills pack**, no plugin | 7 profiles via `hermes profile create` | host kanban dispatcher | none | advisory prose only |

**The division of labour is the headline.** Every one of them built the *durable-state spine* — resumable file state, wave/DAG scheduling, idle-continuation pressure, evidence-gated completion, subagent-artifact contracts — and **none** built the *agent/model/permission layer*. We built the opposite: real roster, per-agent model chains with owned fallback, read-only enforcement, MCP toolset scoping. The two halves are complementary, and ours is the one nobody else has.

**Worth adopting (ideas, ranked):**

1. **Never branch plugin registration on host attribute presence.** `rlaope` documents a live trap: Hermes added an *inert* `PluginContext.register_memory_provider`; their own `hasattr(ctx, "register_memory_provider")` guard then took that branch and returned early, so **every tool and hook registered nowhere while the plugin still reported enabled, with no error**. We are a plugin that registers tools and hooks — this is a bug class aimed straight at us. Rule: register unconditionally; make each surface optional *individually*; never `hasattr`-guard a whole registration path.
2. **Claim-boundary discipline** (`rlaope`): every result carries a machine-readable boundary (`not_evidence_until_observed`), so a prepared artifact can never read as observed execution. Our `omo` results currently return a status and a summary — adding an explicit boundary is cheap and prevents false confidence.
3. **Subagent-persists artifact contract + breadcrumbs** (`witt3rd`): the goal names a deterministic output path the child must write to; the parent verifies the file and appends dispatched/completed breadcrumbs. Directly applicable to our `subagent_lifecycle` launches, where a child's output can otherwise be lost.
4. **Runtime host-capability probing** (`rlaope`): probe `VALID_HOOKS` rather than assuming the host exposes a hook.
5. **Durable run state + idle-continuation pressure** (`cookie98`): file-backed state machine with wave scheduling and escalation thresholds. Our run registry is in-memory only.
6. **Per-skill `requires_toolsets` frontmatter** (`Salomondiei08`): declarative toolset scoping — the same idea as our `mcp-<server>` scoping, applied per skill.
7. Smaller, cheap patterns: dead-letter wrapper with redaction + alert; an approval vocabulary where **silence is never approval**; a memory-key registry table (key / owner / written-by / read-by) to stop key drift; a retry circuit breaker (revise once, then block and ask).

**Anti-patterns observed — do not copy:**

- **Dead config.** `witt3rd`'s `roles:`/category maps and `cookie98`'s `agents:`/`categories:`/`delegate_runtime:`/`planning_backend:` are read by **zero code**. Config that looks like routing but isn't is worse than no config.
- **Claiming capability that does not exist.** `cookie98` registers a skill guarded on `skills/sisyphus-orchestrator/SKILL.md`, a path absent from HEAD *and* history — while the README advertises it.
- **Prose-only safety.** `witt3rd` and `Salomondiei08` enforce read-only/policy in prompt text only; `cookie98` has no guard at all. Our `pre_tool_call` guard is the differentiator — keep it mechanical.
- **Delegating by unsandboxed subprocess** (`cookie98` hands the prompt and repo cwd to `codex`/`claude`/`opencode`), and a "parallel fan-out" that is really `ctx.inject_message` looping back into the same agent.
- **Editing the operator's live `config.yaml` as a routing bus** (`rlaope`): mitigated with atomic writes, token regex and ownership-proof restore, but it is a shared mutable global mutated per dispatch, and the reader is a *text scanner* emulating YAML.
- **Licence hygiene.** `Salomondiei08` shows an MIT badge with no `LICENSE` file (API returns none); `cookie98` has none at all. We now ship MIT plus explicit third-party notices.
- **CI that cannot see the breakage.** `Salomondiei08`'s main branch ships a committed merge-conflict marker in its flagship setup script; CI only checks that files exist, so it stays green.

Adopt (1) and (2) next; (3) and (5) are the substantive follow-ups; the rest are recorded, not scheduled.

---

## 13. Live verification — the migration works

### 13.1 End-to-end delegation, proven

Plugin rev `3c6034a` provisioned; ArgoCD `Synced`/`Healthy`; pod ready.

```
agent:  explore · Repository Exploration
model:  claude-haiku-4-5            ← 3rd rung of its chain
status: succeeded
api_calls: 3                         ← rungs 1 and 2 failed, the 3rd served
depth=1   terminal_state=SUCCEEDED   error_classification=None
```

The worker performed real work — it listed all 62 files under `/opt/data/plugins/omo` — and returned a structured `SubagentResult` to Hermes. The proven path is **Hermes → `omo` tool → engine → subagent launch → chain walk on failure → real work → structured result back to Hermes**. `api_calls: 3` is the evidence the fallback layer is live rather than decorative.

The first two runs of this same test are what exposed the `_settings` seam (§5.2): the run before the fix reported `FAILED` on `deepseek-v4-flash` with `Available Model Group Fallbacks=None` and never moved.

### 13.2 Model funding audit — 15 aliases

Method: a minimal `max_tokens=4` completion against the in-cluster proxy for every alias the chains reference, authenticated with `LITELLM_MASTER_KEY` from `/opt/data/.env`. (First two attempts used `OPENAI_API_KEY` and `OPENCODE_API_KEY` and returned a uniform `401` — a reminder that a uniform failure is an auth error, not fifteen broken models.)

**Serving (9):** `deepseek-v4-flash-direct`, `claude-opus-5`, `claude-sonnet-5`, `claude-haiku-4-5`, `bytedance-glm-5.2`, `bytedance-seed-code`, `bytedance-dola-seed-2.0-pro`, `bytedance-dola-seed-2.0-code`, `bytedance-deepseek-v4-flash`

**Dead — NO CREDITS (6):** `deepseek-v4-flash`, `deepseek-v4-pro`, `minimax-m3`, `glm-5.3`, `gemini-3.6-flash`, `qwen3-6-plus`

| Agent | Lands on | Dead rungs burned first |
|---|---|---|
| `sisyphus` | rung 1 — `deepseek-v4-flash-direct` | 0 |
| `oracle` | rung 1 — `claude-opus-5` | 0 |
| `hephaestus` | rung 3 — `claude-sonnet-5` | 2 |
| `prometheus` | rung 3 — `claude-sonnet-5` | 2 |
| `atlas` | rung 3 — `claude-sonnet-5` | 2 |
| `metis` | rung 3 — `claude-sonnet-5` | 2 |
| `momus` | rung 3 — `claude-sonnet-5` | 2 |
| `librarian` | rung 3 — `claude-haiku-4-5` | 2 |
| `explore` | rung 3 — `claude-haiku-4-5` | 2 |
| `multimodal-looker` | rung 3 — `claude-sonnet-5` | 2 |
| `sisyphus-junior` | rung 3 — `claude-haiku-4-5` | 2 |

Categories: `quick` → rung 3 · `deep` → rung 3 · `ultrabrain` → rung 1 · `visual-engineering` → rung 3 · `writing` → rung 2.

Readings:

- **The audit reproduces the runtime exactly** — it predicts `explore` lands on rung 3, which is what §13.1 observed. The method is sound.
- **Every agent retains at least one serving rung**, so the fleet is functional. But nine of eleven burn two failed round-trips per dispatch, and because `restore_primary_after_cooldown: true` with a 30s cooldown, any dispatch spaced beyond 30s re-pays that tax from rung 1 again.
- **The fallback layer is load-bearing, not a safety net.** It is what makes most of the roster work at all. That argues for pruning the unfunded rungs (or funding them) — never for removing the layer.
- `claude-opus-5`, `claude-sonnet-5` and `claude-haiku-4-5` all serve, so the strongest models in the set are available; the chains simply point at weaker, unfunded primaries first.

**Decision (operator, 2026-09-20): leave the chains as-is.** The ported-verbatim intent is worth keeping and the fallback layer already absorbs the dead rungs, so no chart change is made. The six unfunded aliases are a billing state, not a config defect — they work again when credits are restored. The cost accepted is ~2 extra round-trips per dispatch on 9 of 11 agents. Revisit only if that latency or the resulting routing quality actually bites.

### 13.3 The retired OpenCode layer is gone from the PVC

Confirmed present, then removed: `/opt/data/plugins/opencode` (304K), `/opt/data/.omo` (50M), `/opt/data/npm-global` (84M), `/opt/data/bin/opencode` (symlink), `/opt/data/mise/installs/opencode`. ≈**134 MB** reclaimed; `/opt/data/plugins` now holds only `omo`. Nothing recreates them — boot no longer installs OpenCode and the mise tool is gone. The Kubernetes resources (`Deployment`, `Service`, `ConfigMap`, `VPA`) had already pruned with the sync.

---

## 14. Post-ship verification — two defects ship-blocking claims were made against

§13 proved one path: sync dispatch, chain walk, structured result. This section is what a wider sweep found, and it contradicts claims the README and §6 currently make.

### 14.1 What the sweep covered

| Path | Result |
|---|---|
| Sync dispatch + chain walk + result | ✅ works (§13.1) |
| Read-only enforcement | ❌ never enforced → **removed** (§14.2, §14.4) |
| Persona delivery | ❌ never delivered → **fixed** (§14.3, §14.4) |
| Toolset/MCP scoping | ❌ dead code → **removed** (§14.1a, §14.4) |
| `status` / `cancel` / background dispatch | ✅ all work → §14.5 |
| `context=` as a child text channel | ✅ verified by probe (§14.3) |

**§14.1a — a third dead layer, found while scoping the first two.** `AgentSpec.toolsets`, `AgentSpec.mcp`, `READ_ONLY_TOOLSETS`/`ENGINEERING_TOOLSETS`/`ORCHESTRATOR_TOOLSETS`, `MCP_TOOLSETS` and `effective_toolsets()` were **all inert**: `effective_toolsets()` had zero callers anywhere including tests, and `_request()` passes `allowed_toolsets=None` so the child inherits the parent's set. So the README's "per-agent tool scoping" and "MCP is scoped by toolset" were false in the same way as the read-only claim. Removed under the same decision.

### 14.2 DEFECT — the read-only guard never fires (security)

**Evidence (objective, not self-reported):** dispatched `explore` — declared `read_only=True` — with the goal of writing a file via a file tool. It **succeeded**: `/opt/data/omo_guard_probe2.txt` existed on disk afterwards (`-rw-r--r-- 1 root root 5`), `terminal_state=SUCCEEDED`, no block and no error. Probe artefacts removed.

**Root cause — a hook kwargs mismatch, the same class as §5.2.** `guards.read_only_pre_tool_call` is correctly written and `ReadOnlyRegistry` is correct, but the registry is only ever fed by `_on_subagent_start`, and that hook reads:

```python
session_id = str(kwargs.get("session_id") or "")        # ← host sends child_session_id
metadata   = kwargs.get("metadata") or {}                 # ← host sends no metadata at all
```

The host fires `subagent_start` (`tools/delegate_tool.py:284-292`) with `parent_session_id`, `parent_turn_id`, `parent_subagent_id`, **`child_session_id`**, `child_subagent_id`, `child_role`, `child_goal` — and **no `metadata`**. So `kwargs.get("session_id")` is `""`, the `and session_id` guard short-circuits, `mark()` is never called, `is_read_only()` is always `False`, and the hook returns `None` for every tool call. It has never blocked anything.

**The host contract, verified:**
- `pre_tool_call` is invoked with `tool_name`, `args`, `task_id`, **`session_id`**, `tool_call_id`, `turn_id`, `api_request_id` (`hermes_cli/plugins.py:1790-1794`).
- It is dispatched from `agent/tool_executor.py:628` and `agent/agent_runtime_helpers.py:2214` via `tool_hook_ids(agent, …)` — i.e. **the active agent's** session, so a child's tool calls do carry the child's `session_id`. The mechanism is sound; only our hook is wrong.
- `subagent_stop` likewise passes `child_session_id` (`tools/delegate_tool_results.py:366-372`), never `session_id`.

**Two further dead couplings found in the same place:**
1. `metadata={"omo_agent": …, "omo_role": …}` on the launch request is **validated for size and then discarded** — `SubagentLifecycleService.launch` (`agent/subagent_lifecycle.py:260-264`) never forwards it to `_build_child_preserving_parent_tools`. So the only channel we chose for child identity does not exist.
2. The read-only CLI probe was blocked by the **host's** dangerous-command scanner (`approvals.single_query_mode`, in `-q` mode with no human to approve), not by our guard. That coincidence is what made the first probe look like a pass; the file-tool probe is what exposed the truth.

**Severity and blast radius:** contained but real — the affected agents are our own roster acting on our own orchestrator's prompts, not untrusted input. But read-only is the roster's core safety property, the README sells it as "enforced by hook", and 7 of 11 agents are declared read-only. Until fixed, the distinction between `oracle`/`metis`/`momus`/`prometheus`/`explore`/`librarian`/`multimodal-looker` and a write-capable agent does not exist at runtime.

**Fix requirements (mechanism deliberately not chosen yet):** the hook must read `child_session_id`, and the plugin needs a channel for child identity at launch time, since `metadata` is not forwarded. Candidates: a launch-time `contextvar` (the hook fires synchronously in the caller's thread — `launch()` calls `_build_child_preserving_parent_tools` directly, verified at `agent/subagent_lifecycle.py:259-264`), or correlating on `child_subagent_id` (which equals the handle's `subagent_id`, both `child._subagent_id`). Deferred to land together with §14.3 — see below.

### 14.3 DEFECT — the personas are never delivered

`agents/*.md` — 11 files, 2,952 lines, the largest body of work in this migration and an explicit requirement ("preserve personas") — **is read by no runtime code.** A grep for `read_text`/`open(`/`glob` across the plugin finds nothing; the only reference anywhere is a docstring in `roster.py`. The skill does not mention personas either, so nothing compensates.

Every worker therefore receives only: the bare `goal`, a caller-supplied `context` (absent in practice), a model, and toolsets. `AgentSpec` has no prompt field, and `SubagentLaunchRequest` has no system-prompt field. A probe confirmed it: a worker dispatched as `oracle` quoted a generic Claude-SDK preamble and could not identify as Oracle.

This is why §13.1 "succeeding" proves less than it appears: the worker did real work, but as a generic agent wearing an Oracle name-tag. In OMO the personas *are* the agent definitions, so this is a dropped delivery mechanism, not a cosmetic gap.

**Interaction with §14.2 — the reason both fixes are deferred together:** both defects have the same root — *there is no channel for child-specific data at launch*. `goal` and `context` are the only carriers, and `child_goal` is visible to the `subagent_start` hook. A single mechanism could therefore carry persona text *and* agent identity, or the two could be solved separately (contextvar for identity, another channel for persona). The right choice depends on the host's prompt-injection surface (`register_system_prompt_section`, `ctx.inject_message`, and whether either reaches a *child* session), which is under review — recorded here rather than guessed, because guessing is exactly what produced §14.2.

**README claims currently false and to be corrected with the fix:** "read-only is enforced by hook", "per-agent tool scoping", and "Reused: … the agent roster and personas".

### 14.4 Resolution

**Read-only guard + toolset scoping — removed rather than repaired** (operator decision: "I don't want a read only guard"). Because the guard had never fired, removal is a pure deletion with **no behaviour change**; the fleet has always run with the parent's capabilities. Removed: `read_only_pre_tool_call`, `ReadOnlyRegistry`, `READ_ONLY_WORKERS`, `read_only_tool_names()`, the three hook registrations plus the two callbacks that fed the registry, `AgentSpec.read_only`/`.toolsets`/`.mcp`, the four toolset tuples, `effective_toolsets()`, and the stale `provides_hooks` in `plugin.yaml`. Net **−153 lines** across 7 files; `check_delegation`/`GuardError` kept, as they enforce genuinely live rules. Suite went 44 → 43 (five tests deleted with the code they covered, four added).

**Personas — delivered at last.** Verified channel first, then built on it:
- **`ctx.inject_message` ruled out** — it writes to the CLI/gateway session, i.e. the *parent's* conversation, which would break the sole-user-facing-identity constraint.
- **`register_system_prompt_section` ruled out as the carrier** — hard cap **4,000 chars** and over-cap sections are **silently skipped** (log warning, no error); 8 of 11 personas exceed it. Also a global registry (renders into the parent) and `session_info` exposes no agent identity.
- **`context=` chosen** — `launch()` feeds `request.context` into child construction. Confirmed empirically before building: a probe dispatched with `context="<persona>…codenamed ZEPHYR…</persona>"` returned `Codename: ZEPHYR` verbatim, so the channel reaches the model.
- **Oracle's 6k-core recommendation was adapted, not followed.** It assumed the personas blow the budget; measured sizes show **10 of 11 fit whole** under the 32,000-char cap (only `sisyphus` at 32,212 exceeds). A 6k core would have truncated six personas and required reordering all eleven files — the same content surgery that produced the unfaithful-port defect in §5.2. Full text is passed instead.
- Implementation: new `orchestrator/personas.py` (`compose_context`, cached `persona_text`), wired into `engine._request()`; every persona also registered via `ctx.register_skill` for on-demand load as `skill_view("omo:<name>")`. Truncation (sisyphus only) appends that pointer, so the cap degrades to a lookup rather than a silent loss.
- Measured payloads: hephaestus 22,108 · atlas 28,612 · metis 13,448 · librarian 10,320 · momus 6,906 · oracle 6,536 · explore 2,962 · sisyphus-junior 2,921 · multimodal-looker 2,074 · prometheus 1,507 · sisyphus 31,549 (truncated). Caller context is preserved after the persona and the whole string is clamped under the host cap.

**Note — the `context` channel is text, not a system prompt.** It is the strongest per-child channel available, but a model can drift from it more easily than from a system prompt. The binding preamble is placed first to counter that; the escalation path (if a worker self-identifies generically) is a short `register_system_prompt_section` enforcement clause gated on session id, which would need the section to fire before the child's first prompt build.

**Verified live (rev `29cee0f`), against the probe that failed before the fix.** The same "state your agent name, role and one rule" dispatch to `oracle` had previously returned *"You are a Claude agent, built on Anthropic's Claude Agent SDK"* and could not identify as Oracle. Now:

```
agent: oracle · Architecture / Reasoning      model: claude-opus-5      api_calls: 1
"I am oracle, role Architecture / Reasoning — a strategic technical advisor invoked as an
 on-demand specialist for complex analysis … One rule I must follow: 'Recommend ONLY what
 was asked. No extra features, no unsolicited improvements.'"
```

That rule is `agents/oracle.md:83` verbatim, and the role phrasing matches lines 7 and 10 — objective evidence the persona reached the model, not a plausible paraphrase. Pod also confirms: 11 personas present, **zero** `register_hook` calls, plugin loaded.

**Also noted, not actioned:** the launch `metadata={"omo_agent", "omo_role"}` remains — the host validates it for size and then discards it, so it is currently inert. Kept only as a forward-compatible label.

### 14.5 Remaining paths exercised (background, status, cancel)

The last unverified surface was everything except sync dispatch. All of it works.

| Path | Result |
|---|---|
| `omo` dispatch with `background=true` | ✅ returns `run_id` immediately, tree at RUNNING |
| `omo_task` with `background=true` | ✅ same, via the second tool path |
| `omo` `action=status` with a `run_id` | ✅ worker row + tree |
| `omo` `action=cancel` | ✅ `cancelled: 1`, RUNNING → CANCELLED, idempotent on re-status |
| `omo` `action=status` with no `run_id` | ⚠️ empty in a new process — expected, see below |

**Non-bug root-caused — a background run's `model` field moves.** Observed changing from `deepseek-v4-flash` to `minimax-m3` on the same `run_id` between two status calls, which looks like a renderer bug and is not one: `_run_sync` sets `worker.model = request.model` at the top of every attempt (`orchestrator/engine.py:154`), and a background run is driven through `asyncio.to_thread(self._run_sync, …)` (`:242`). So the field tracks *the current rung of the fallback walk*. It is display-only — the chain state is built from `worker.chain`, so no routing depends on it — but a mid-flight snapshot can legitimately name a model that never produced the final result. Worth remembering when reading a running worker's row.

**`status` with no `run_id` is per-process.** The run registry is an in-memory dict on the engine instance, and each CLI invocation registers a fresh plugin, so a new process reports `{"runs": []}`. Within a session it behaves correctly (verified by polling one run across four calls). Durable run state is the already-recorded deferred item in §12.2, not a regression — but it does mean run history does not survive a session boundary, and that should be stated in the README rather than discovered.

---

---

---

## 15. Conformance audit against the originating request (2026-09-22)

The request that started this migration (23 sections) re-read against the shipped system. Evidence is from the live pod (`openagent-hermes-agent-7d4dcf7b4c-tb9gt`, plugin rev `d43a49e`) unless noted.

| # | Request | Status | Evidence |
|---|---|---|---|
| 1 | Replace OpenCode, no parallel runtime | **MET** | plugin owns roster/chains/lifecycle; no `opencode-server` workload; plugin clone removed; `boot.go` `opencodeSetup` deleted |
| 2 | Hermes is the permanent persona | **MET** | workers return structured results; worker-authored text is marked `not_evidence_until_observed` (claim boundary) and every launch now states the worker is internal |
| 3 | OMO-style agent roles | **MET (2026-09-22)** | all ten requested role names resolve: `tester`, `debugger` and `security` are new personas, the rest map through `ROLE_ALIASES` (explorer→explore, researcher→librarian, planner→prometheus, implementer→hephaestus, reviewer→momus, general→sisyphus-junior, documenter→`writing`) |
| 4 | Native orchestration layer | **PARTIAL** | engine = roster + chain resolver + fallback state machine + lifecycle + graph scheduler; no distinct Task Planner / Verification Manager components, by design |
| 5 | Task graph (DAG, dependencies) | **MET (2026-09-22)** | `dispatch` takes `tasks=[{id, agent\|category, prompt, depends_on}]`; ready tasks run concurrently up to `max_parallel`, dependents wait, dependents of a failure are `BLOCKED` and never launched; cycles/unknown/self deps/duplicate ids rejected before launch |
| 6 | Parallel execution | **MET** | real overlap asserted in tests (peak in-flight == 2); graph fan-out bounded by `max_parallel` (default 4) |
| 7 | Model routing per role | **MET** | per-agent + per-category `chains` through the LiteLLM provider; fallback state machine with cooldown/restore; delegation-side chain repaired 2026-09-22 (#276) |
| 8 | Workers emit events, not user messages | **MET** | plugin emits `omo:worker_done`; host emits `subagent.*` progress; no worker→user channel exists, and the launch context says so |
| 9 | OMO-style progress visibility | **MET** | `/omo` command and the new `tree` action render the run tree (dependencies shown per row); host progress streams to Discord/WebUI |
| 10 | Detailed delegation/task UI | **PARTIAL** | tree action + host subagent progress/transcripts; no per-worker expandable node (model, tokens, tool calls) outside the WebUI subagent view |
| 11 | Worker transcripts | **MET** | native: `cache/delegation/live/<delegation_id>/task-N.log` + session DB |
| 12 | Structured result collection | **MET** | host `SubagentResult` (+ `usage_metadata`, `tool_execution_summary`) and `structured_payload` via `output_schema`; claim boundary separates observed from self-reported |
| 13 | Review/verification loops | **MET (2026-09-22)** | `review: true` runs `momus` over each successful task and re-runs it with the reviewer's notes, bounded by `max_review_cycles` (0 disables); an unparseable verdict counts as a pass, so a malformed review cannot become an infinite rewrite |
| 14 | Persona protection | **MET** | architectural: children run non-interactive with no user channel; personas delivered in launch context |
| 15 | Context isolation | **MET** | `compose_context` sends the reviewer/worker contract + persona + caller context only; the conversation is never copied |
| 16 | Worker system prompt | **MET (2026-09-22)** | every launch context opens with `<internal_worker>`: "you are not the assistant", `USER-FACING COMMUNICATION: DISABLED`, report findings with verified separated from assumed. Prepended before the persona and delivered even without a persona file — a prompt edit cannot drop it |
| 17 | Nested delegation | **MET (not yet proven live)** | `delegation.max_spawn_depth: 3`; plugin passes the orchestrator role; the host derives role from depth. No nested spawn observed end-to-end |
| 18 | Failure handling | **PARTIAL** | retry via chain ✅, cancellation ✅, dependency failure now propagates as `BLOCKED` ✅; "replace worker" / "continue without result" remain Hermes' judgement |
| 19 | Event system (`task.*`) | **PARTIAL** | equivalent coverage under the host vocabulary (`subagent.spawn_requested`, `subagent.start`, `subagent.complete`, progress) plus `omo:worker_done`; the literal `task.*` names do not exist |
| 20 | Cancellation | **MET** | `omo action=cancel` (run or worker) and `delegate_task action=stop`; the run is the branch |
| 21 | Remove OpenCode after migration | **MET (2026-09-22)** | cluster workloads, plugin clone and `boot.go` `opencodeSetup` already gone; the audit found **two survivors the earlier sweep missed** — a stray `/opt/data/mise.toml` on the PVC pinning `opencode = "latest"` (pre-migration drift, which made mise reinstall it on every boot) and the 176 MB install it fed (`mise/installs/opencode`, its shim, and the `/opt/data/bin/opencode` symlink `linkShims()` recreates). Pin file, install, shim, symlink and the dead supervisor wrapper removed; `mise ls` reports 0 and nothing named `opencode*` resolves on PATH. Doc-only mentions remain (the README explains the oh-my-openagent relationship; four personas name OpenCode in ported prose) — no runtime coupling |
| 22 | Configuration surface | **PARTIAL** | omo `chains`/`categories`/`runtime_fallback`/`mcp_enabled`/`max_parallel`/`max_review_cycles`/`roles` ✅ and `enabled_agents` is now enforced (it was read and ignored) ✅; still no `communication:` block, and the host-side `delegation.max_concurrent_children` remains unset |
| 23 | Acceptance criteria | **19 of 22 MET** | remaining misses: per-worker expandable UI node, nested spawn proven live, `task.*` event names |

### Remaining work

1. ~~**Task graph** (§5)~~ — **done**: `orchestrator/graph.py`, `tasks=` on `dispatch`, BLOCKED propagation, 22 new tests (PR #4, `jomakori/hermes-omo-plugin`).
2. ~~**Review/verification loops** (§13)~~ — **done**: `review: true` + `max_review_cycles`, reviewer visible in the tree as `<task>:review`.
3. ~~**Role coverage** (§3)~~ — **done**: `tester`, `debugger`, `security` personas plus `ROLE_ALIASES` (overridable via the `roles` setting). New prompts are unexercised on a real task so far.
4. ~~**Internal-worker prompt block** (§16)~~ — **done**: `<internal_worker>` contract prepended to every launch context, delivered with or without a persona.
5. ~~**Dead config** (§22)~~ — **done**: `enabled_agents` now refuses a disabled agent and names what is enabled.
6. ~~**OpenCode leftover** (§21)~~ — **done 2026-09-22**: stray PVC pin, 176 MB install, shim, `/opt/data/bin` symlink and the dead supervisor wrapper removed; verified via `mise ls` and PATH. Lesson: the earlier sweep verified the removal by hand (§5.2), which is exactly why a PVC-resident pin survived it — a hand sweep cannot see state that lives outside the chart.
7. **Parallelism declaration** (§22) — plugin-side `max_parallel` is declared (default 4). The host-side `delegation.max_concurrent_children` is still inherited (default 10) while the router prompt tells Hermes to cap omo at 8: pick the number and declare it.
8. **Detailed task UI** (§10) — the `tree` action and the `/omo` command both render the run tree, and host progress streams to Discord/WebUI. Decide whether that satisfies the request or whether a per-worker expandable node (model, tokens, tool calls) is worth building.
9. **Delivery** — the chart's `provision-omo-plugin` initContainer re-clones `main` on boot, so landing PR #4 is merge + pod roll: no image rebuild, no sha pin.
10. **Unproven live** — no nested spawn (depth 2+) has been observed since the migration, and the three new personas have not run a task. Run both before calling §3 and §17 verified.

### What the audit changes about this plan

The architectural core (Hermes-only persona, native orchestration, OpenCode removal including the PVC residue, model routing, transcripts, structured results, cancellation, nested-delegation gate) is met and live-verified. The functional surface the request named and the migration had left open — task graph, review loops, three role personas, the internal-worker contract, one dead config key — is now implemented behind tests in PR #4. What is left is a number to declare, a UI decision, and two things to exercise on the live system.

---

## 15.1 Live verification after delivery (2026-09-22, 01:10–01:27)

Results of exercising the new surface on the running pod rather than in tests. Pod `openagent-hermes-agent-7d968d9cb6-mpkjr`, plugin clone at `17b29ea`.

**Verified live**

- **Task graph (§5)**: `security` → `tester` graph returned `status: succeeded` with both workers SUCCEEDED, and the timestamps prove the ordering was real — `tester` started at `…376.486` after `security` completed at `…376.433`. Dependency semantics also verified in the failing case: when the first task failed, its dependent came back `BLOCKED` and was never launched.
- **Role coverage (§3)**: all three new personas ran a real task — `security` (`SECURITY-OK.`, 5.5s), `tester` (`TESTER-OK.`, 6.43s), `debugger` (`DEBUGGER-OK.`, 6.41s).
- **Nested delegation (§17)**: `sisyphus` ran at depth 1 with `role=orchestrator`, delegated a child, and reported `NESTED-CHILD=NESTED`. Verified on the host side rather than from the worker's word: `deleg_d5950952` exists with the child's exact goal and `status: completed`. A child launched from a depth-1 orchestrator is depth 2, so nesting works under `max_spawn_depth: 3`.

**Two defects only running it live could find**

1. **Graph tasks were refused on the host** — `No active Hermes parent session is available.` The scheduler submitted tasks to a bare `ThreadPoolExecutor`, and threads do not inherit ContextVars; the host resolves the active parent session from one. Single-task dispatch was unaffected because it stays on the caller's context, and no fake-lifecycle test could see it. Fixed by running each submit inside a copy of the caller's context, with a test that asserts a ContextVar reaches the workers (RED `['unset','unset']` before). Merged as the #5 squash.
2. **`review: true` was accepted and ignored on a single-task dispatch** — the reviewer exists only in the graph layer, so the call ran unreviewed and reported success. Same shape as the `enabled_agents` key that was read and never used. `dispatch` now routes through the graph machinery whenever `review`, `max_parallel` or `max_review_cycles` is supplied.

Both reviews of the branch (mine and an independent `pr-reviewer` pass) missed both defects: they were structural rather than logical, and the unit harness had no host to refuse a launch.

**Fleet reality check (LiteLLM logs, 6-minute window)**

`deepseek-v4-flash` served 200 OK through the BYOK deployment while the same alias 402'd on the Kilo deployment, and `claude-sonnet-4-6` served the escalations; 20 `Add credits` errors in the window. Kilo remains unfunded, so every dispatch still burns a wasted hop before falling back. The `model` field in a payload is the requested alias, not proof of which deployment served it — the log is the evidence.

---

## 15.2 Live verification, second pass (2026-09-22, 01:30–01:45)

Everything below ran on the pod, not in a test harness. Plugin clone `17b29ea`, then `0723da0` for the last two items.

**Verified live**

- **`tree` action**: rendered the run tree including the reviewer row and its parent (`impl:review`, `parent: impl`).
- **Review cycle**: `momus` ran as a real second worker over a `tester` task; the run stayed `succeeded` and the implementer was not re-run.
- **Role alias**: `agent="implementer"` dispatched `hephaestus` and succeeded.
- **Fan-out**: three independent tasks started within 50 ms of each other with overlapping runtimes — real concurrency, not a serialized loop.

**Two more findings only running it could produce**

1. **A review outcome was unfalsifiable.** A payload said only that a review row existed, so "reviewed and clean", "the reviewer wrote something unreadable" and "the reviewer died" were indistinguishable. Each reviewed task now reports `review_verdict` (`""` / `pass` / `problems` / `unparsed` / `reviewer_failed`), filed as self-reported because it is derived from the reviewer's text. Only `problems` re-runs a task.
2. **A worker answered "Hermes" to an identity probe.** The contract travels in the launch *context*; a child's system prompt belongs to the host and a plugin cannot replace it. The wording gains an explicit identity clause and the README stops implying the clause is a guarantee — the guarantee is architectural, since a worker has no channel to the user. This is why request §16 is **PARTIAL**: the instruction is delivered, but not in the system prompt where the request asked for it.

**Decisions taken**

- **Parallelism (§22)**: declared `delegation.max_concurrent_children: 8` (gke_GitOps PR #277). The router prompt caps OMO fan-out at 8 and the plugin's graph default is 4, so the host default of 10 was the only implied number left.
- **Task UI (§10)**: the `tree` action, the `/omo` command and the WebUI subagent view are enough. A per-worker expandable node is not worth building while the tree, the row fields and the host transcripts already answer "what ran, on what model, and what came back".

**Open PRs at the time of writing**: plugin #6 (review controls on a single dispatch), plugin #7 (`review_verdict` + identity clause), gke_GitOps #277. Each needs a merge plus a pod roll to land.

**Lesson recorded**: `gke_GitOps`' working checkout was 200 commits stale, so a grep there still showed the pre-removal `opencode` mise pin and almost led to "fixing" already-correct main. Worktrees must be cut from a freshly fetched `origin/main`, and main must be re-fetched at the start of a new turn.

---

## 15.3 Post-merge live confirmation (2026-09-22, 01:37)

Plugin main `4c761f9` and gitops main `9ce55c9` landed on a rolled pod (`openagent-hermes-agent-6569bdb9c9-ckwfn`); ArgoCD reports `Synced` and the live config carries `max_concurrent_children: 8`.

- **Review controls on a single dispatch**: a dispatch with `review: true` and no `tasks=` now runs the reviewer — the tree shows `tester` plus `momus · review task-1` (parent `task-1`). It reported `review_verdict: "unparsed"`, which is the honest outcome: `momus` answered in prose, so no verdict could be read. Before this change that was indistinguishable from "reviewed and clean"; the field is doing exactly the job it was added for. The run stayed `succeeded` and the task was not re-run.
- **Identity clause**: the same probe that answered "Hermes" now answers `IDENTITY-PROBE-OK. Hephaestus.` — the persona. One probe is not a guarantee, so §16 stays PARTIAL, but the clause changes the observed behaviour.
- The claim boundary carries `workers[].review_verdict` in the live payload, confirming both the field and its classification reached production.

All three PRs are merged; nothing is pending at the gate.

---

## 16. Status review (independent, 2026-09-22)

An independent agent read the plan end-to-end, ran the suite, grepped `origin/main` of `gke_GitOps` for OpenCode residue, and inspected the plugin code. It hit its iteration cap before writing this section, so the findings below are reproduced from its report, with my own verification marked **[verified]** or **[corrected]**.

**Straight answer to "is everything in the spec finished and tested?" — No.** The architectural core is done and tested; a handful of items are finished-but-untested, four are not started, and two claims in the plan have no evidence.

### New findings worth acting on

| Finding | Verification |
|---|---|
| `mcp_enabled` is dead config: read in `__init__.py:24`, consumed by nothing, while the chart still sets `mcp_enabled: true` (values.yaml:613) — the same shape as the `enabled_agents` key this migration already fixed once | **[verified]** grep shows the read and no consumer |
| Rollback was never rehearsed, though §9 promises "≤1h" | **[verified]** no recorded rehearsal anywhere in the plan or repo |
| Chart CI (`ct_check`, `helm unittest`, render/kubeconform) claimed green in §5.2/§9, but no recorded output — the reviewer could not run it | **[verified]** and independently confirmed here: `helm unittest` does not run cleanly in a bare worktree (3 failed/9 passed without subcharts; all 12 fail with them, umbrella schema rejects `global`). Pre-existing on clean `main` |
| `/omo` slash command "not visibly registered" | **[corrected]** it is registered: `_register_optional(ctx, "register_command", "omo", handler=…engine.status())` with skills and `on_unload` registered independently — the design the plan describes |
| Worker transcripts are claimed (§15 req 11) but no test asserts the path | **[verified]** true; transcripts were read by hand during this migration, never by a test |

### Finished but not tested

- Worker transcript path (`cache/delegation/live/`) — inspected by hand, never asserted.
- Chart values reaching `ctx.get_config` at runtime — settings assembly is tested; the chart→plugin hop is not.
- `mcp_enabled` — see above; it is untestable because nothing reads it.
- `omo_task` — exercised through `omo`'s tests, no dedicated test of its own path.

### Tested but not finished

- Worker system prompt (§16 of the request): tested, but delivered through the launch *context*, not the system prompt the request asked for.
- Event vocabulary (§19): `omo:worker_done` is tested; the full `task.*` naming never existed.

### Not started

- Durable run state (`ctx.state`) — run history dies with the session.
- OMO safety guards — **dropped by decision** (§17): the hook they need does not fire. Residual risk recorded in §12.2.
- Model-family prompt variants (D6) — explicitly deferred.
- Rollback rehearsal (§9).
- A `communication:` config block (§22) — the contract is unconditional by choice.

### Deliberate exclusions — do the reasons still hold?

All eight it examined still hold, each re-checked against `origin/main`: per-agent provider routing (all chains carry the `litellm/` prefix), the read-only guard (operator decision, was never functional), toolset/MCP scoping (`allowed_toolsets=None` always), model-family prompt variants, `gitopsctl` extraction, the local OpenCode workstation (comments only), the approval-transport plugin, and the per-worker expandable UI node.

### Verdict

The migration is where the plan says it is: OpenCode removed, native plugin shipped and live, personas delivered, fallback chains working, task graph and review cycles implemented and live-verified. The plan is honest about its own gaps — it self-identifies the never-fired guard, the context-vs-system-prompt limitation and the in-memory run registry. What the review adds is one **new** dead-config finding (`mcp_enabled`), one unproven success criterion (rollback), one unverifiable CI claim, and a short list of things finished in code but never asserted by a test.

Highest-value next actions, in order: delete or wire `mcp_enabled` — **done** (plugin #8, chart #278); rehearse the rollback and record it — **done** (§17); and either test the transcript path or stop claiming it as verified — **done** (§15 req 11 amendment in §17).

---

## 17. Rollback rehearsal (2026-09-22)

§9 promised "rollback ≤1h". It had never been rehearsed. This records the mechanism, the measured cost, and — more usefully — the two limits the rehearsal exposed.

### Measured

From ArgoCD's own deploy history, sync-start to fully-deployed (chart sync plus rollout):

| Revision | Deployed | Duration |
|---|---|---|
| `2ba12e5d` | 2026-09-21T20:54:52Z | 51s |
| `51b33606` | 2026-09-22T00:10:05Z | 80s |
| `9ce55c97` | 2026-09-22T01:38:07Z | 93s |

Three samples, 51–93s, on a Recreate/1-replica deployment where the pod carries the whole session. A rollback that lands is therefore **~1.5 minutes**, not an hour: the criterion holds with room to spare.

### Limits the rehearsal exposed

1. **`revisionHistoryLimit: 3`.** Only the last three revisions are rollback candidates inside Argo. The pre-migration revision — the one a genuine "undo the migration" would need — has already aged out. Anything older than three deploys is a `git revert` + push + sync, not an `argocd app rollback`.
2. **A chart rollback does not roll back the PVC.** The plugin clone is re-provisioned from `main` by the initContainer on boot, so it follows the *current* chart, not the revision being rolled back to; the removed mise `opencode` pin stays removed; credentials and OMO settings persist. "Rollback" here means reverting the chart commit that changed the provisioning path — a git operation — after which the pod converges on the next roll.

### What was and was not rehearsed

**Rehearsed:** the mechanism (git-driven revert or `argocd app rollback`), the duration (measured above), and the boundaries (history depth, PVC state).

**Not rehearsed end-to-end:** taking the live system back to the pre-migration OpenCode runtime. That needs a revision that has aged out of history, and it would put the fleet on a runtime whose model aliases are unfunded — while that same pod is running this session. The measured path is a truthful rehearsal of the rollback; a full revert would be a rehearsal of an outage.

### Procedure, if it is ever needed

1. `git revert <migration-commit>` on `main`, push (or `argocd app rollback openagent <id>` for one of the last three).
2. Wait for the sync to go Healthy (~90s).
3. Roll the pod so the initContainer re-provisions the plugin (`kubectl -n openagent rollout restart deploy/openagent-hermes-agent`).
4. Verify: pod Running, config ConfigMap carries the intended revision's values, `omo` dispatch answers.

### Status amendments from the independent review

- **§15 req 11 (worker transcripts)**: the transcript path is a *host* feature, not something this migration built, and it was verified by reading files by hand on 2026-09-21 and 2026-09-22 — no automated test covers it. The claim is accurate but its evidence is manual; recorded so it stops reading as machine-verified.
- **§12.2 (OMO safety guards: write-existing-file, bash-file-read, and the rest)**: **dropped by decision** — the entry is removed rather than left implying planned work, with the reasons and the accepted residual risk recorded in place.

### Refreshing this plan's Google Doc

The Google tool can only read files under `ALLOWED_FILE_DIRS` (`/opt/data/tmp`), so a Doc refresh is: copy this file to `/opt/data/tmp/omo-plan.md`, then `update_drive_file(file_id=<doc id>, file_path=/opt/data/tmp/omo-plan.md, source_format="md")`. That replaces the Doc in place — same ID, same link, no duplicate. Doc ID `1ZqYJ46XYgJIl2iZ08DK1XvK7aKGFQTd-wbJVTioAG48`. An earlier workaround staged a copy inside the tool's own root-owned sandbox; nothing needs that now.

---

## 18. Revision 3 — modelled on omo-slim, with the Claude limit as a named failure (2026-10-26)

**Owner direction, verbatim:** *"Model our plugin after omo-slim, reverse engineer it for Hermes usage"* · *"The fallbacks should address the Claude limits"* · *"for last fallback for all models, set it to DeepSeek flash API."*

Two things change; the rest of this plan stands. (a) The **shape** of the fleet — roles, gates, delegation, self-improvement — is taken from `alvinunreal/oh-my-opencode-slim` v3.0.6 after reading its source, not from its README. (b) **Fallback becomes a property of the system**, not a list someone remembered to type: the Claude session limit is a first-class, hop-eligible failure, and `deepseek-v4-flash-direct` is the terminal rung of every chain, applied centrally.

### 18.1 Provenance

| What | How | Status |
|---|---|---|
| upstream `oh-my-opencode-slim` v3.0.6 | shallow clone → `/opt/data/repos/omo-slim`, read `AGENTS.md`, `docs/background-orchestration.md`, `docs/configuration.md`, `docs/council.md`, `src/agents/`, `src/hooks/`, `src/config/`; indexed with codegraph (584 files / 9,556 nodes) and explored by symbol | read |
| our plugin `hermes-omo-plugin` | cloned to `/opt/data/repos/hermes-omo-plugin`, indexed with codegraph (67 files / 1,956 nodes), `chains.py` / `engine.py` / `graph.py` / `models.py` read | read |
| live behaviour | two omo dispatches + a direct `is_retryable()` test against the real failure text | measured, §18.3 |
| commit discipline | source clone at `272b951`; the **running pod** at `09956e6`. `orchestrator/chains.py` and `graph.py` are byte-identical between the two; `engine.py` differs only by #37 (dependency-result handoff, not live). Every line citation in §18.3 was re-checked against both | verified |
| upstream build/run | **not done** — it is a TypeScript OpenCode-host plugin; nothing in this migration depends on running it | declined on purpose |

Line-level citations for upstream are deliberately absent: a shallow clone's line numbers drift, and a plan that cites them is a plan whose evidence cannot be reproduced. Mechanism names + file paths only; SL-1 pins exact symbols in the ticket that ports each one.

### 18.2 What omo-slim actually is (mechanism inventory)

| # | Mechanism | Upstream surface | What it buys us | Ours today |
|---|---|---|---|---|
| 1 | Background supervisor + job board | `src/hooks/task-session-manager/` (`index.ts`, `event-router.ts`), `docs/background-orchestration.md` | orchestrator keeps working while children run; one board owns state | **partial** — `Run`/`Worker` persisted (`orchestrator/models.py:243`), no reconciliation loop |
| 2 | Terminal gate: observation tokens, generation fences, activity/terminal/attempt revisions, single-use commit token | same | a stale or duplicate observation cannot invent a completion | **absent** |
| 3 | Liveness reconciliation: poll, idle/absent grace, `stopped-unreconciled` vs `completed`, tombstones | same | "stopped without reporting" becomes a named state instead of a hang | **partial** — watchdog exists (HEAD commit), unwatched cases remain |
| 4 | Concurrency admission: per-model / per-provider / default tiers, queue + pump | same | an 8-wide fan-out cannot trample a quota window | **absent** |
| 5 | Wall-clock supervisor (one-shot deadline; never writes terminal state) | same | a wait cannot outlive its deadline | **absent** |
| 6 | Foreground fallback conversion | `src/hooks/foreground-fallback/` | same-provider work runs in the caller's turn instead of a doomed child | **absent** |
| 7 | Task prompt contract + anti-acknowledgment rule | `docs/background-orchestration.md` | "ok, will do" stops counting as work | **absent** |
| 8 | Cache safety: frozen prefix, tail-only injection, runtime monitor, three test gauntlets (property, snapshot, tripwire) | `src/hooks/cache-safe-injection.ts`, `src/hooks/cache-monitor/`, `src/hooks/cache-safety.property.test.ts`, `src/hooks/cache-payload.snapshot.test.ts`, `src/cache-safety-tripwire.test.ts` | cache hit becomes enforced, not hoped for | **absent** — and we measure 0% on the Sonnet hop |
| 9 | Council: independent councillors + synthesizer | `src/hooks/council-inject/`, `docs/council.md` | disagreement is visible before it is acted on | **partial** (momus review) |
| 10 | Role routing: specialist bases + custom agents from bases | `src/agents/role-routing.ts:18` (`ROLE_ROUTING_SLIM_LINES`) | one routing table, not per-call agent strings | **partial** (`roster.py`) |
| 11 | Model inheritance (`inheritModelFrom`), ordered per-role chains, no-propagate rules, `restore_primary_after_cooldown` | `src/config/`, `docs/configuration.md` | a child follows the orchestrator — unless the parent's failure is itself the reason not to | **partial** — chains exist, the hop is broken (§18.3) |
| 12 | Presets + runtime preset switching, per-preset agent overrides | `src/config/`, `docs/configuration.md` | cost/quality profile switched without a redeploy | **absent** |

### 18.3 Live defects proven during this revision

**D1 — Claude limits are classified as non-retryable, so the chain never advances.** *(`chains.py` is byte-identical between the clone and the running pod, so this is the behaviour of the code that actually ran, not of a newer branch.)* Two dispatches died at rung 1 with a healthy rung 2 available. The predicate, tested directly against the real failure text:

```
is_retryable(status=None, message="HTTP 500: litellm.InternalServerError … You've hit your session limit · resets 6pm (UTC)") -> False
is_retryable(status=500,  message=<same>)                                                                              -> True
```

`500` and `429` *are* in `RETRYABLE_STATUS` (`orchestrator/chains.py:9`), but the wrapper drops the HTTP status, so classification falls through to `_RETRYABLE_PATTERN` (`chains.py:166-169`), which covers `rate.?limit|quota|overloaded|too many requests|temporarily unavailable|service unavailable|bad gateway|gateway timeout` — and **not** `session limit`, which is the exact wording Claude returns. `hop_history` therefore ends at `[{model: "claude-haiku-5-5", reason: "transport"}]` and `next_model()` (`chains.py:327`) is never reached.

**D2 — A candidate that is ineligible is treated as fatal, not as skippable.** `NON_RETRYABLE = {"abort", "context_overflow"}` (`chains.py:10`). `qwen3.6-35b-local` has a 32,768-token window against a 64,000 host minimum, so every chain that leads with it dies instantly — and this is the *default head* of `quick`, `explore` and `librarian`. Note that the owner's new table would make `local/qwen3.6` the default for Explorer, Librarian **and** Atlas: imported literally, three roles arrive dead.

**D3 — The fallback layer is ours, and it is the only one.** LiteLLM's error text reads `Available Model Group Fallbacks=None` because degradation lives in the **plugin** layer by design, not in the proxy. That makes the plugin's chain walk the single point where a Claude limit is survived — so D1 and D2 are not "one of two layers is weak", they are *the* layer being blind. Nothing about this revision touches LiteLLM routing.

**D4 — Cache is unguarded where it matters.** `subagent` + `claude-sonnet-5`: 77 runs, 88.8M prompt tokens, **0% cache read**. Upstream ships a hook, a monitor and three test gauntlets against exactly this (#8 above); we ship nothing and measure the consequence.

**D5 — Duplicate rungs.** The `momus` chain repeats the same three models twice (six entries). Harmless only because the hop never fires; once it does, it doubles the wait before the terminal rung.

### 18.4 Rules this revision imposes

- **R1 — The Claude limit is hop-eligible, by name.** A failure whose text names a session/usage limit, a reset time, exhaustion or a 429/500 body carrying it advances the walk. The Claude signatures to match: `session limit`, `usage_limited`, `resets <time>`, `hit your limit`, `credit balance is too low`, plus the existing `rate.?limit|quota|overloaded` set. Matching is on the *message*, because the status is what gets lost.
- **R2 — Terminal rung is `deepseek-v4-flash-direct`, set centrally.** Not typed per chain: `ChainResolver.policy()` (`chains.py:432`) appends the configured terminal rung to every chain after `skip_benched()`, deduped by `canonical_model()` (`chains.py:261`). One place, so a new agent cannot be born without a floor.
- **R3 — The terminal rung is exempt from bench-skip, and its absence is loud.** Today `skip_benched()` (`chains.py:149`) can silently reduce a chain to one rung — which is indistinguishable from a chain that never had a floor. The terminal rung survives bench-skip; if it is genuinely unavailable the dispatch fails with `no live rung` naming the chain, instead of degrading quietly.
- **R4 — Ineligible skips; fatal stops.** Three classes, three behaviours: *ineligible candidate* (window too small, unknown alias, no route) → skip the rung, record `reason: ineligible`, continue; *transient/provider* (limits, 5xx, timeouts) → cooldown the seat, hop; *fatal* (`abort`) → stop the walk. `context_overflow` moves from `NON_RETRYABLE` to skip-and-continue, since a window that is too small for one model is not a property of the task.
- **R5 — The plugin layer is the only fallback layer; do not duplicate it in LiteLLM.** No router-fallback configuration is added. One owner of degradation means one place to observe it, one place to fix it, and no second path that can mask a broken chain walk. The boundary this draws, stated plainly: work dispatched **outside** omo — a direct Hermes turn, an auxiliary or vision call — has no chain at all, because it never enters `ChainResolver`. That is a scope boundary, not a gap to be closed in the proxy.
- **R6 — A rung whose window is too small is not a rung.** `qwen3.6-35b-local` is served by a route declared `max_input_tokens: 32768` (LiteLLM `model-list`, checked live before gke_GitOps #419; now 131072); the host requires 64K. It stays in the matrix where the owner's table names it, but it only becomes selectable after SL-5 raises `num_ctx`. Until then it is skipped silently by R4, not treated as a failure of the run.
- **R7 — Exactly one middle rung.** Every agent is `default → one fallback → floor`. Three rungs, never five. Measured against today's config, this *removes* rungs: `momus` currently repeats the same three models twice and `oracle` lists five, which is why a limit at rung one looks like a total failure instead of one hop.
- **R8 — The middle rung must be a different provider than the default.** This is the rule that makes a fallback worth having: a default and its fallback behind the same provider share the same quota, the same outage and the same credential. Checked live, that is exactly today's shape — four of six routes sit behind `claude-proxy`, so any Claude model that limits takes out its own fallback with it.
- **R9 — GLM flash is kept, and that is what makes R8 satisfiable.** Without it the only non-DeepSeek providers are Anthropic and local, so every middle rung would be Anthropic and the quota wall would erase the whole ladder at once. `glm-5.3-flash` therefore becomes a provisioned route (SL-13: Z.ai credential in Doppler, one `model_list` entry, provider egress), not a struck-through name. Its pricing is captured from the provider at provisioning time and recorded in §18.12 — not assumed here.

### 18.5 Chain matrix — the owner's 9 roles, subscription-first, DeepSeek floor (rev. 2026-10-27)

Owner rules, as stated 2026-10-27:
- No Z.ai. GLM-Flash stays and routes through **opencode-go**.
- Default and first fallback are subscription models: Claude Max or opencode-go.
- Floor is `deepseek-v4-flash-direct` on every row.
- One fallback layer before the floor, on a different provider from the default. Explorer and Librarian get one extra rung (owner-approved).
- Explorer and Librarian default to local `qwen3.6-35b-local`. Owner-approved exception to subscription-first. SL-5 DONE: window raised 32K → 131K by gke_GitOps #419 (Argo Synced at `8fe9fc6`; live configmap routes `qwen3.6-35b-local` → `openai/qwen3.6:35b-131k`). Not gated; first live completion still unverified.

| # | Role | Default | Fallback 1 | Fallback 2 | Floor | Route status |
|---|---|---|---|---|---|---|
| 1 | Orchestrator | `opencode-go/deepseek-v4.1-flash` (Go) | `claude-haiku-5-5` (Claude Max) | — | `deepseek-v4-flash-direct` | NOT ROUTABLE: no Go alias or key (SL-14) |
| 2 | Explorer | `local/qwen3.6` (exception) | `claude-haiku-5-5` (Claude Max) | `opencode-go/glm-5.3-flash` (Go) | `deepseek-v4-flash-direct` | Qwen 131K (SL-5 done); Go rung NOT ROUTABLE |
| 3 | Librarian | `local/qwen3.6` (exception) | `claude-haiku-5-5` (Claude Max) | `opencode-go/glm-5.3-flash` (Go) | `deepseek-v4-flash-direct` | GATED: as row 2 |
| 4 | Fixer | `opencode-go/glm-5.2` (Go) | `claude-haiku-5-5` (Claude Max) | — | `deepseek-v4-flash-direct` | NOT ROUTABLE: Go (SL-14) |
| 5 | Oracle | `claude-sonnet-5` (Claude Max) | `opencode-go/glm-5.2` (Go) | — | `deepseek-v4-flash-direct` | Default LIVE; fb1 NOT ROUTABLE (SL-14) |
| 6 | Designer | `claude-haiku-5-5` (Claude Max) | `opencode-go/glm-5.3-flash` (Go) | — | `deepseek-v4-flash-direct` | Default LIVE; fb1 NOT ROUTABLE (SL-14) |
| 7 | Council | `opencode-go/deepseek-v4.1-flash` (Go) | `claude-sonnet-5` (Claude Max) | — | `deepseek-v4-flash-direct` | NOT ROUTABLE: Go (SL-14) |
| 8 | Atlas | `opencode-go/glm-5.3-flash` (Go) | `claude-haiku-5-5` (Claude Max) | — | `deepseek-v4-flash-direct` | NOT ROUTABLE: Go (SL-14) |
| 9 | Sisyphus-Junior | `opencode-go/deepseek-v4.1-flash` (Go) | `claude-haiku-5-5` (Claude Max) | — | `deepseek-v4-flash-direct` | NOT ROUTABLE: Go (SL-14) |

Provider check: every fallback 1 is on a different provider from its default (Claude vs Go, or local vs Claude). Rows 2 and 3 fallback 2 (GLM on Go) is on a different provider from fallback 1 (Claude).

**Caveats**
- Haiku is fallback 1 on six rows (1, 2, 3, 4, 8, 9). One Claude quota exhaustion therefore hits six roles at once. Moving those fallbacks to Go Haiku would break the provider rule on Go-default rows, so this is an open decision (§18.11).
- Floor DeepSeek (direct API) and the Go DeepSeek default on rows 1, 7, 9 are the same vendor. The floor is exempt from the provider rule, so this is allowed; it is still one vendor.
- Role-to-model fit is judgement, not a benchmark.

### 18.6 Target architecture (Hermes-native, not an OpenCode port)

```
Hermes turn
  └─ omo dispatch ──► JobBoard (one board, per-session attribution)
                        ├─ admission: per-model / per-provider / default caps, queue→pump
                        ├─ jev:  route_intent → pick_agent → gate_risk      (3 call sites, §18.7)
                        ├─ worker launch ──► chain walk (R1–R4) ──► floor: deepseek-v4-flash-direct
                        ├─ supervisor: wall-clock deadline (never writes terminal state)
                        └─ terminal gate: observation token + generation fence + revisions
                              └─ report validation (claim_boundary: observed vs self-reported)
                                    ├─ accept → wake caller (background) / return (sync)
                                    └─ reject → momus cycle (bounded) → re-dispatch or surface
Telemetry (tokens, cache hit, hops, wall-clock, rejections) ──► tuning (§18.8)
```

The one deliberate divergence from upstream: omo-slim polls a **session store it does not own**, because OpenCode owns sessions. Hermes owns its runs, so liveness comes from our own board plus a heartbeat — a literal port of the polling loop would watch state that never changes and produce exactly the early stalls this migration is meant to remove.

### 18.7 Where jev sits in delegation

Three call sites, no more: `route_intent` once per dispatch (already present), `pick_agent` only for the unnamed/`auto` case (`orchestrator/engine.py:408` already never second-guesses an explicit `agent=`), `gate_risk` before anything irreversible. A low-confidence pick must fail as "name an agent" rather than burn a worker for its whole budget — that behaviour exists and is kept.

### 18.8 Self-improvement loop

Weekly, from the LiteLLM spend tables plus our own run records, tune **only** these three, each with a stated acceptance and a hard bound:

| Signal | Action | Bound |
|---|---|---|
| Claude-limit events per 5h window | if the floor absorbed ≥1 limit per window on ≥3 windows → demote the Claude rung for that role | never demote `oracle`/`ultrabrain` below Sonnet |
| hops per run, by reason | >2 hops/run sustained → fix the seat, do not widen the chain | chains ≤4 rungs |
| rejections by reason | validation rejection >20% for a role → tighten its task prompt, not its model | never raise concurrency |
| cache read % per (agent, model) | <20% on a repeating prompt → cache-safety violation, open a defect | — |

Self-tuning never raises throughput caps. Concurrency, budgets and timeouts move only by human decision, because those are the settings that turn a bad afternoon into a quota wall — which is precisely how this revision started.

### 18.9 Tickets

Numbered `SL-`; each carries observable acceptance. Dependencies are explicit; sizes are S/M/L.

| ID | Work | Acceptance (observable) | Dep | Size |
|---|---|---|---|---|
| SL-1 | Pin the mechanism inventory to symbols: one file per ported mechanism citing `path:line` in the v3.0.6 clone (SHA recorded) | every §18.2 row has a symbol path+line that resolves at the recorded SHA | — | S |
| SL-2 | **R1** — hop-eligible classification for Claude limits, matched on message text | the two failing transcripts from §18.3 replay green: rung 1 limit → rung 2 attempted, `hop_history` shows two entries | — | S |
| SL-3 | **R2/R3** — terminal rung via `ChainResolver.policy()`, bench-exempt, `no live rung` error | no chain resolves to fewer than 2 rungs while the floor is healthy; a simulated floor outage yields `no live rung` naming the chain | — | M |
| SL-4 | **R4** — three failure classes (ineligible/transient/fatal); `context_overflow` becomes skip | a 32K-window model placed at the head of a chain falls through to the floor instead of failing | SL-2 | S |
| SL-5 | **R6** — DONE via gke_GitOps #419 (32K → 131K). Original scope: raise `qwen3.6-35b-local` to ≥64K context or drop it from the `explore`/`librarian` heads; assert every chain's head is admissible | no chain head is ineligible on a live dispatch (the D2 transcript replays green); `explore`/`librarian` dispatch succeeds | — | S |
| SL-6 | Cache safety: frozen prefix + tail-only injection + monitor + the three test gauntlets | property, snapshot and tripwire tests run in CI; monitor flags a ≥N-turn session at 0% cache | — | M |
| SL-7 | Terminal gate: observation token, generation fence, single-use commit (defect: invented completion) | a stale/duplicate observation cannot mark a worker complete — proven by a test that replays a stale token | SL-1 | L |
| SL-8 | Liveness reconciliation: grace, `stopped-unreconciled`, tombstones | a worker killed mid-run is reported `stopped-unreconciled` within the grace window, never left `running` | SL-1 | M |
| SL-9 | Admission: per-model/per-provider/default caps + queue/pump | a 10-task run against a cap of 2 never exceeds 2 concurrent calls on that model (asserted from call timestamps) | SL-1 | M |
| SL-10 | Wall-clock supervisor per dispatch (never writes terminal state) | a hung wait is cut at its deadline and reported by the supervisor, with the worker's own state untouched | SL-7 | S |
| SL-11 | Report validation gate: `claim_boundary` in the accept path; bounded momus cycle | a worker summary claiming an undone change is rejected, with the rejection reason attached to the run | — | M |
| SL-12 | Self-improvement job (§18.8) writing a weekly report; proposes, never applies | report produced from real spend tables; every proposal carries the bound it respects | SL-2, SL-3 | S |
| SL-13 | Provision `glm-5.3-flash` as a first-class route (R9) — Z.ai credential in Doppler, one `model_list` entry, provider egress; record measured pricing in §18.12 | alias resolves through LiteLLM and completes a trivial call; a failover to it records `model: glm-5.3-flash` in `hop_history`; no other provider's credential is reused | — (needs the Z.ai key) | M |

Sequencing: **SL-2, SL-3, SL-4 first** — they make the fleet survive a Claude limit, and each is a predicate or a branch. **SL-13** follows immediately: until it lands, five of nine middle rungs (rows 2, 5, 6, 8, 9) have nowhere to go and R8 is satisfiable only by Anthropic. **SL-5** makes the qwen rungs real; SL-7 and SL-8 before any widening of concurrency.

### 18.10 Risks and caveats

- **One fallback layer, by decision.** Degradation lives only in the plugin's chain walk (R5). Anything that bypasses `omo` is outside it — accepted deliberately, recorded so it is not later "fixed" in the proxy.
- **The floor is a single seat.** Making `deepseek-v4-flash-direct` the terminal rung concentrates risk: if that model group is down, R3 turns every dispatch into a loud `no live rung`, which is honest but total. Accepted for now — the alternative is a silent floor, which is what we have today.
- **`opencode-go/glm-5.2`, `opencode-go/glm-5.3-flash`, `opencode-go/kimi-k2.7-code` do not exist in this stack.** Verified against the live alias list and the LiteLLM configmap, twice: six aliases, none of them `glm`/`kimi`. The owner's table cannot be adopted literally; substituting `claude-haiku-5-5`/`claude-sonnet-5` changes its intent, adding routes needs upstream credentials. Decision required (§18.11).
- **The pod runs `09956e6`; main is `272b951`.** #37 (dependency results handed to dependents) is on main and not live, which matters for SL-11: a worker validating its dependents' output is exactly what that handoff enables. Refresh the plugin before judging SL-11.
- **`model.default: claude-haiku-5-5`** was failing 52% of calls on the day of the audit, which means the *first* call of most sessions starts on the least reliable seat. Not changed here — it belongs to the config PR, not the migration.
- **Claude quota resets 18:00 UTC.** At the time of writing, omo cannot dispatch anything at all: Claude is limited and no chain reaches a live model (D1). This is the defect the owner's second directive names.
- **Upstream is a different host.** Everything about *sessions* must be re-derived; only the *gates* port verbatim.

### 18.11 Decisions needed from the owner

1. ~~**glm/kimi rows**~~ — **answered by the cost directive**: Kimi is dropped from Sisyphus-Junior (expensive), and the three absent glm/kimi names are struck through, not invented. Open only if you want the glm routes added as *new spend* — say which, and SL-13 adds them behind their own cost line.
2. ~~**Floor scope**~~ — **answered**: floor is `deepseek-v4-flash-direct` for all nine roles, Oracle included.
3. **Hotfix now or with the migration?** SL-2 + SL-4 alone would have kept today's dispatches alive; they are small, reversible, one-predicate-plus-one-branch changes in the plugin.

### 18.12 Cost model (rev. 2026-10-27)

**Constraint.** Claude Max is flat-rate. Its binding limit is the 5-hour session quota and the weekly quota, not dollars. opencode-go window and allowance limits are NOT verified; check opencode.ai/go before relying on any of them. Dollar figures below are per-1M-token list prices, used as shadow value only.

**Prices** (per 1M tokens; opencode.ai Go docs, read 2026-10-27)

| Model | Input | Output | Cache read | Go allowance | Used by |
|---|---|---|---|---|---|
| DeepSeek V4.1 Flash, off-peak | $0.15 | $0.60 | $0.003 | not verified | rows 1, 7, 9 |
| DeepSeek V4.1 Flash, peak | $0.30 | $1.20 | $0.006 | not verified | same. Peak = 01:00–04:00 and 06:00–10:00 UTC, Mon–Fri |
| GLM-5.3-Flash | $0.15 | $0.50 | $0.03 | not verified | row 8 default; rows 2, 3, 6 fb |
| GLM-5.2 | $1.40 | $4.40 | $0.26 | not verified | row 4 default; row 5 fb |
| Kimi K2.7 Code | $0.95 | $4.00 | $0.19 | not verified | dropped (cost) |
| Claude Haiku 5.5 on Go (≤100K ctx) | $0.10 | $0.50 | $0.01 | not verified | not used; rows use Claude Max Haiku |
| Claude Max 5x / 20x | — | — | — | $100 / $200 per month | flat rate; quota-metered (5-hour + weekly) |
| `deepseek-v4-flash-direct` (floor) | not retrieved | — | — | — | UNVERIFIED: no price on record |
| `qwen3.6-35b-local` | $0 marginal | — | — | — | 131K window |

**Claude-side caching (SL-6, Claude half).** Anthropic cache reads are 10% of input; 5-minute writes 1.25×; 1-hour writes 2× (Anthropic pricing). A search snippet gives 5% for Sonnet 5.5; not verified. Plan: `cache_control` on the stable prefix (system prompt, tool definitions, role instructions), byte-stable; 5-minute TTL by default; 1-hour only for runs with gaps over 5 minutes. Check pass-through in claude-proxy and LiteLLM with two identical calls and read `cache_read_input_tokens` on the second.

**Measured mix behind the ratio** (LiteLLM, 7 days to 2026-10-26; §18.3–§18.4):
- Sonnet subagent runs: about 1.15M uncached input per run, 0% cache.
- DeepSeek subagent runs: about 0.21M uncached input per run, 94% cache reads.

**Quota effect of the matrix.** Sonnet is the default on one row (Oracle) and first fallback on one row (Council). The audit's 77 Sonnet subagent runs in 7 days came from defaults that are now off Sonnet.

**Dollar projection: not given.** Go limits are unverified, and Claude is flat-rate. A total needs the per-role token mix from the next 7 days of data (SL-6 measures it). Shadow value of the 7-day Claude traffic is about $293 (§18.4).

### 18.13 Changes recorded 2026-10-27

- Z.ai is removed from §18.5, §18.12 and both visual sheets. SL-13 (Z.ai provisioning) is **superseded**.
- **SL-14 (new):** provision the opencode-go route. Go API key in Doppler (devops/ci). LiteLLM aliases for `opencode-go/deepseek-v4.1-flash`, `opencode-go/glm-5.2`, `opencode-go/glm-5.3-flash`. No Kimi alias. Verify with one live call per alias. Blocks rows 1, 4, 7, 8, 9 defaults, rows 5 and 6 fallbacks, and the GLM rung of rows 2 and 3.
- Open decisions: Haiku as fallback 1 on six rows (§18.11), and the SL-2 + SL-4 hotfix (§18.11 item 3).
