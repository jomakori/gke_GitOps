# headlamp

![Version: 0.42.0](https://img.shields.io/badge/Version-0.42.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.42.0](https://img.shields.io/badge/AppVersion-0.42.0-informational?style=flat-square)
A Helm chart for the headlamp k8s dashboard

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://kubernetes-sigs.github.io/headlamp/ | headlamp | 0.42.0 |

## Under the hood

This chart deploys the [Headlamp](https://headlamp.dev) Kubernetes dashboard — a web UI for cluster management. It is a **hybrid chart** with an upstream dependency and local templates for user-facing service account creation.

### Hybrid chart structure

- **Upstream dependency**: `headlamp/headlamp` — the dashboard application with Kubescape security plugin pre-installed via init container.
- **Local templates**: `user-serviceaccount.yaml` (creates a user-facing `ServiceAccount`), `user-clusterrolebinding.yaml` (binds it to `cluster-admin`).

### User authentication

Headlamp does not require external secret injection. Instead, it ships with local templates that create a dedicated `ServiceAccount` (`headlamp-user` by default) with a `ClusterRoleBinding` to `cluster-admin`. To log in:

```bash
kubectl create token headlamp-user -n headlamp --duration=24h
```

Paste the token into the Headlamp login screen. No Doppler config or ExternalSecret is needed.

### Kubescape integration

An init container (`quay.io/kubescape/headlamp-plugin:latest`) injects the Kubescape security plugin into Headlamp's plugin directory at startup. This enables in-dashboard Kubernetes security scanning without separate configuration.

### Ingress and auth

The Headlamp VirtualService in the istio umbrella chart is **disabled by default** (`virtualServices.headlamp.enabled: false` in the istio values). Access Headlamp via port-forward or enable it by setting that value to `true` and updating the gateways section in the argocd-appset services configuration.

### Setup

| Aspect | Detail |
|--------|--------|
| **Namespace** | `headlamp` |
| **Sync wave** | Not explicitly set (deployed via service registration) |
| **Doppler config** | None — no secrets required |
| **User SA** | `headlamp-user` with `cluster-admin` ClusterRole (configurable via `userServiceAccount.name` / `userServiceAccount.clusterRole`) |
| **Plugins** | Kubescape security plugin pre-loaded |
| **Ingress** | Disabled by default — enable via istio `virtualServices.headlamp.enabled: true` and service gateways config |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| headlamp.clusterRoleBinding.clusterRoleName | string | `"cluster-admin"` |  |
| headlamp.clusterRoleBinding.create | bool | `true` |  |
| headlamp.initContainers[0].command[0] | string | `"/bin/sh"` |  |
| headlamp.initContainers[0].command[1] | string | `"-c"` |  |
| headlamp.initContainers[0].command[2] | string | `"mkdir -p /build/plugins && cp -r /plugins/* /build/plugins/"` |  |
| headlamp.initContainers[0].image | string | `"quay.io/kubescape/headlamp-plugin:latest"` |  |
| headlamp.initContainers[0].name | string | `"kubescape-plugin"` |  |
| headlamp.initContainers[0].volumeMounts[0].mountPath | string | `"/build/plugins"` |  |
| headlamp.initContainers[0].volumeMounts[0].name | string | `"headlamp-plugins"` |  |
| headlamp.volumeMounts[0].mountPath | string | `"/build/plugins"` |  |
| headlamp.volumeMounts[0].name | string | `"headlamp-plugins"` |  |
| headlamp.volumes[0].emptyDir | object | `{}` |  |
| headlamp.volumes[0].name | string | `"headlamp-plugins"` |  |
| userServiceAccount.clusterRole | string | `"cluster-admin"` |  |
| userServiceAccount.name | string | `"headlamp-user"` |  |
