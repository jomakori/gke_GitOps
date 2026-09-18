# gke_GitOps

ArgoCD App-of-Apps repository for the **jmak-lab** Minikube cluster. Terraform (from [k8s-maklab-cluster](https://github.com/jomakori/devops_Terraform)) creates the top-level ArgoCD `Application` resources that point here; ArgoCD syncs automatically (prune + self-heal, exponential backoff retry).

## Structure

```
.
├── services/          ← 3rd-party infrastructure
│   ├── helm/          ← Helm charts (25 services incl. openagent stack)
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
| 2 | [openagent](services/helm/openagent/) | custom umbrella | LiteLLM + hermes agent + CRDs — 11-agent OMO fleet (loop engineering) in single umbrella chart | enabled |
| 3 | [cloudflare-tunnel](services/helm/cloudflare-tunnel/) | hybrid | Cloudflare Zero Trust tunnel — ingress via Cloudflare edge | enabled |
| 4 | [external-dns](services/helm/external-dns/) | external-dns/external-dns | Cloudflare DNS records from Istio Gateway hosts | enabled |
| 4 | [postgres-operator](services/helm/postgres-operator/) | stackgres-operator | PostgreSQL operator (StackGres) | enabled |
| 4 | [keda](services/helm/keda/) | kedacore/keda | Event-driven autoscaling | not enabled |
| 4 | [mongodb-operator](services/helm/mongodb-operator/) | psmdb-operator | MongoDB operator (Percona) | not enabled |
| 5 | [kube-prometheus-stack](services/helm/kube-prometheus-stack/) | prometheus-community/kube-prometheus-stack | Cluster monitoring, metrics, alerting, Grafana | enabled |

| 5 | [redis-operator](services/helm/redis-operator/) | ot-operator/redis-operator | Redis cluster management | not enabled |
| 5 | [headlamp](services/helm/headlamp/) | headlamp | Kubernetes dashboard UI | enabled |
| 5 | [opencost](services/helm/opencost/) | opencost | Cost allocation and monitoring | enabled |

Dependency chain: local-path + cert-manager + VPA → external-secrets (ClusterSecretStores) → openagent umbrella (remote OCI + local subcharts) → istio umbrella (CRDs → control plane → ingress gateway → config) → wave 3+ services. The openagent stack is bootstrapped early so it is ready to serve before wave 4 operators arrive.

### OpenAgent Loop Engineering System

The openagent umbrella — LLM gateway, Hermes Agent gateway, web UI, Claude proxy, the OMO agent fleet, and MCP manifest verification — is documented in its own chart README: [`services/helm/openagent/README.md`](services/helm/openagent/README.md). It covers the components, chart structure, runtime/toolchain and health model, agent fleet, skills, LLM routing, and the web dashboard.

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
| `svc_argocd` | argocd ApplicationSet PR generator (apps/openkite-preview, OKT-75) | `ARGOCD_GITHUB_TOKEN` (repo-scoped fine-grained PAT: Pull requests read) |

| `svc_openagent` | openagent, litellm (openagent), openagent-discord | Provider keys: `DEEPSEEK_API_KEY`, `MINIMAX_API_KEY`, `MINIMAX_API_BASE`, `ZAI_API_KEY`, `ANTHROPIC_API_KEY`, `MOONSHOT_API_KEY`, `OPENCODE_API_KEY`, `OPENCODE_API_BASE`. Discord: `DISCORD_BOT_TOKEN`, `DISCORD_BOT_CLIENT_ID`, `AGENT_API_URL` (Sisyphus web endpoint), `AGENT_API_KEY` (endpoint auth). GHCR: `GITHUB_TOKEN`. |


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
| **GitHub Actions** | `.github/workflows/helm_lint-test.yaml` — lint/check on PRs |
| **Renovate** | `renovate.json` — auto-updates Helm chart versions every Tuesday |
| **Pre-commit** | `.pre-commit-config.yaml` — merge conflict check, trailing whitespace, detect-secrets, yamllint, helm-docs |
| **Chart Testing** | `.ct-config.yml` — dry-run validation via `.useful-scripts/ct_check.sh` |
| **Devbox** | `devbox.json` — reproducible shell with `yq-go` + `git` for image tag bumps |

## CodeGraph (token-efficient code lookup)

This repo is indexed with [CodeGraph](https://github.com/colbymchenry/codegraph) — a local, auto-syncing code knowledge graph that agents query over MCP in a single call instead of grepping and reading whole files. Fully local: no service, no API key.

The CLI is installed (`codegraph --version`) and the MCP server is wired globally for opencode as `mcp.codegraph` → `codegraph serve --mcp` in `~/.config/opencode/opencode.jsonc`; the agent-side rule lives in the `CODEGRAPH_START`/`CODEGRAPH_END` block of `~/.config/opencode/AGENTS.md`. Re-wire any agent with `codegraph install --target opencode --location global --yes`.

```bash
codegraph init --yes     # build .codegraph/ (one time; the file watcher keeps it fresh)
codegraph status         # files / nodes / edges
codegraph sync           # force a catch-up (only needed if the watcher is off)

# prefer these over grep/find for "where is X / what calls Y / what breaks if I change Z"
codegraph explore "how are image tags bumped"
codegraph query <symbol>
codegraph callers <symbol>
codegraph impact <symbol>
```

- `.codegraph/` is a local artifact — never commit it (gitignored here).
- The index is **per working tree**: a `git worktree` needs its own `codegraph init`; `codegraph.json` excludes `.worktrees/` so worktree copies never bloat the parent graph.
- `codegraph uninit` removes a project's index; `codegraph telemetry off` disables the anonymous usage stats.

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

# Claude proxy local access (for OpenCode provider)
kubectl port-forward svc/claude-proxy -n openagent 4523:4523
```
