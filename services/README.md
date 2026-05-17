# Services

This folder contains GitOps configuration for **3rd-party infrastructure services** that the jmak-lab Minikube cluster depends on.

## Structure

Follows the ArgoCD [App-of-Apps](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#app-of-apps) pattern.

```
services/
├── argocd-appset/          ← ArgoCD Application manifests
│   ├── Chart.yaml
│   ├── templates/           ← One Application per service (11 total)
│   └── values.yaml
└── helm/                   ← Helm chart source for each service
    ├── db-operator/
    ├── external-secrets/
    ├── generic-device-plugin/
    ├── headlamp/
    ├── keda/
    ├── kube-prometheus-stack/
    ├── metrics-server/
    ├── mongodb/
    ├── opencost/
    ├── ramalama/
    └── redis-operator/
```

### argocd-appset

Each template defines an ArgoCD `Application` resource pointing at the corresponding Helm chart in `services/helm/`. Services are toggled on/off via `values.yaml` — some values (Grafana creds, DB creds) are injected by Terraform at apply time.

### helm

Each service is a minimal Helm chart containing only `Chart.yaml` and `values.yaml` (templates come from the upstream chart).

## Adding a Service

1. Add the Helm chart under `helm/<service-name>/` with upstream repo/dependency
2. Create an Application manifest in `argocd-appset/templates/<service-name>.yaml`
3. Register and configure it in `argocd-appset/values.yaml`
4. If it needs secrets, add Terraform variables to `k8s-maklab-cluster` and pass them through `argocd_app-of-apps/services.yml`
5. Test and PR
