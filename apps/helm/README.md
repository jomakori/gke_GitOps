# apps

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![AppVersion: 1.0](https://img.shields.io/badge/AppVersion-1.0-informational?style=flat-square)
Spec-driven Helm chart for all application workloads

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| local |  |  |

## Setup

This is a **single parameterized chart** that generates all Kubernetes manifests for every application workload. It is invoked from the apps ArgoCD AppSet with `--set appName=<key>`. The chart name is `apps`.

### Architecture

All manifests are produced by a single 373-line `_helpers.tpl` define (`app.manifests`). The `app.yaml` template looks up the app config via `index .Values <appName>` and delegates to `app.manifests`. There is one invocation per app per environment.

### Generated resources

| Resource | Condition |
|----------|-----------|
| `ServiceAccount` + ECR `dockercfg` Secret | Always |
| `ExternalSecret` | `environments.<env>.dopplerConfig` is set |
| `SGCluster` (StackGres) or `PerconaServerMongoDB` | `enable_db.type` is `postgres` or `mongodb` |
| `Deployment` | Always (scheduled on `intent: apps` nodes) |
| `Service` | Always (ClusterIP when Istio, NodePort otherwise) |
| `VirtualService` | `enable_domain` + `enable_istio` (both default true) |
| `HorizontalPodAutoscaler` | `enable_scaling.HPA` is set |
| `PersistentVolumeClaim` | `service.storage.size` is set |

### Database provisioning

When `enable_db` is set, the chart generates an operator CR in the app namespace:

| DB type | Operator CR | Autoscaling |
|---------|-------------|-------------|
| `postgres` | `SGCluster` (StackGres) | Connection-based horizontal (`replicasConnectionsUsageTarget: 0.5`) |
| `mongodb` | `PerconaServerMongoDB` (Percona) | Static replica set size (no connection-based scaling) |

- `deployment: db` → single instance (fixed)
- `deployment: cluster` → multiple instances with PDB

Sync wave for all DB resources: `-1` (before the app pod).

### Environments

Each app supports `staging` and `production` environments. Per-environment config:

| Field | Source | Default |
|-------|--------|---------|
| Subdomain | `environments.<env>.subdomain` | `<appName>` (prod) / `staging.<appName>` (staging) |
| Image tag | `environments.<env>.tag` | `latest` |
| Doppler config | `environments.<env>.dopplerConfig` | (none — ExternalSecret only created when set) |
| Namespace | Auto | `<appName-kebab>-<envName>` |

Staging can be disabled per-app via `enable_staging: false`.

### Ingress

When `enable_domain` + `enable_istio` are true, a `VirtualService` is generated:

- **Host**: `<subdomain>.<clusterDomain>` (e.g., `demo-api.maklab.net`)
- **Gateway**: `istio-system/maklab-gateway` (overridable via `istio.gateway`)
- **Retry**: 3 attempts, 5s per-try timeout, on gateway/connect/retriable errors
- **Timeout**: 30s

### Global config

| Parameter | Default | Description |
|-----------|---------|-------------|
| `clusterDomain` | `maklab.net` | Domain for all VirtualService hosts |
| `registry` | `123456.dkr.ecr.us-east-2.amazonaws.com` | ECR registry for app images |
| `storageClass` | `csi-hostpath-sc` | Default PVC storage class |

### Node scheduling

All app deployments use `nodeSelector: intent: apps` to land on application-dedicated nodes.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| clusterDomain | string | `"maklab.net"` |  |
| demoApi.enable_db.deployment | string | `"cluster"` |  |
| demoApi.enable_db.storage | string | `"10Gi"` |  |
| demoApi.enable_db.type | string | `"postgres"` |  |
| demoApi.enable_db.version | string | `"17"` |  |
| demoApi.enable_istio | bool | `true` |  |
| demoApi.enable_scaling.HPA.maxReplicas | int | `6` |  |
| demoApi.enable_scaling.HPA.minReplicas | int | `2` |  |
| demoApi.enable_scaling.db.maxInstances | int | `5` |  |
| demoApi.enable_scaling.db.minInstances | int | `2` |  |
| demoApi.enable_staging | bool | `true` |  |
| demoApi.environments.production.tag | float | `1.43` |  |
| demoApi.environments.staging.tag | float | `1.23` |  |
| demoApi.istio.gateway | string | `"istio-system/maklab-gateway"` |  |
| demoApi.istio.requestTimeout | string | `"30s"` |  |
| demoApi.istio.retryAttempts | int | `3` |  |
| demoApi.istio.retryTimeout | string | `"5s"` |  |
| demoApi.service.port | int | `3000` |  |
| demoApi.service.resourceRequests.cpu | string | `"256m"` |  |
| demoApi.service.resourceRequests.memory | string | `"256Mi"` |  |
| demoApi.service.storage.size | string | `"1Gi"` |  |
| notesUi.enable_istio | bool | `true` |  |
| notesUi.enable_scaling.HPA.maxReplicas | int | `6` |  |
| notesUi.enable_scaling.HPA.minReplicas | int | `1` |  |
| notesUi.enable_staging | bool | `true` |  |
| notesUi.environments.production.tag | string | `"1.0.1"` |  |
| notesUi.environments.staging.tag | string | `"pr-7"` |  |
| notesUi.istio.gateway | string | `"istio-system/maklab-gateway"` |  |
| notesUi.istio.requestTimeout | string | `"30s"` |  |
| notesUi.istio.retryAttempts | int | `3` |  |
| notesUi.istio.retryTimeout | string | `"5s"` |  |
| notesUi.service.port | int | `8080` |  |
| notesUi.service.resourceRequests.cpu | string | `"100m"` |  |
| notesUi.service.resourceRequests.memory | string | `"256Mi"` |  |
| notesUi.service.storage.size | string | `"1Gi"` |  |
| registry | string | `"123456.dkr.ecr.us-east-2.amazonaws.com"` |  |
| storageClass | string | `"csi-hostpath-sc"` |  |
