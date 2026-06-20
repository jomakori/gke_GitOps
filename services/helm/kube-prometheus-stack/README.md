# kube-prometheus-stack

![Version: 85.0.3](https://img.shields.io/badge/Version-85.0.3-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.90.1](https://img.shields.io/badge/AppVersion-v0.90.1-informational?style=flat-square)
A Helm chart for kube-prometheus-stack

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://prometheus-community.github.io/helm-charts | kube-prometheus-stack | 85.0.3 |

## Under the hood

This chart is a thin wrapper around the upstream [prometheus-community/kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack) chart. It deploys a complete monitoring stack: Prometheus, Alertmanager, and Grafana.

### Sync wave and namespace

- **Sync wave**: 4 — ensures `external-secrets` ClusterSecretStores exist before the Grafana ExternalSecret syncs.
- **Namespace**: `kube-prometheus-stack`

### Doppler config

A local `ExternalSecret` (sync-wave: `-1`) pulls the `svc_grafana` Doppler config and creates a `grafana-admin-credentials` Secret. The upstream Grafana chart is configured to use this existing secret:

| Secret Key | Source |
|------------|--------|
| `ADMIN_USER` | Doppler `GRAFANA_ADMIN` |
| `ADMIN_PASSWORD` | Doppler `GRAFANA_PW` |

### Ingress

Grafana is exposed at `grafana.maklab.net` via the istio umbrella VirtualService (auto-generated from `enable_public: true` in the services appset).

### Disabled components

The following upstream components are disabled — they are not reachable on Minikube:

- `kubeEtcd`
- `kubeControllerManager`
- `kubeSchedulerAlerting` / `kubeSchedulerRecording`
- Prometheus operator admission webhooks (no cert-manager dependency)
- Prometheus operator TLS

### Ports

| Service | Port |
|---------|------|
| Prometheus | 15017 |
| Alertmanager | 15010 |

### Storage

Prometheus TSDB uses a 30Gi PVC via the `local-path` storage class. Alertmanager and Grafana use the upstream defaults (emptyDir — no persistent storage configured).

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| dopplerConfig | string | `"svc_grafana"` |  |
| kube-prometheus-stack.alertmanager.service.port | int | `15010` |  |
| kube-prometheus-stack.defaultRules.create | bool | `true` |  |
| kube-prometheus-stack.defaultRules.rules.etcd | bool | `false` |  |
| kube-prometheus-stack.defaultRules.rules.kubeScheduler | bool | `false` |  |
| kube-prometheus-stack.grafana.admin.existingSecret | string | `"grafana-admin-credentials"` |  |
| kube-prometheus-stack.grafana.admin.passwordKey | string | `"ADMIN_PASSWORD"` |  |
| kube-prometheus-stack.grafana.admin.userKey | string | `"ADMIN_USER"` |  |
| kube-prometheus-stack.grafana.adminPassword | string | `"changeme"` |  |
| kube-prometheus-stack.grafana.adminUser | string | `"admin"` |  |
| kube-prometheus-stack.grafana.enabled | bool | `true` |  |
| kube-prometheus-stack.kubeControllerManager.enabled | bool | `false` |  |
| kube-prometheus-stack.kubeEtcd.enabled | bool | `false` |  |
| kube-prometheus-stack.kubeSchedulerAlerting.enabled | bool | `false` |  |
| kube-prometheus-stack.kubeSchedulerRecording.enabled | bool | `false` |  |
| kube-prometheus-stack.prometheus.kubelet.serviceMonitor.metricRelabelings[0].action | string | `"replace"` |  |
| kube-prometheus-stack.prometheus.kubelet.serviceMonitor.metricRelabelings[0].sourceLabels[0] | string | `"node"` |  |
| kube-prometheus-stack.prometheus.kubelet.serviceMonitor.metricRelabelings[0].targetLabel | string | `"instance"` |  |
| kube-prometheus-stack.prometheus.kubelet.serviceMonitor.relabelings[0].action | string | `"replace"` |  |
| kube-prometheus-stack.prometheus.kubelet.serviceMonitor.relabelings[0].regex | string | `"(.*)"` |  |
| kube-prometheus-stack.prometheus.kubelet.serviceMonitor.relabelings[0].replacement | string | `"$1"` |  |
| kube-prometheus-stack.prometheus.kubelet.serviceMonitor.relabelings[0].sourceLabels[0] | string | `"__meta_kubernetes_pod_node_name"` |  |
| kube-prometheus-stack.prometheus.kubelet.serviceMonitor.relabelings[0].targetLabel | string | `"kubernetes_node"` |  |
| kube-prometheus-stack.prometheus.prometheusSpec.storageSpec.volumeClaimTemplate.spec.accessModes[0] | string | `"ReadWriteOnce"` |  |
| kube-prometheus-stack.prometheus.prometheusSpec.storageSpec.volumeClaimTemplate.spec.resources.requests.storage | string | `"30Gi"` |  |
| kube-prometheus-stack.prometheus.prometheusSpec.storageSpec.volumeClaimTemplate.spec.storageClassName | string | `"local-path"` |  |
| kube-prometheus-stack.prometheus.service.port | int | `15017` |  |
| kube-prometheus-stack.prometheusOperator.admissionWebhooks.enabled | bool | `false` |  |
| kube-prometheus-stack.prometheusOperator.enabled | bool | `true` |  |
| kube-prometheus-stack.prometheusOperator.tls.enabled | bool | `false` |  |
