# Kagent — Loop Engineering System

The **kagent** services implement a *loop-engineered* AI execution model: tasks are decomposed, delegated to specialized agents, reviewed, and iterated — not answered in a single pass. This is the cluster's native AI workforce.

## What Loop Engineering Means

**Single-pass AI**: User asks → model answers → done. No verification, no iteration, no delegation.

**Loop-engineered AI**: User asks → **orchestrator** parses intent → **planner** breaks into steps → **reviewer** catches gaps → **worker** executes → **verifier** checks output → loop repeats until quality gate passes. Each role is a distinct agent with a specialized model, prompt, and execution context.

The name "headroom" (the LLM proxy) reflects this: it creates *cognitive margin* — the safety buffer between normal operation and the output ceiling — so the system can explore alternatives, backtrack, and improve before committing.

## System Architecture

| Component | Wave | Chart | Purpose |
|-----------|------|-------|---------|
| `kagent-substrate` | 3 | `services/helm/kagent-substrate/` | gVisor sandbox runtime. Replaces per-agent pods with isolated actors. |
| `kagent-headroom` | 4 | `services/helm/kagent-headroom/` | LLM proxy — OpenRouter backend, SQLite CCR cache, observability. All agent LLM traffic routes through here. |
| `kagent` | 5 | `services/helm/kagent/` | Main control plane — Agent/ModelConfig CRDs, controller, UI (port 8080), Postgres (StackGres), prompts ConfigMap. |
| `kagent-discord` | 6 | `services/helm/kagent-discord/` | Discord gateway bot — polls Discord, routes messages to A2A agent. |

**Namespace**: All run in `kagent` except `kagent-substrate` which runs in `ate-system`.

**Secrets**: `svc_kagent` Doppler config. Must include `OPENAI_API_KEY` (used by headroom proxy auth) and any provider keys headroom needs.

## Agent Taxonomy

All agents are `SandboxAgent` CRs (`kagent.dev/v1alpha2`) running on the substrate. Each has a `agent-role` label and a dedicated system prompt injected via ConfigMap.

| Agent | Role | Model | Purpose |
|-------|------|-------|---------|
| **sisyphus** | orchestrator | opencode-big-pickle | Main orchestrator. Parses intake, plans, delegates via A2A. |
| **atlas** | orchestrator | opencode-big-pickle | Master orchestrator. Coordinates agents, verifies work. |
| **prometheus** | planner | moonshotai-kimi-k2-6 | Strategic planner. Builds step-by-step implementation plans. |
| **metis** | planner | opencode-big-pickle | Pre-planning consultant. Identifies hidden intentions, ambiguities, AI failure points. |
| **momus** | reviewer | moonshotai-kimi-k2-6 | Ruthless plan reviewer. Identifies gaps, risks, ambiguities. |
| **oracle** | architect | opencode-big-pickle | Read-only architecture/security consultant. |
| **hephaestus** | worker | opencode-north-mini-code-free | Deep implementation coder. Writes production-quality code. |
| **librarian** | researcher | opencode-deepseek-v4-flash-free | Docs/RAG searcher. Finds authoritative docs, OSS examples, remote repos. |
| **explore** | researcher | opencode-deepseek-v4-flash-free | Contextual grep for codebases. |
| **multimodal-looker** | researcher | opencode-mimo-v2-5-free | Media file analyzer (PDFs, images, diagrams). |
| **ultrabrain-agent** | worker | anthropic-claude-opus-4-7 | Hard logic, architecture decisions, algorithms. |
| **sisyphus-junior** | worker | opencode-big-pickle | Focused task executor. Same discipline, no delegation. |
| **writing-agent** | worker | google-gemini-3-5-flash | Documentation, prose, technical writing. |
| **ultraworker** | worker | opencode-big-pickle | Ultrawork loop executor. |
| **opencrust** | worker | opencode-big-pickle | Shell/command specialist. |
## Intent Classification & Routing Gates

Not every request needs the full loop. **Sisyphus classifies intent inline** on every request — classification is part of orchestration, not a separate step. The orchestrator that will execute the plan is the same one that reads between the lines and decides loop depth.

### Why Inline Classification

- **Lower latency**: No extra model call before routing.
- **Full context**: Sisyphus has the complete conversation history, tool outputs, and file contents.
- **Unified reasoning**: The agent that classifies is the same one that executes. No impedance mismatch.
- **Simpler architecture**: One agent owns intent → routing → execution.

### Complexity Tiers

