# Apps

This folder contains GitOps configuration for **1st-party applications** — my own workloads running on the jmak-lab Minikube cluster.

## Structure

Follows the ArgoCD [App-of-Apps](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#app-of-apps) pattern.

```
apps/
├── argocd-appset/          ← ArgoCD Application manifests
│   ├── Chart.yaml
│   ├── templates/
│   │   ├── namespaces.yaml
│   │   ├── demo-app.yaml
│   │   └── notes-app.yaml
│   └── values.yaml
└── helm/                   ← Helm chart source for each app
    ├── demo-app/
    └── notes-app/
```

### argocd-appset

ArgoCD Application manifests that tell ArgoCD where to find each app's Helm chart and what parameters to pass. Templates use values from `values.yaml` — some of which are injected by Terraform (Doppler tokens).

### helm

Each app has a standard Helm chart with template files (`.tpl` extension for Go-templated YAML):

```
helm/<app>/
├── Chart.yaml
├── values.yaml
└── templates/
    ├── deployment.tpl
    ├── doppler_secrets.tpl
    ├── hpa.tpl
    ├── ingress.tpl
    ├── service-account.tpl
    └── service.tpl
```

## Adding an App

1. Create a Helm chart under `helm/<app-name>/`
2. Add a namespace in `argocd-appset/templates/namespaces.yaml`
3. Create an Application manifest in `argocd-appset/templates/<app-name>.yaml`
4. Register it in `argocd-appset/values.yaml`
5. If it needs environment secrets, add Doppler tokens to the Terraform config in `k8s-maklab-cluster`
6. Test and PR
