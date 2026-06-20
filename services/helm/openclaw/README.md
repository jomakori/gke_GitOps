# openclaw

![Version: 0.2.0](https://img.shields.io/badge/Version-0.2.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 2026.5.22](https://img.shields.io/badge/AppVersion-2026.5.22-informational?style=flat-square)
OpenClaw AI assistant gateway, powered by OpenCrust

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| local |  |  |

## Under the hood

This chart deploys [OpenCrust](https://github.com/opencrust-org/opencrust) — the Rust implementation of the OpenClaw gateway — behind the `openclaw` service name.

### Doppler config

The ExternalSecret pulls the entire `svc_openclaw` Doppler config. Required keys:

| Secret | Purpose |
|--------|---------|
| `ANTHROPIC_API_KEY` | Claude provider |
| `DEEPSEEK_API_KEY` | DeepSeek provider |
| `MOONSHOT_API_KEY` | Moonshot / Kimi provider |
| `OPENCODE_API_KEY` | OpenCode Zen provider |
| `WHATSAPP_AGENT_NUMBER` | WhatsApp account linked via QR |
| `WHATSAPP_ALLOW_FROM` | Allowed self-chat sender |

### WhatsApp QR pairing

After the pod starts, exec in and run:

```bash
opencrust channels login --channel whatsapp
```

Scan the QR code with the phone number stored in `WHATSAPP_AGENT_NUMBER`.

### Ingress and auth

OpenCrust binds to `0.0.0.0:3888` and exposes only `host`, `port`, and `api_key`. Public access is handled by the Istio umbrella VirtualService (subdomain `openclaw`) and Cloudflare Access at the edge.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| config.agent.default_provider | string | `"opencode-big-pickle"` |  |
| config.agent.max_context_tokens | int | `100000` |  |
| config.agent.max_tokens | int | `4096` |  |
| config.agent.system_prompt | string | `"You are OpenClaw, a personal AI assistant running on Kubernetes.\nYou use oh-my-openagent for multi-agent orchestration and task delegation.\n\nWhen the user gives a request:\n1. ANALYZE — understand what's needed\n2. PLAN — break it down into subtasks\n3. DELEGATE — use the `handoff` tool to dispatch work to specialized sub-agents\n4. REVIEW — check results from sub-agents\n5. SYNTHESIZE — combine results into a coherent answer\n\nAvailable sub-agents:\n- sisyphus — orchestrator for complex multi-step tasks\n- hephaestus — builder for implementation work\n- oracle — architecture and design consultant (read-only)\n- librarian — codebase search and documentation lookup\n- explore — contextual grep and pattern discovery\n- prometheus — task planning and work breakdown\n- metis — pre-planning ambiguity analysis\n- momus — plan review and gap detection\n- atlas — heavy-lifting implementation and refactoring\n- sisyphus-junior — focused single-task executor\n- ultraworker — deep research on hard problems\n- coder — code generation and implementation\n- researcher — deep research and analysis\n\nWhen a task is simple, handle it directly. For complex or multi-step\nwork, always delegate via handoff.\n"` |  |
| config.agents.atlas.provider | string | `"opencode-big-pickle"` |  |
| config.agents.atlas.system_prompt | string | `"You are Atlas. Heavy-lifting implementation and\ncross-file refactoring. Follow existing patterns.\n"` |  |
| config.agents.coder.provider | string | `"deepseek"` |  |
| config.agents.coder.system_prompt | string | `"You are a coder agent. Write clean, correct code.\nFollow existing patterns, don't suppress type errors,\nand verify your work.\n"` |  |
| config.agents.explore.provider | string | `"opencode-deepseek-flash"` |  |
| config.agents.explore.system_prompt | string | `"You are Explore. Contextual grep for codebases.\nFind patterns, locate files, map module structure.\n"` |  |
| config.agents.hephaestus.provider | string | `"opencode-north-mini"` |  |
| config.agents.hephaestus.system_prompt | string | `"You are Hephaestus, a builder agent. Implement features,\nfix bugs, and write tests. Follow existing patterns.\n"` |  |
| config.agents.librarian.provider | string | `"opencode-deepseek-flash"` |  |
| config.agents.librarian.system_prompt | string | `"You are Librarian. Search codebases, read docs, find\nimplementation examples. Return concise answers.\n"` |  |
| config.agents.metis.provider | string | `"opencode-big-pickle"` |  |
| config.agents.metis.system_prompt | string | `"You are Metis, pre-planning consultant. Identify hidden\nintentions, ambiguities, and AI failure points.\n"` |  |
| config.agents.momus.provider | string | `"moonshot-kimi"` |  |
| config.agents.momus.system_prompt | string | `"You are Momus, plan critic. Evaluate plans for clarity,\nverifiability, and completeness. Catch gaps.\n"` |  |
| config.agents.multimodal-looker.provider | string | `"opencode-mimo"` |  |
| config.agents.multimodal-looker.system_prompt | string | `"You are Multimodal Looker. Analyze images, PDFs,\nand diagrams. Extract specific information.\n"` |  |
| config.agents.opencrust.provider | string | `"opencode-big-pickle"` |  |
| config.agents.opencrust.system_prompt | string | `"You are OpenCrust, the sidecar orchestrator.\nRoute requests to the right agent and synthesize results.\n"` |  |
| config.agents.oracle.provider | string | `"opencode-big-pickle"` |  |
| config.agents.oracle.system_prompt | string | `"You are Oracle, a read-only architecture consultant.\nAnalyze designs, identify problems, and propose solutions.\nDo not implement. Do not write code.\n"` |  |
| config.agents.prometheus.provider | string | `"moonshot-kimi"` |  |
| config.agents.prometheus.system_prompt | string | `"You are Prometheus, plan agent. Break down tasks into\nstructured work plans with parallel execution opportunities.\n"` |  |
| config.agents.researcher.provider | string | `"opencode"` |  |
| config.agents.researcher.system_prompt | string | `"You are a research agent. Find information, read docs,\nand summarize findings. Be thorough and cite sources.\n"` |  |
| config.agents.sisyphus-junior.provider | string | `"moonshot-kimi"` |  |
| config.agents.sisyphus-junior.system_prompt | string | `"You are Sisyphus-Junior, focused task executor.\nSame discipline as Sisyphus, no delegation.\n"` |  |
| config.agents.sisyphus.provider | string | `"opencode-big-pickle"` |  |
| config.agents.sisyphus.system_prompt | string | `"You are Sisyphus, orchestrator agent. Decompose complex tasks\ninto parallel subtasks and oversee execution.\nUse handoff to delegate to sub-agents when needed.\n"` |  |
| config.agents.ultraworker.provider | string | `"opencode-big-pickle"` |  |
| config.agents.ultraworker.system_prompt | string | `"You are Ultraworker. Autonomous problem-solving on\nhairy problems requiring deep research.\n"` |  |
| config.channels.whatsapp.allow_from[0] | string | `"${WHATSAPP_ALLOW_FROM}"` |  |
| config.channels.whatsapp.chunk_mode | string | `"length"` |  |
| config.channels.whatsapp.dm_policy | string | `"allowlist"` |  |
| config.channels.whatsapp.enabled | bool | `true` |  |
| config.channels.whatsapp.reaction_level | string | `"minimal"` |  |
| config.channels.whatsapp.reply_to_mode | string | `"off"` |  |
| config.channels.whatsapp.self_chat_mode | bool | `true` |  |
| config.channels.whatsapp.send_read_receipts | bool | `true` |  |
| config.channels.whatsapp.text_chunk_limit | int | `4000` |  |
| config.channels.whatsapp.type | string | `"whatsapp"` |  |
| config.data_dir | string | `"/data"` |  |
| config.gateway.host | string | `"0.0.0.0"` |  |
| config.gateway.port | int | `3888` |  |
| config.llm.anthropic.model | string | `"claude-sonnet-4-5-20250929"` |  |
| config.llm.anthropic.provider | string | `"anthropic"` |  |
| config.llm.claude-haiku.model | string | `"claude-haiku-4-5-20251001"` |  |
| config.llm.claude-haiku.provider | string | `"anthropic"` |  |
| config.llm.claude-opus.model | string | `"claude-opus-4-6"` |  |
| config.llm.claude-opus.provider | string | `"anthropic"` |  |
| config.llm.deepseek.model | string | `"deepseek-chat"` |  |
| config.llm.deepseek.provider | string | `"deepseek"` |  |
| config.llm.moonshot-kimi.base_url | string | `"https://api.moonshot.ai/v1"` |  |
| config.llm.moonshot-kimi.model | string | `"kimi-k2.6"` |  |
| config.llm.moonshot-kimi.provider | string | `"openai"` |  |
| config.llm.moonshot.base_url | string | `"https://api.moonshot.ai/v1"` |  |
| config.llm.moonshot.model | string | `"moonshot-v1-auto"` |  |
| config.llm.moonshot.provider | string | `"openai"` |  |
| config.llm.opencode-big-pickle.base_url | string | `"https://opencode.ai/zen/v1"` |  |
| config.llm.opencode-big-pickle.model | string | `"big-pickle"` |  |
| config.llm.opencode-big-pickle.provider | string | `"openai"` |  |
| config.llm.opencode-deepseek-flash.base_url | string | `"https://opencode.ai/zen/v1"` |  |
| config.llm.opencode-deepseek-flash.model | string | `"deepseek-v4-flash-free"` |  |
| config.llm.opencode-deepseek-flash.provider | string | `"openai"` |  |
| config.llm.opencode-mimo.base_url | string | `"https://opencode.ai/zen/v1"` |  |
| config.llm.opencode-mimo.model | string | `"mimo-v2.5-free"` |  |
| config.llm.opencode-mimo.provider | string | `"openai"` |  |
| config.llm.opencode-north-mini.base_url | string | `"https://opencode.ai/zen/v1"` |  |
| config.llm.opencode-north-mini.model | string | `"north-mini-code-free"` |  |
| config.llm.opencode-north-mini.provider | string | `"openai"` |  |
| config.llm.opencode.base_url | string | `"https://opencode.ai/zen/v1"` |  |
| config.llm.opencode.model | string | `"opencode-zen"` |  |
| config.llm.opencode.provider | string | `"openai"` |  |
| config.log_level | string | `"info"` |  |
| config.memory.enabled | bool | `true` |  |
| dopplerConfig | string | `""` | Doppler project; set by ArgoCD appset. Creates ExternalSecret when present.    Expected env vars: ANTHROPIC_API_KEY, WHATSAPP_ACCESS_TOKEN, etc. |
| enable_scaling | bool | `false` | HPA scaling (optional) |
| hpa.maxReplicas | int | `3` |  |
| hpa.minReplicas | int | `1` |  |
| hpa.targetCPUUtilizationPercentage | int | `80` |  |
| hpa.targetMemoryUtilizationPercentage | int | `80` |  |
| image | object | `{"pullPolicy":"IfNotPresent","repository":"ghcr.io/opencrust-org/opencrust","tag":"latest"}` | Container image |
| initImage | object | `{"pullPolicy":"IfNotPresent","repository":"busybox","tag":"1.36.1"}` | Init container image for config rendering (needs sh + sed) |
| resources | object | `{"limits":{"cpu":"2000m","memory":"2Gi"},"requests":{"cpu":"200m","memory":"512Mi"}}` | Resource requests/limits |
| service | object | `{"port":3888}` | Service settings |
| storage | object | `{"accessMode":"ReadWriteMany","size":"5Gi","storageClass":""}` | Storage (ReadWriteMany for multi-replica scaling) |