| Tier | Criteria | Loop Depth | Agents |
|---|---|---|---|
| **Trivial** | Single file, <10 lines, typo, rename, obvious syntax error | Direct | sisyphus-junior only |
| **Quick** | Explicit file/line, clear command, single domain | Shallow | 1 specialist (hephaestus / opencrust / writing-agent) |
| **Scoped** | Known domain, unclear location | Explore → Execute | 2–4 agents (explore + librarian in parallel, then worker) |
| **Exploratory** | "how does X work?", multi-module discovery | Research → Synthesize | 2–3 agents (librarian + explore → answer) |
| **Complex** | Multi-file, cross-cutting, architecture, security | Full loop | 5–8 agents (metis → prometheus → momus → hephaestus → oracle → atlas) |
| **Ambiguous** | Multiple interpretations with 2x+ effort difference | Ask → Re-classify | 0 agents — ask user ONE precise question |

### Domain Routing

| Domain | Primary Agent | Fallback | When to use |
|---|---|---|---|
| **Visual** (UI, CSS, styling, layout, design, animation) | hephaestus | opencrust | Always visual-engineering category |
| **Logic** (algorithms, architecture, complex business logic) | ultrabrain-agent | oracle | Use oracle for review after |
| **Writing** (docs, prose, technical writing) | writing-agent | sisyphus-junior | Caveman mode for terse output |
| **Research** (docs lookup, OSS examples, remote repos) | librarian | explore | librarian for docs, explore for codebase |
| **Git** (commits, branches, rebases, history) | opencrust | sisyphus-junior | Shell/command specialist |
| **General** | sisyphus-junior | hephaestus | Determine after exploration |

### Intent Gates

**ask_gate** — Before routing to implementation, check:
1. Is the action irreversible? (delete, push, publish) → Flag: `require_confirmation`
2. Does it have external side effects? (sending, deleting, publishing, pushing to production) → Flag: `require_confirmation`
3. Is critical information missing that would materially change the outcome? → Flag: `ask_user`

**re_entry_rule** — Don't re-classify on every turn:
- **Confirmation turn**: If user confirms/refines prior intent, do NOT re-classify from scratch. Acknowledge and act.
- **Explicit decision**: If user already chose an option ("yes do it", "A로 가자"), do NOT re-litigate. Execute.
- **Post-decision meta-question**: "what do you think?" after a decision = acknowledgment request, NOT new classification.
- **Already-in-context**: If answer is verbatim in context window, RETURN IT. Do not re-derive.

### Key Triggers (check BEFORE classifying)

- External library/source mentioned → include **librarian**
- 2+ modules involved → include **explore**
- Ambiguous or complex request → include **metis** before planner
- Work plan saved to `.sisyphus/plans/*.md` → include **momus** for review
- Security/performance concerns → include **oracle**
- "Look into" + "create PR" → Full implementation cycle, not just research

### Verification Tiers

Sisyphus enforces verification tiers based on its inline classification:

| Tier | When | Required Evidence |
|---|---|---|
| **V1** | Trivial fixes (typo, rename, comment) | `lsp_diagnostics` clean on changed file |
| **V2** | Quick/scoped changes (≤3 files, single domain) | `lsp_diagnostics` + related tests pass + run entry point once |
| **V3** | Complex/cross-cutting changes or any delegated work | `lsp_diagnostics` (zero errors) + all tests pass + build passes + manual QA |

### Why This Matters

Without classification, every request gets the full loop — a typo fix goes through metis → prometheus → momus → hephaestus → oracle → atlas. That's 6 agents, 6 model calls, 30+ seconds, and unnecessary cost.

With inline classification:
- A typo fix → sisyphus-junior → V1 verify → done. **1 agent, 3 seconds.**
- "How does X work?" → librarian + explore (parallel) → synthesize → done. **2 agents, 5 seconds.**
- "Refactor the auth layer" → full loop with review. **8 agents, 60 seconds, but correct.**

Sisyphus is the gatekeeper.

## Feedback Loop Pattern

Every primary agent has a `-fb` (feedback) counterpart that reviews its output before the loop continues:

```
User request
  → Sisyphus (orchestrator) parses intent
    → Metis (pre-planning) probes for ambiguities
      → Prometheus (planner) builds step-by-step plan
        → Momus (reviewer) catches gaps in the plan
          → Hephaestus (worker) implements the step
            → Oracle (architect) reviews for security/quality
              → Atlas (master orchestrator) verifies completion
                → Loop repeats for next step or terminates
```

The `-fb` agents are lightweight reviewers that run the same model config as their primary but with a critique-oriented system prompt. This keeps the loop tight — review happens inline rather than as a separate round-trip.

## LLM Routing (Headroom)

All agents talk to `http://kagent-headroom.kagent.svc.cluster.local:8787` via `ModelConfig` CRs. Headroom:
- Routes to OpenRouter (and thus 200+ providers)
- Caches responses in SQLite (CCR — cached completion repository)
- Provides observability on latency, cost, token usage
- Enables swapping backends without touching agent configs

