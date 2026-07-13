# gke_GitOps

ArgoCD App-of-Apps repository for the **jmak-lab** Minikube cluster. Terraform (from [k8s-maklab-cluster](https://github.com/jomakori/devops_Terraform)) creates the top-level ArgoCD `Application` resources that point here; ArgoCD syncs automatically (prune + self-heal, exponential backoff retry).

## Structure

```
.
├── services/          ← 3rd-party infrastructure
│   ├── helm/          ← Helm charts (26 services incl. openagent stack)
│   └── argocd-appset/ ← App-of-Apps manifests (single applications.yaml template)
├── apps/              ← Application workloads
│   ├── helm/          ← Single parameterized Helm chart (chart name: apps)
│   └── argocd-appset/ ← App-of-Apps manifests
├── .github/workflows/ ← PR lint workflow
├── .pre-commit-config.yaml
├── .ct-config.yml
├── renovate.json
└── devbox.json
```

### Services

All services registered in `services/argocd-appset/values.yaml` — synced in wave order by ArgoCD:

**Wave Philosophy**: Init services → Secret services → Core networking services → Edge networking services → Operator services → General services → Apps

| Wave | Service | Chart | Purpose | Status |
|------|---------|-------|---------|--------|
| 0 | [local-path](services/helm/local-path/) | rancher/local-path-provisioner | Default k3s storage class — single working provisioner | enabled |
| 0 | [cert-manager](services/helm/cert-manager/) | jetstack/cert-manager | Automated TLS via Let's Encrypt + Cloudflare DNS-01 | enabled |
| 0 | [metrics-server](services/helm/metrics-server/) | metrics-server/metrics-server | Resource usage aggregation for HPA | enabled |
| 0 | [vpa](services/helm/vpa/) | fairwinds/vpa | Vertical Pod Autoscaler — auto-adjust CPU/memory requests | enabled |
| 1 | [external-secrets](services/helm/external-secrets/) | external-secrets/external-secrets | Doppler secret injection via ESO | enabled |
| 2 | [istio](services/helm/istio/) | custom umbrella | base + istiod + ingress gateway (single chart, 3 upstream deps) | enabled |
| 2 | [openagent](services/helm/openagent/) | custom umbrella | LiteLLM + headroom + discord bot + CRDs — 10-persona loop engineering in single umbrella chart | enabled |
| 3 | [cloudflare-tunnel](services/helm/cloudflare-tunnel/) | custom | Cloudflare Zero Trust tunnel — ingress via Cloudflare edge | enabled |
| 4 | [external-dns](services/helm/external-dns/) | external-dns/external-dns | Cloudflare DNS records from Istio Gateway hosts | enabled |
| 4 | [postgres-operator](services/helm/postgres-operator/) | stackgres-operator | PostgreSQL operator (StackGres) | enabled |
| 4 | [keda](services/helm/keda/) | kedacore/keda | Event-driven autoscaling | not enabled |
| 4 | [mongodb-operator](services/helm/mongodb-operator/) | psmdb-operator | MongoDB operator (Percona) | not enabled |
| 5 | [kube-prometheus-stack](services/helm/kube-prometheus-stack/) | prometheus-community/kube-prometheus-stack | Cluster monitoring, metrics, alerting, Grafana | enabled |
| 5 | [onedev](services/helm/onedev/) | custom (vendored upstream + SGCluster) | All-in-one DevOps platform (Git, CI/CD, issue tracker) with StackGres PostgreSQL | enabled |
| 5 | [redis-operator](services/helm/redis-operator/) | ot-operator/redis-operator | Redis cluster management | not enabled |
| 5 | [headlamp](services/helm/headlamp/) | headlamp | Kubernetes dashboard UI | enabled |
| 5 | [opencost](services/helm/opencost/) | opencost | Cost allocation and monitoring | enabled |

Dependency chain: local-path + cert-manager + VPA → external-secrets (ClusterSecretStores) → openagent umbrella (LiteLLM + headroom + discord bot + CRDs) → istio umbrella (CRDs → control plane → ingress gateway → config) → wave 3+ services. The openagent stack is bootstrapped early so it is ready to serve before wave 4 operators arrive.

### OpenAgent Loop Engineering System

The **openagent** services implement a *loop-engineered* AI execution model: tasks are decomposed, delegated to specialized personas, reviewed, and iterated — not answered in a single pass. This is the cluster's native AI workforce, built on the **Sympozium** orchestrator.

#### Components

The openagent umbrella chart (`services/helm/openagent/`) bundles all components:

| Component | Deployed Via | Purpose |
|-----------|-------------|---------|
| `openagent-crds` | sympozium CLI (out-of-band) | `sympozium.ai/v1alpha1` CRDs (Ensemble, SkillPack). Not a Helm chart — installed directly by CLI. |
| `openagent-headroom` | umbrella subchart (`charts/openagent-component`) | LLM proxy (`chopratejas/headroom`) — routes to LiteLLM, SQLite CCR cache. |
| `openagent-litellm` | umbrella upstream dep (`charts/litellm-helm`) | Multi-provider LLM gateway v1.92.0 — 12 models, fallback chains. |
| `openagent-discord` | umbrella subchart (`charts/openagent-component`) | Discord gateway bot (Go binary) — OpenAI-compatible chat completions. |
| `openagent` templates | umbrella locals (`templates/`) | Ensemble + SkillPacks + StackGres + istio gateway. 10-persona loop engineering. |

**Chart structure** (`services/helm/openagent/`):
```
openagent/                       ← umbrella (v2.0.0)
├── charts/
│   ├── litellm-helm/            ← upstream dep (vendored, OCI fallback)
│   └── openagent-component/      ← custom subchart (headroom + bot, 15 files)
├── templates/                    ← CRDs only
│   ├── ensemble/                 ← omo-loop-engineering (10 personas)
│   ├── skillpacks/               ← 3 SkillPack CRDs
│   ├── db/                       ← StackGres CRDs (SGCluster, config)
│   ├── shared/                   ← ExternalSecret, GHCR, VPA
│   └── gateway/                  ← Istio VirtualService + AuthPolicy
├── src/                          ← Bot Go source code
├── values.yaml                   ← 177 lines, full config surface
└── Chart.yaml                    ← 2 deps + conditions
```

**Namespaces**: Application resources live in `openagent`. The c control plane runs separately in `sympozium-system` (out of band — installed by the sympozium CLI). The Discord bot calls `http://omo-loop-engineering-sisyphus-web-endpoint-server.sympozium-system.svc.cluster.local:8080/v1/chat/completions`.

**Secrets**: `svc_openagent` Doppler config. Must include provider keys (DeepSeek, MiniMax, z.ai, Anthropic, Moonshot, OpenCode), `AGENT_API_URL` (Sympozium Sisyphus web endpoint), `AGENT_API_KEY` (endpoint auth token), `DISCORD_BOT_TOKEN`, `DISCORD_BOT_CLIENT_ID`, and `GITHUB_TOKEN` (for GHCR image pulls). All flow via `envFrom: secretRef` in deployment templates — no Helm `--set` parameters for secrets.

#### The 10 Personas (Ensemble `omo-loop-engineering`)

| Persona | Role | Model | Purpose |
|---------|------|-------|---------|
| **sisyphus** | orchestrator | deepseek-v4-pro | Main entry point — intent classification, delegation, verification enforcement. |
| **atlas** | orchestrator | deepseek-v4-pro | Cross-persona coordination, quality verification, supervision. |
| **prometheus** | planner | deepseek-v4-pro | Strategic planner — builds step-by-step plans from objectives. |
| **metis** | planner | deepseek-v4-pro | Pre-planning consultant — hidden intentions, ambiguity, AI failure points. |
| **momus** | reviewer | deepseek-v4-pro | Ruthless plan reviewer — gaps, risks, missing context. |
| **oracle** | architect | deepseek-v4-pro | Read-only architecture/security consultant. |
| **hephaestus** | worker | minimax/M3 | Deep implementation coder — production-quality code. |
| **sisyphus-junior** | worker | deepseek-v4-flash | Focused task executor — no re-delegation. |
| **librarian** | researcher | zai/glm-4.7-flash | Docs/RAG searcher — web search, official documentation, OSS examples. |
| **explore** | researcher | zai/glm-4.7-flash | Codebase pattern discovery — grep, glob, file reading. |

**Delegation graph** (spec.relationships): sisyphus → {prometheus, metis, hephaestus, oracle, atlas}; atlas → {librarian, explore, sisyphus-junior}; prometheus → momus. Supervision: atlas → {sisyphus, prometheus, hephaestus}. Stimulus: `omo-loop-engineering` → sisyphus.

**Skills attached**: sisyphus loads `k8s-ops`, `omo-core-skills`, `hashline-editor`, `web-endpoint`. Other personas inherit ensemble defaults.

#### LLM Routing

```
openagent-discord (Go bot)
  → POST /v1/chat/completions
  → omo-loop-engineering-sisyphus-web-endpoint-server.sympozium-system.svc:8080
    → openagent-headroom.openagent.svc:8787   (headroom proxy, CCR cache)
      → litellm-openagent-litellm.openagent.svc:4000/v1   (LiteLLM gateway)
        → provider APIs (DeepSeek, MiniMax, z.ai, Anthropic, Moonshot, OpenCode)
```

The headroom proxy is configured with `OPENAI_TARGET_API_URL=http://litellm-openagent-litellm.openagent.svc.cluster.local:4000/v1` — all upstream LLM calls route through LiteLLM, never directly to OpenRouter.

See `services/helm/openagent/templates/ensemble-omo-loop-engineering.yaml` for the full system prompts, delegation rules, and verification tiers.

### Apps

Apps at **wave 6+** (depend on all services being ready). Both apps use a [single parameterized chart](apps/helm/) (chart name: `apps`) invoked via `--set appName=<key>`. All manifests (Deployment, Service, HPA, VirtualService, ExternalSecret, PVC) are generated from a single `_helpers.tpl` — no per-app chart directories.

| Wave | App Key | Environments | Status |
|------|---------|-------------|--------|
| 6 | `demoApi` | staging + production | `enable: false` (ready to activate) |
| 6 | `notesUi` | staging + production | `enable: false` (ready to activate) |

Toggled via `apps/argocd-appset/values.yaml`.

## Secrets

No secrets in this repo. The chain:

1. **Doppler** stores actual values in project+config pairs.
2. **Terraform** stores a personal token as a K8s Secret in `external-secrets`.
3. **ClusterSecretStore** resources (one per config) reference that token with their `project` + `config`.
4. **ExternalSecrets** use `dataFrom.extract` (zero rewrite rules) — K8s Secret keys match Doppler key names. `refreshInterval: 24h`.
5. **Pods** consume via standard `secretKeyRef`.

| Doppler Config | Used By | Secrets |
|---------------|---------|---------|
| `svc_grafana` | Grafana | `GRAFANA_ADMIN`, `GRAFANA_PW` |
| `svc_cloudflare` | istio (umbrella), external-dns, cloudflare-tunnel | `CF_API_TOKEN`, `TUNNEL_TOKEN` |
| `svc_postgres_operator` | postgres-operator (StackGres) | `ADMIN_USER`, `ADMIN_PASSWORD` |
| `svc_onedev` | onedev | `DB_PASSWORD`, `DB_USER` |
| `svc_openagent` | openagent, openagent-headroom, litellm (openagent), openagent-discord | Provider keys: `DEEPSEEK_API_KEY`, `MINIMAX_API_KEY`, `MINIMAX_API_BASE`, `ZAI_API_KEY`, `ANTHROPIC_API_KEY`, `MOONSHOT_API_KEY`, `OPENCODE_API_KEY`, `OPENCODE_API_BASE`. Discord: `DISCORD_BOT_TOKEN`, `DISCORD_BOT_CLIENT_ID`, `AGENT_API_URL` (Sisyphus web endpoint), `AGENT_API_KEY` (endpoint auth). GHCR: `GITHUB_TOKEN`. |


New secrets are added in the Doppler dashboard — the ExternalSecret already pulls the entire config.

## Adding a Service or App

For **services**, the `applications.yaml` template auto-generates Application resources from `services/argocd-appset/values.yaml`:
1. **Create the Helm chart** under `services/helm/<name>/` (or add upstream dependency in `Chart.yaml`).
2. **Register it** in `services/argocd-appset/values.yaml` with an `enable: true/false` flag, sync wave, and namespace.
3. **Wire secrets** via ESO: add a `dopplerConfig` key in the values entry matching a ClusterSecretStore. No Terraform changes needed.
4. **If public ingress is needed**, set `gateways.enable_public: true` — the template auto-generates a VirtualService via the istio umbrella chart. For custom subdomains or non-default service names:

   ```yaml
   gateways:
     enable_public: true       # required
     subdomain: my-app          # optional — defaults to chart name
     destination:
       serviceName: my-svc      # optional — defaults to chart name
       servicePort: 8080        # optional — defaults to 80
   ```

   The template derives everything from centralized `clusterDomain` + `destNamespace`: host → `{subdomain}.{clusterDomain}`, dest → `{serviceName}.{destNamespace}.svc.cluster.local`, VS name → `{subdomain}`.
5. **Validate locally**:
   ```bash
   .useful-scripts/ct_check.sh services/helm/<name>
   ```
6. **PR and merge** — ArgoCD auto-syncs.

For **apps**, the single parameterized chart at `apps/helm/` generates all manifests:
1. **Add an entry** in `apps/argocd-appset/values.yaml` with app key, environments, and dopplerConfig per environment.
2. **Add a namespace** in `apps/argocd-appset/templates/namespaces.yaml`.
3. **Set `enable: true`** — both apps are currently disabled, ready for activation when workloads are ready.
4. **PR and merge** — ArgoCD auto-syncs.

## CI & Automation

| Tool | What |
|------|------|
| **GitHub Actions** | `.github/workflows/pull_request.yaml` — lint/check on PRs |
| **Renovate** | `renovate.json` — auto-updates Helm chart versions every Tuesday |
| **Pre-commit** | `.pre-commit-config.yaml` — merge conflict check, trailing whitespace, detect-secrets, yamllint, helm-docs |
| **Chart Testing** | `.ct-config.yml` — dry-run validation via `.useful-scripts/ct_check.sh` |
| **Devbox** | `devbox.json` — reproducible shell with `yq-go` + `git` for image tag bumps |

## Prerequisites (Local Testing)

- kubectl, helm, [ct](https://github.com/helm/chart-testing), yamllint (macOS/Linux)

## Tips

Node scheduling with tolerations and affinity:

```yaml
tolerations:
  - key: "dedicated"
    operator: "Equal"
    value: "apps"
    effect: "NoSchedule"
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
        - matchExpressions:
            - key: class
              operator: In
              values:
                - guaranteed
```

Port-forward for local access:

```bash
kubectl port-forward -n <namespace> svc/<service-name> 8080:80
```
