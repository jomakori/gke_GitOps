# gke_GitOps

ArgoCD App-of-Apps repository for the **jmak-lab** Minikube cluster. Terraform (from [k8s-maklab-cluster](https://github.com/jomakori/devops_Terraform)) creates the top-level ArgoCD `Application` resources that point here; ArgoCD syncs automatically (prune + self-heal, exponential backoff retry).

## Structure

```
.
├── services/          ← 3rd-party infrastructure
│   ├── helm/          ← Helm charts (17 services)
│   └── argocd-appset/ ← App-of-Apps manifests
├── apps/              ← Application workloads
│   ├── helm/          ← Helm charts (notes-app, demo-app)
│   └── argocd-appset/ ← App-of-Apps manifests
├── .github/workflows/ ← PR lint workflow
├── .pre-commit-config.yaml
├── .ct-config.yml
├── renovate.json
└── devbox.json
```

### Services

| Service | Chart | Purpose |
|---------|-------|---------|
| [metrics-server](services/helm/metrics-server/) | metrics-server/metrics-server | Resource usage aggregation for HPA |
| [generic-device-plugin](services/helm/generic-device-plugin/) | custom | Device plugin for hardware resources |
| [kube-prometheus-stack](services/helm/kube-prometheus-stack/) | prometheus-community/kube-prometheus-stack | Cluster monitoring, metrics, and alerting |
| [istio-base](services/helm/istio-base/) | istio/base | Istio CRDs and cluster-scoped resources |
| [external-secrets](services/helm/external-secrets/) | external-secrets/external-secrets | Doppler secret injection via ESO |
| [istiod](services/helm/istiod/) | istio/istiod | Istio control plane — ambient mode, STRICT mTLS |
| [cert-manager](services/helm/cert-manager/) | jetstack/cert-manager | Automated TLS via Let's Encrypt + Cloudflare DNS-01 |
| [istio-ingressgateway](services/helm/istio-ingressgateway/) | istio/gateway | Shared ingress gateway for `*.maklab.net` |
| [istio-config](services/helm/istio-config/) | custom | Gateway, PeerAuthentication, cert-manager ClusterIssuer + Certificate |
| [external-dns](services/helm/external-dns/) | external-dns/external-dns | Automatic Cloudflare DNS records from Istio Gateway hosts |
| [keda](services/helm/keda/) | kedacore/keda | Event-driven autoscaling |
| [db-operator](services/helm/db-operator/) | db-operator/db-operator | Database lifecycle management (StackGres Postgres) |
| [mongodb](services/helm/mongodb/) | mongodb/mongodb | MongoDB document store |
| [opencost](services/helm/opencost/) | opencost/opencost | Cost monitoring and allocation |
| [headlamp](services/helm/headlamp/) | headlamp/headlamp | Kubernetes UI dashboard |
| [ramalama](services/helm/ramalama/) | custom | AI/ML model serving |
| [redis-operator](services/helm/redis-operator/) | ot-operator/redis-operator | Redis cluster management (disabled by default) |

Toggled on/off via `services/argocd-appset/values.yaml`.

### Apps

| App | Environments | Secrets |
|-----|-------------|---------|
| [notes-app](apps/helm/notes-app/) | staging + production | Doppler tokens per environment |
| [demo-app](apps/helm/demo-app/) | single | — |

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
| `svc_cloudflare` | istio-config, external-dns | `CF_API_TOKEN` |
| `svc_postgres` | db-operator (StackGres) | `PG_USER`, `PG_PW`, `PG_HOST` |
| `svc_mongodb` | MongoDB | `MONGODB_USER`, `MONGODB_PW`, `MONGODB_DB` |

New secrets are added in the Doppler dashboard — the ExternalSecret already pulls the entire config.

## Adding a Service or App

1. **Create the Helm chart** under `services/helm/<name>/` or `apps/helm/<name>/`.
2. **Add an ArgoCD Application manifest** in the corresponding `argocd-appset/templates/` dir.
3. **Register it** in the corresponding `argocd-appset/values.yaml` with an `enable: true/false` flag.
4. **Wire secrets** via ESO: add a `dopplerConfig` key in the values entry and create an ExternalSecret template using `dataFrom.extract`. No Terraform changes needed.
5. **Validate locally**:
   ```bash
   .useful-scripts/ct_check.sh services/helm/<name>
   ```
6. **PR and merge** — ArgoCD auto-syncs.

For apps, also add a namespace entry in `apps/argocd-appset/templates/namespaces.yaml`.

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