**ModelConfig CRs** define the model string (e.g. `opencode/big-pickle`, `moonshotai/kimi-k2.6`) and the provider (`OpenAI` — because headroom speaks OpenAI-compatible API). The actual provider selection happens at the headroom layer via `--backend openrouter`.

## Prompt Injection System

Agent system prompts are loaded from a ConfigMap (`kagent-prompts`) mounted at runtime. The `values.yaml` includes two foundational prompt templates:

- **`caveman`** — Ultra-compressed communication mode. Drops articles, filler, pleasantries. Pattern: `[thing] [action] [reason]. [next step].`
- **`ponytail`** — YAGNI ladder. Lazy senior dev discipline: check if needed → stdlib → existing dep → one-liner → small function → new module → new abstraction.

Each agent's `systemMessageFrom` references a key in this ConfigMap (e.g. `sisyphus-system.txt`, `oracle-system.txt`). The `-fb` variants use the same base prompt with critique suffixes.

## Skills

Skills are prompt templates injected via the `kagent-prompts` ConfigMap. They provide reusable context and behavior patterns.

### Core Skills (all agents)

| Skill | Prompt Key | Purpose |
|---|---|---|
| **caveman** | `caveman.txt` | Ultra-compressed communication mode. Drops filler, keeps substance. |
| **ponytail** | `ponytail.txt` | YAGNI ladder. Lazy senior dev discipline. |

### Agent-Specific Skills

| Skill | Prompt Key | Used By |
|---|---|---|
| **cavecrew** | `cavecrew-system.txt` | Sisyphus — when to delegate to compressed subagents |
| **github-repo-fetcher** | `github-repo-fetcher-system.txt` | Librarian, Explore — fetch repos on demand |
| **repo-loader** | `repo-loader-system.txt` | Sisyphus — lazy repo loading strategy |

### Porting Local Skills

Skills from `~/.config/opencode/skills/` are ported to the ConfigMap:

1. Convert SKILL.md content to a system prompt
2. Add to `templates/prompts-configmap.yaml`
3. Reference from agent's `systemMessageFrom` if needed
4. Remove hardcoded URLs/accounts — use `.Values.*` or env vars

### Skill Lifecycle

Skills are living documents. They evolve as the system changes.

**Update triggers**:

| Trigger | Action | Owner |
|---|---|---|
| Fresh session start | Load latest skill content from ConfigMap | Sisyphus |
| ArgoCD sync | Helm renders latest prompts into ConfigMap | ArgoCD |
| Repo change | Update repo-loader skill | Developer |
| New CLI installed | Update tool-references in relevant skills | Developer |
| New agent added | Update agent taxonomy in all relevant skills | Developer |
| Prompt tuning | After test failures, refine classification prompts | Developer |

**Update flow**:

1. Developer edits skill prompt in `prompts-configmap.yaml`
2. ArgoCD detects drift, syncs new ConfigMap
3. Agents pick up new prompt on next session start
4. No agent restart required — prompts are read at runtime

**Dynamic skills** (generated at runtime):

- GitHub repo list — fetched via `gh repo list` when needed
- Local repo paths — discovered via `find ~ -name .git` when needed
- Installed CLI inventory — built from `which` checks when needed

**Versioning**: Git commit hash = skill version. ArgoCD sync wave = delivery mechanism. Agent session = consumption point.

If an agent behaves oddly, check:
1. `kubectl get cm kagent-prompts -n kagent -o yaml | grep <skill-key>`
2. `git log --oneline -5 services/helm/kagent/templates/prompts-configmap.yaml`
3. ArgoCD UI for sync status

**Testing skill changes**:

1. `helm template kagent services/helm/kagent/` — verify renders
2. Check specific prompt: `helm template ... | grep -A 20 <skill-key>`
3. Run intent classification tests against new prompt

## Adding a New Agent

1. **Create agent CR** in `services/helm/kagent/templates/agents/<name>.yaml`:
   ```yaml
   apiVersion: kagent.dev/v1alpha2
   kind: SandboxAgent
   metadata:
     name: my-agent
     labels:
       agent-type: primary
       agent-role: worker   # or planner, reviewer, orchestrator, researcher, architect
   spec:
     type: Declarative
     description: "What this agent does."
     declarative:
       runtime: go
       modelConfig: opencode-big-pickle
       systemMessageFrom:
         name: kagent-prompts
         key: my-agent-system.txt
         type: ConfigMap
     platform: substrate
     substrate:
       workerPoolRef:
         name: kagent-default
   ```
2. **Add feedback agent** `agents/<name>-fb.yaml` with `agent-role: reviewer` and critique prompt.
3. **Add system prompt** to `templates/prompts-configmap.yaml`.
4. **Ensure ModelConfig exists** in `templates/modelconfigs/` if using a new model.
5. **ArgoCD sync** — no Terraform changes needed.
