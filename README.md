# gke_GitOps

ArgoCD App-of-Apps repository for the **jmak-lab** Minikube cluster. This repo is the source of truth for everything running inside the cluster — both 3rd-party services and my own application workloads.

## Architecture

The repo follows the [ArgoCD App-of-Apps pattern](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#app-of-apps). Two top-level ArgoCD `Application` resources, created by the [k8s-maklab-cluster](https://github.com/jomakori/devops_Terraform) Terraform config, point at the two directories below:

```
gke_GitOps
├── services/          ← 3rd-party infrastructure (managed dependencies)
│   ├── helm/          ← Helm charts for each service
│   └── argocd-appset/ ← ArgoCD Application manifests (App-of-Apps)
├── apps/              ← My own application workloads
│   ├── helm/          ← Helm charts for each app
│   └── argocd-appset/ ← ArgoCD Application manifests (App-of-Apps)
```

### Services — 3rd-Party Infrastructure

Pre-built, off-the-shelf tools the cluster depends on. Each service has its own Helm chart under `services/helm/` and an ArgoCD Application manifest under `services/argocd-appset/templates/`.

| Service | Chart | Purpose |
|---------|-------|---------|
| [istio-base](services/helm/istio-base/) | istio/base | Istio CRDs and cluster-scoped resources |
| [istiod](services/helm/istiod/) | istio/istiod | Istio control plane — ambient mode, STRICT mTLS |
| [istio-ingressgateway](services/helm/istio-ingressgateway/) | istio/gateway | Shared ingress gateway for `*.maklab.net` |
| [istio-config](services/helm/istio-config/) | custom | Gateway, PeerAuthentication, cert-manager ClusterIssuer + Certificate |
| [cert-manager](services/helm/cert-manager/) | jetstack/cert-manager | Automated TLS via Let's Encrypt + Cloudflare DNS-01 |
| [external-dns](services/helm/external-dns/) | external-dns/external-dns | Automatic Cloudflare DNS records from Istio Gateway hosts |
| [kube-prometheus-stack](services/helm/kube-prometheus-stack/) | prometheus-community/kube-prometheus-stack | Cluster monitoring, metrics, and alerting |
| [metrics-server](services/helm/metrics-server/) | metrics-server/metrics-server | Resource usage aggregation for HPA |
| [external-secrets](services/helm/external-secrets/) | external-secrets/external-secrets | Doppler secret injection into namespaces |
| [db-operator](services/helm/db-operator/) | db-operator/db-operator | Database lifecycle management (Postgres) |
| [mongodb](services/helm/mongodb/) | mongodb/mongodb | MongoDB document store |
| [keda](services/helm/keda/) | kedacore/keda | Event-driven autoscaling |
| [headlamp](services/helm/headlamp/) | headlamp/headlamp | Kubernetes UI dashboard |
| [opencost](services/helm/opencost/) | opencost/opencost | Cost monitoring and allocation |
| [redis-operator](services/helm/redis-operator/) | ot-operator/redis-operator | Redis cluster management (disabled by default) |
| [generic-device-plugin](services/helm/generic-device-plugin/) | custom | Device plugin for hardware resources |
| [ramalama](services/helm/ramalama/) | custom | AI/ML model serving |

Services are toggled on/off via `services/argocd-appset/values.yaml`.

### Apps — My Own Workloads

Applications I build and deploy.

| App | Environments | Secrets |
|-----|-------------|---------|
| [notes-app](apps/helm/notes-app/) | staging + production | Doppler tokens per environment |
| [demo-app](apps/helm/demo-app/) | single | — |

Apps are toggled via `apps/argocd-appset/values.yaml`.

## GitOps Flow

```
Terraform (k8s-maklab-cluster)
  │
  ├── kubectl_manifest.services    → creates "services" ArgoCD Application
  └── kubectl_manifest.apps        → creates "apps" ArgoCD Application
        │
        ├── services/argocd-appset/   (App-of-Apps)
        │     └── manages all service Helm releases
        │
        └── apps/argocd-appset/      (App-of-Apps)
              └── manages all app Helm releases
```

ArgoCD syncs automatically (prune + self-heal, exponential backoff retry, max 10 retries).

## Secrets Management

Secrets never live in this repo. The chain is:

1. **Doppler** stores the actual secret values
2. **Terraform** (`k8s-maklab-cluster`) pulls them via `TF_VAR_` environment variables and passes them as Helm parameters to the ArgoCD Application manifests
3. **ArgoCD** passes those parameters to the chart templates
4. **external-secrets** (or Doppler Operator) syncs secrets into individual namespaces

### Required Terraform Variables Passed to Services

| Parameter | Source | Used By |
|-----------|--------|---------|
| `grafanaCreds.admin` | Doppler (TF_VAR_) | Grafana admin login |
| `grafanaCreds.pw` | Doppler (TF_VAR_) | Grafana admin password |
| `dbOperator.creds.user` | Doppler (TF_VAR_) | Postgres operator |
| `mongoDBCreds.host` | Doppler (TF_VAR_) | MongoDB connection |
| `mongoDBCreds.user` | Doppler (TF_VAR_) | MongoDB auth |
| `mongoDBCreds.pw` | Doppler (TF_VAR_) | MongoDB auth |

### Required Terraform Variables Passed to Apps

| Parameter | Source | Used By |
|-----------|--------|---------|
| `notesApp.environment.staging.dopplerToken` | Doppler (TF_VAR_) | notes-app staging env |
| `notesApp.environment.production.dopplerToken` | Doppler (TF_VAR_) | notes-app production env |

## Tooling

### Devbox

A [devbox](https://www.jetify.com/devbox) shell provides reproducible tooling with `yq-go` and `git`. A `deploy` script automates image tag bumps:

```bash
GH_USER_EMAIL="..." GH_USER="..." ENV_NUM="0" CONTAINER_TAG="v1.2.3" GIT_MESSAGE="bump tag" devbox run deploy
```

### Renovate

[Renovate](renovate.json) runs every Tuesday, auto-updating Helm chart versions for services (`Chart.yaml` and `values.yaml` under `services/helm/`).

### Pre-commit Hooks

Configured via [`.pre-commit-config.yaml`](.pre-commit-config.yaml):

| Hook | Purpose |
|------|---------|
| `check-merge-conflict` | Blocks unresolved merge markers |
| `trailing-whitespace` | Trims trailing whitespace |
| `check-added-large-files` | Prevents giant file commits |
| `end-of-file-fixer` | Ensures files end with newline |
| `detect-secrets` | Blocks accidental secret commits |
| `yamllint` | Lints all YAML |
| `helm-docs` | Auto-generates Helm chart READMEs |

### Chart Testing (ct)

A [ct](https://github.com/helm/chart-testing) config at [`.ct-config.yml`](.ct-config.yml) uses `--dry-run --debug` for local validation.

```bash
.useful-scripts/ct_check.sh <path-to-helm-chart>
```

### GitHub Actions

A PR workflow at [`.github/workflows/pull_request.yaml`](.github/workflows/pull_request.yaml) runs lint/check on pull requests.

## Adding a New Service

1. **Create the Helm chart** under `services/helm/<service-name>/` with `Chart.yaml` and `values.yaml`
2. **Create the ArgoCD Application manifest** under `services/argocd-appset/templates/<service-name>.yaml`
3. **Register it** in `services/argocd-appset/values.yaml` with an `enable: true/false` flag
4. **Pass secrets** via Terraform if needed (add variable to `k8s-maklab-cluster`'s `3-gitops.tf` and `argocd_app-of-apps/services.yml`)
5. **Test locally**: `.useful-scripts/ct_check.sh services/helm/<service-name>`
6. **PR and merge** — ArgoCD auto-syncs

## Adding a New App

1. **Create the Helm chart** under `apps/helm/<app-name>/` with templates and values
2. **Create a namespace** in `apps/argocd-appset/templates/namespaces.yaml`
3. **Create the ArgoCD Application manifest** under `apps/argocd-appset/templates/<app-name>.yaml`
4. **Register it** in `apps/argocd-appset/values.yaml`
5. **Pass secrets** via Terraform (add doppler token to `k8s-maklab-cluster`'s `3-gitops.tf` and `argocd_app-of-apps/apps.yml`)
6. **Test locally**: `.useful-scripts/ct_check.sh apps/helm/<app-name>`
7. **PR and merge**

## Testing Helm Charts Locally

### Prerequisites

- kubectl
- helm
- yamllint
- ct ([chart-testing](https://github.com/helm/chart-testing))

```bash
.useful-scripts/ct_check.sh <path-to-helm-chart>
```

> Only compatible with macOS/Linux.

## Repository Structure

```
.
├── apps/
│   ├── argocd-appset/           ← App-of-Apps for my workloads
│   │   ├── Chart.yaml
│   │   ├── templates/
│   │   │   ├── namespaces.yaml
│   │   │   ├── demo-app.yaml
│   │   │   └── notes-app.yaml
│   │   └── values.yaml
│   ├── helm/
│   │   ├── demo-app/            ← Demo app Helm chart
│   │   └── notes-app/           ← Notes app Helm chart (staging + prod)
│   └── README.md
├── services/
│   ├── argocd-appset/           ← App-of-Apps for 3rd-party infra
│   │   ├── Chart.yaml
│   │   ├── templates/           ← 11 service Application manifests
│   │   └── values.yaml
│   ├── helm/                    ← 11 service Helm charts
│   └── README.md
├── .ct-config.yml               ← Chart testing config
├── .pre-commit-config.yaml      ← Pre-commit hook configuration
├── .github/
│   ├── pull_request_template.md
│   └── workflows/pull_request.yaml
├── devbox.json                  ← Devbox shell definition
├── renovate.json                ← Auto-update schedule for service charts
└── README.md
```

## Tips

### Scheduling on Specific Nodes

Use tolerations and node affinity in your Helm values:

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

### Exposing a Service

```bash
kubectl port-forward -n <namespace> svc/<service-name> 8080:80
```
