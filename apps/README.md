# Apps

This folder contains GitOps configuration for **1st-party applications** — my own workloads running on the jmak-lab Minikube cluster.

## Structure

Follows the ArgoCD [App-of-Apps](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#app-of-apps) pattern.

```
apps/
├── argocd-appset/          ← ArgoCD Application manifests
│   ├── Chart.yaml
│   ├── templates/
│   │   ├── _helpers.tpl
│   │   ├── applications.yaml  ← Single template, auto-generates per-app Applications
│   │   └── namespaces.yaml
│   └── values.yaml
└── helm/                   ← Single parameterized Helm chart for all apps
    ├── Chart.yaml
    ├── templates/
    │   ├── _helpers.tpl       ← 273-line define (app.manifests) — generates all resources
    │   └── app.yaml           ← Invokes _helpers.tpl manifest generation
    └── values.yaml
```

### argocd-appset

A single `applications.yaml` template auto-generates ArgoCD `Application` resources from `values.yaml`. Each app is registered by key (e.g., `demoApi`, `notesUi`) with its environment configs and dopplerConfig references. The AppSet passes `--set appName=<key>` to the single chart.

### helm

A single parameterized chart (`name: apps`) handles all application workloads. All manifests are generated from `_helpers.tpl` via one `app.yaml` invocation:

| Resource | Conditional On |
|----------|---------------|
| ServiceAccount + ECR dockercfg Secret | Always |
| ExternalSecret | `environments.<env>.dopplerConfig` set |
| Deployment | Always (`nodeSelector: intent: apps`) |
| Service | Always (ClusterIP for Istio; NodePort fallback) |
| VirtualService | `enable_domain` + `enable_istio` |
| HPA | `enable_scaling` |
| PVC | `storage.size` defined |

Supports multi-environment (staging + production) per app.

## Adding an App

1. **Add an entry** in `argocd-appset/values.yaml` with the app key, environments, and `dopplerConfig` per environment.
2. **Add a namespace** in `argocd-appset/templates/namespaces.yaml` (one per app).
3. **Set `enable: true`** — both existing apps (`demoApi`, `notesUi`) are currently disabled, ready for activation.
4. **PR and merge** — ArgoCD auto-syncs.
