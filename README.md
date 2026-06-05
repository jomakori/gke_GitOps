# gke_GitOps

ArgoCD App-of-Apps repository for the **jmak-lab** Minikube cluster. Terraform (from [k8s-maklab-cluster](https://github.com/jomakori/devops_Terraform)) creates the top-level ArgoCD `Application` resources that point here; ArgoCD syncs automatically (prune + self-heal, exponential backoff retry).

## Structure

```
.
├── services/          ← 3rd-party infrastructure
│   ├── helm/          ← Helm charts (14 services)
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

| Wave | Service | Chart | Purpose | Status |
|------|---------|-------|---------|--------|
| 0 | [metrics-server](services/helm/metrics-server/) | metrics-server/metrics-server | Resource usage aggregation for HPA | enabled |
| 0 | [cert-manager](services/helm/cert-manager/) | jetstack/cert-manager | Automated TLS via Let's Encrypt + Cloudflare DNS-01 | enabled |
| 1 | [external-secrets](services/helm/external-secrets/) | external-secrets/external-secrets | Doppler secret injection via ESO | enabled |
| 2 | [istio](services/helm/istio/) | custom umbrella | base + istiod + ingress gateway (single chart, 3 upstream deps) | enabled |
| 3 | [external-dns](services/helm/external-dns/) | external-dns/external-dns | Cloudflare DNS records from Istio Gateway hosts | enabled |
| 3 | [postgres-operator](services/helm/postgres-operator/) | stackgres-operator | PostgreSQL operator (StackGres) | enabled |
| 3 | [keda](services/helm/keda/) | kedacore/keda | Event-driven autoscaling | **disabled** |
| 3 | [mongodb-operator](services/helm/mongodb-operator/) | psmdb-operator | MongoDB operator (Percona) | **disabled** |
| 4 | [kube-prometheus-stack](services/helm/kube-prometheus-stack/) | prometheus-community/kube-prometheus-stack | Cluster monitoring, metrics, alerting, Grafana | enabled |
| 4 | [cloudflare-tunnel](services/helm/cloudflare-tunnel/) | custom | Cloudflare Zero Trust tunnel — ingress via Cloudflare edge | enabled |
| 4 | [onedev](services/helm/onedev/) | custom (vendored upstream + SGCluster) | All-in-one DevOps platform (Git, CI/CD, issue tracker) with StackGres PostgreSQL | enabled |
| 4 | [redis-operator](services/helm/redis-operator/) | ot-operator/redis-operator | Redis cluster management | **disabled** |

Dependency chain: cert-manager → external-secrets → istio umbrella (CRDs → control plane → ingress gateway → config, reconciled by Kubernetes) → wave 3/4 services. kube-prometheus-stack at wave 4 ensures external-secrets ClusterSecretStores exist before its Grafana ExternalSecret syncs.

### Apps

Both apps use a [single parameterized chart](apps/helm/) (chart name: `apps`) invoked via `--set appName=<key>`. All manifests (Deployment, Service, HPA, VirtualService, ExternalSecret, PVC) are generated from a single `_helpers.tpl` — no per-app chart directories.

| App Key | Environments | Status |
|---------|-------------|--------|
| `demoApi` | staging + production | `enable: false` (ready to activate) |
| `notesUi` | staging + production | `enable: false` (ready to activate) |

Toggled via `apps/argocd-appset/values.yaml`.

## Secrets

No secrets in this repo. The chain:

1. **Doppler** stores actual values in project+config pairs.
2. **Terraform** stores a personal token as a K8s Secret in `external-secrets`.
3. **ClusterSecretStore** resources (one per config) reference that token with their `project` + `config`.
4. **ExternalSecrets** use `dataFrom.extract` (zero rewrite rules) — K8s Secret keys match Doppler key names. `refreshInterval: 1m`.
5. **Pods** consume via standard `secretKeyRef`.

| Doppler Config | Used By | Secrets |
|---------------|---------|---------|
| `svc_grafana` | Grafana | `GRAFANA_ADMIN`, `GRAFANA_PW` |
| `svc_cloudflare` | istio (umbrella), external-dns, cloudflare-tunnel | `CF_API_TOKEN`, `TUNNEL_TOKEN` |
| `svc_postgres_operator` | postgres-operator (StackGres) | `ADMIN_USER`, `ADMIN_PASSWORD` |
| `svc_onedev` | onedev | `DB_PASSWORD`, `DB_USER` |
| `svc_mongodb` | mongodb-operator | `MONGODB_USER`, `MONGODB_PW`, `MONGODB_DB` |

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
