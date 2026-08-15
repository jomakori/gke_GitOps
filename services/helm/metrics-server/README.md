# metrics-server

![Version: 3.13.0](https://img.shields.io/badge/Version-3.13.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.8.0](https://img.shields.io/badge/AppVersion-0.8.0-informational?style=flat-square)
A Helm chart for Metrics Server

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://kubernetes-sigs.github.io/metrics-server | metrics-server | 3.13.0 |

## Under the hood

This chart deploys [Metrics Server](https://github.com/kubernetes-sigs/metrics-server) as a thin wrapper around the upstream `metrics-server` chart. Metrics Server aggregates resource usage (CPU/memory) from kubelets and exposes them through the Kubernetes Metrics API — required for HPA to function.

### Sync wave and positioning

Runs at **sync wave 0** — the first service deployed alongside cert-manager. No dependencies on other services. Deploys to `kube-system` namespace with `Replace=true` sync option (allows full replacement on upgrades).

### Minikube configuration

The metrics server runs with `--kubelet-insecure-tls` to skip kubelet certificate verification — required for Minikube where kubelet certificates are self-signed and not part of the cluster CA. The container port is explicitly set to `10250` to avoid the issues documented in [kubernetes-sigs/metrics-server#1064](https://github.com/kubernetes-sigs/metrics-server/issues/1064).

### High availability

Three replicas with a PodDisruptionBudget allowing at most 1 unavailable — ensures the Metrics API remains available during node maintenance or rolling updates. Resource requests are 200m CPU / 300Mi memory per replica.

### No Doppler config

Metrics Server has no ExternalSecret — it does not need any secrets or external configuration.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| metrics-server.args[0] | string | `"--kubelet-insecure-tls"` |  |
| metrics-server.containerPort | int | `10250` |  |
| metrics-server.podDisruptionBudget.enabled | bool | `true` |  |
| metrics-server.podDisruptionBudget.maxUnavailable | int | `1` |  |
| metrics-server.replicas | int | `3` |  |
| metrics-server.resources.limits.memory | string | `"500Mi"` |  |
| metrics-server.resources.requests.cpu | string | `"200m"` |  |
| metrics-server.resources.requests.memory | string | `"300Mi"` |  |
