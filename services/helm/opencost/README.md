# opencost

![Version: 2.5.20](https://img.shields.io/badge/Version-2.5.20-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.120.2](https://img.shields.io/badge/AppVersion-1.120.2-informational?style=flat-square)
A Helm chart for OpenCost (open-source KubeCost)

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| local |  |  |

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://opencost.github.io/opencost-helm-chart | opencost | 2.5.20 |

## Under the hood

This chart is a thin wrapper around the upstream [opencost/opencost-helm-chart](https://github.com/opencost/opencost-helm-chart). It deploys [OpenCost](https://www.opencost.io/) — the open-source Kubernetes cost monitoring tool (the engine behind KubeCost).

### Sync wave and namespace

- **Sync wave**: 4
- **Namespace**: `opencost`

### Prometheus scraping

OpenCost is configured to scrape the internal Prometheus instance in the `kube-prometheus-stack` namespace:

| Parameter | Value |
|-----------|-------|
| Service name | `kube-prometheus-stack-prometheus` |
| Port | 15017 |
| Scheme | `http` |

No additional secret or Doppler config is needed — OpenCost reads Prometheus metrics directly.

### Ingress

Public ingress is **disabled by default** (`enable_public: false` in the services appset). OpenCost is accessed via `kubectl port-forward` for ad-hoc cost analysis. No VirtualService is auto-generated.

### Node scheduling

OpenCost pods are scheduled on `intent: apps` nodes. No tolerations or dedicated affinity are configured.

### UI

The OpenCost UI is enabled (`opencost.ui.enabled: true`). It binds to port 9003 inside the cluster and is reachable via port-forward or kubectl proxy.

### Features

- KSM v1 metrics only (`EMIT_KSM_V1_METRICS_ONLY: true`)
- Kube state metrics v1 emit enabled
- ServiceMonitor is **disabled** (no additional scrape targets needed — OpenCost queries Prometheus directly)
- Default cluster ID is empty (uses auto-detected cluster name)

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| opencost.clusterName | string | `"cluster.local"` |  |
| opencost.opencost.exporter.defaultClusterId | string | `""` |  |
| opencost.opencost.exporter.extraEnv.EMIT_KSM_V1_METRICS | string | `"false"` |  |
| opencost.opencost.exporter.extraEnv.EMIT_KSM_V1_METRICS_ONLY | string | `"true"` |  |
| opencost.opencost.exporter.extraEnv.LOG_LEVEL | string | `"warn"` |  |
| opencost.opencost.exporter.resources.requests.cpu | string | `"10m"` |  |
| opencost.opencost.exporter.resources.requests.memory | string | `"55Mi"` |  |
| opencost.opencost.metrics.kubeStateMetrics.emitKsmV1MetricsOnly | bool | `true` |  |
| opencost.opencost.metrics.serviceMonitor.enabled | bool | `false` |  |
| opencost.opencost.nodeSelector.intent | string | `"apps"` |  |
| opencost.opencost.prometheus.internal.enabled | bool | `true` |  |
| opencost.opencost.prometheus.internal.namespaceName | string | `"kube-prometheus-stack"` |  |
| opencost.opencost.prometheus.internal.port | int | `15017` |  |
| opencost.opencost.prometheus.internal.scheme | string | `"http"` |  |
| opencost.opencost.prometheus.internal.serviceName | string | `"kube-prometheus-stack-prometheus"` |  |
| opencost.opencost.prometheus.labels."app.kubernetes.io/name" | string | `"prometheus"` |  |
| opencost.opencost.prometheus.namespace | string | `"kube-prometheus-stack"` |  |
| opencost.opencost.prometheus.port | int | `15017` |  |
| opencost.opencost.ui.enabled | bool | `true` |  |
