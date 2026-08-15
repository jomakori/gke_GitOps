# redis-operator

![Version: 13.0.4](https://img.shields.io/badge/Version-13.0.4-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 7.4.1](https://img.shields.io/badge/AppVersion-7.4.1-informational?style=flat-square)
A Helm chart for deploying redis-cluster using a Redis operator

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://charts.bitnami.com/bitnami | redis-cluster | 13.0.4 |

## Under the hood

This chart deploys a [Redis Cluster](https://github.com/bitnami/charts/tree/main/bitnami/redis-cluster) using the Bitnami `redis-cluster` chart as a dependency. It is a thin wrapper with no local templates.

### Current status

**Disabled by default** (`enable: false` in the appset registry). Enable in `services/argocd-appset/values.yaml` when Redis-backed workloads are ready. Sync wave 4, namespace `redis-operator`.

### Cluster topology

The chart provisions a 6-node Redis cluster (3 master shards × 1 replica each). All nodes schedule on `intent: apps` labelled nodes with resource requests of 512m CPU / 1Gi memory each.

### Persistence

Each Redis node gets a dedicated PVC. The storage class is parameterized — set via `redis-cluster.persistence.storageClass` from the ArgoCD Application parameters, defaulting to the cluster-wide `storageClass` value:

Set `redis-cluster.persistence.storageClass` via ArgoCD Application parameters if the cluster-wide default storage class is not desired.

Volume size defaults to 50Gi per node (6 × 50Gi = 300Gi total cluster capacity).

### Authentication

Authentication is disabled (`global.redis.password: ""`) — the cluster is accessible only within the cluster network. No Doppler config or ExternalSecret is needed.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| redis-cluster.cluster.nodes | int | `6` |  |
| redis-cluster.global.redis.password | string | `""` |  |
| redis-cluster.persistence.size | string | `"50Gi"` |  |
| redis-cluster.persistence.storageClass | string | `nil` |  |
| redis-cluster.redis.nodeSelector.intent | string | `"apps"` |  |
| redis-cluster.redis.resources.requests.cpu | string | `"512m"` |  |
| redis-cluster.redis.resources.requests.memory | string | `"1Gi"` |  |
| redis-cluster.redis.resourcesPreset | string | `"none"` |  |
| redis-cluster.updateJob.nodeSelector.intent | string | `"apps"` |  |
