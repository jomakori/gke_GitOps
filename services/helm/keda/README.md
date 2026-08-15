# keda

![Version: 2.19.0](https://img.shields.io/badge/Version-2.19.0-informational?style=flat-square) ![AppVersion: 2.19.0](https://img.shields.io/badge/AppVersion-2.19.0-informational?style=flat-square)
A Helm chart for Kubernetes Event-driven Autoscaling (KEDA)

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://kedacore.github.io/charts | keda | 2.19.0 |

## Under the hood

This chart deploys [KEDA](https://keda.sh/) (Kubernetes Event-driven Autoscaling) as a thin wrapper around the upstream `keda` chart. KEDA extends the Kubernetes HPA with event-driven scaling based on metrics from external sources (Kafka, RabbitMQ, Prometheus, HTTP, cron, etc.).

### Current status

**Disabled by default** (`enable: false` in the appset registry). Not yet used by any core service. Enable in `services/argocd-appset/values.yaml` when event-based autoscaling is needed. Sync wave 3, namespace `keda`.

### Integration points

KEDA installs as a metrics adapter that HPA objects can reference via `ScaledObject` custom resources. Core services currently use standard CPU/memory-based HPA (enabled via `enable_scaling: true` in app values). KEDA unlocks patterns like:

- Scale workloads based on queue depth (RabbitMQ, SQS, Kafka)
- Cron-based scaling for predictable load patterns
- Prometheus metric-driven autoscaling
- HTTP request rate-based scaling (KEDA HTTP add-on)

### Configuration

The chart enables the metrics server (required for ScaledObject to expose custom metrics to HPA) and webhooks (for validating ScaledObject/ScaledJob resources). A dedicated service account `keda` is created with RBAC for cluster-wide scaling operations.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| keda.metricsServer.enabled | bool | `true` |  |
| keda.serviceAccount.create | bool | `true` |  |
| keda.serviceAccount.name | string | `"keda"` |  |
| keda.webhooks.enabled | bool | `true` |  |
