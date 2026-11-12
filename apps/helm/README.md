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
| `ClusterRole` + `ClusterRoleBinding` | `clusterRead` is set (never in previews) |
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

- `deployment: db` → single instance (fixed)
- `deployment: cluster` → multiple instances with PDB

Sync wave for all DB resources: `-1` (before the app pod).

### Environments

Each app supports `staging` and `production` environments. Per-environment config:

| Field | Source | Default |
|-------|--------|---------|
| Subdomain | `environments.<env>.subdomain` | `<appName>` (prod) / `staging-<appName>` (staging) |
| Image tag | `environments.<env>.tag` | `latest` |
| Doppler config | `environments.<env>.dopplerConfig` | (none — ExternalSecret only created when set) |
| Namespace | Auto | `<appName-kebab>-<envName>` |

Each environment is controlled per app. `enable_staging` and `enable_production`
default to on; an Application that renders one environment turns the other off, so a
shared environment always has exactly one owner.

### Ingress

When `enable_domain` + `enable_istio` are true, a `VirtualService` is generated:

- **Host**: `<subdomain>.<clusterDomain>` (e.g., `demo-api.maklab.net`)
- **Gateway**: `istio-system/maklab-gateway` (overridable via `istio.gateway`)
- **Retry**: 3 attempts, 5s per-try timeout, on gateway/connect/retriable errors
- **Timeout**: 30s

### Cluster access

An app that reads the cluster (a console) sets `clusterRead: true`. The chart then renders a
`ClusterRole`/`ClusterRoleBinding` pair named `<namespace>-read`, bound to the app's own
`<namespace>-sa` — never a wildcard subject, and never a credential from a Secret. Rules:
cluster-wide `get`/`list`/`watch` on the kinds a console watches and reads (`get` covers the
bridge's single-object `get_resource`, `list`/`watch` the reflectors), `get`/`list`/`watch` on
`secrets` (a secrets list returns values; recorded and accepted in OKT-159 as strictly smaller
than the cluster-admin grant the removed kubeconfig reached) and `get` on `pods/log`. The
Deployment of a `clusterRead` host also sets `automountServiceAccountToken: true`: the identity
is the pod's own projected ServiceAccount token (the OKT-130 kubeconfig mount was removed —
it carried a Tailscale identity bound to cluster-admin, not a scoped credential), stated
explicitly rather than inherited from the default. It is never rendered for a preview: previews
run unreviewed PR code (see `clusterReadOn` in `templates/_helpers.tpl`). See OKT-178.

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
| openkite.clusterAdmin | bool | `true` |  |
| openkite.enable_istio | bool | `true` |  |
| openkite.enable_staging | bool | `true` |  |
| openkite.environments.production.tag | string | `"v0.70.0"` |  |
| openkite.environments.staging.subdomain | string | `"staging-openkite"` |  |
| openkite.environments.staging.tag | string | `"v0.52.0"` |  |
| openkite.image.repository | string | `"ghcr.io/jomakori/openkite"` |  |
| openkite.istio.gateway | string | `"istio-system/maklab-gateway"` |  |
| openkite.istio.requestTimeout | string | `"30s"` |  |
| openkite.istio.retryAttempts | int | `3` |  |
| openkite.istio.retryTimeout | string | `"5s"` |  |
| openkite.service.port | int | `8080` |  |
| openkite.service.resourceLimits.memory | string | `"640Mi"` |  |
| openkite.service.resourceRequests.cpu | string | `"100m"` |  |
| openkite.service.resourceRequests.memory | string | `"320Mi"` |  |
| registry | string | `"123456.dkr.ecr.us-east-2.amazonaws.com"` |  |
| storageClass | string | `"local-path"` |  |
| tickerlight.enable_istio | bool | `true` |  |
| tickerlight.enable_staging | bool | `true` |  |
| tickerlight.environments.production.subdomain | string | `"trade"` |  |
| tickerlight.environments.production.tag | string | `"v0.2.0"` |  |
| tickerlight.environments.staging.subdomain | string | `"staging-trade"` |  |
| tickerlight.environments.staging.tag | string | `"v0.2.0"` |  |
| tickerlight.image.repository | string | `"ghcr.io/jomakori/tickerlight"` |  |
| tickerlight.service.args[0] | string | `"-addr"` |  |
| tickerlight.service.args[1] | string | `"0.0.0.0:8080"` |  |
| tickerlight.service.args[2] | string | `"-journal"` |  |
| tickerlight.service.args[3] | string | `"/data/journal.jsonl"` |  |
| tickerlight.service.args[4] | string | `"-judge-enabled"` |  |
| tickerlight.service.containerSecurityContext.allowPrivilegeEscalation | bool | `false` |  |
| tickerlight.service.containerSecurityContext.capabilities.drop[0] | string | `"ALL"` |  |
| tickerlight.service.envFrom | bool | `false` |  |
| tickerlight.service.extraContainers[0].args[0] | string | `"-data-dir"` |  |
| tickerlight.service.extraContainers[0].args[1] | string | `"/data"` |  |
| tickerlight.service.extraContainers[0].envFromVars | bool | `true` |  |
| tickerlight.service.extraContainers[0].env[0].name | string | `"TRADING_MCP_HTTP_ADDR"` |  |
| tickerlight.service.extraContainers[0].env[0].value | string | `"127.0.0.1:8090"` |  |
| tickerlight.service.extraContainers[0].image | string | `"ghcr.io/jomakori/trading-mcp:v0.3.4"` |  |
| tickerlight.service.extraContainers[0].name | string | `"engine"` |  |
| tickerlight.service.extraContainers[0].ports[0].containerPort | int | `8090` |  |
| tickerlight.service.extraContainers[0].ports[0].name | string | `"mcp"` |  |
| tickerlight.service.extraContainers[0].resources.limits.memory | string | `"384Mi"` |  |
| tickerlight.service.extraContainers[0].resources.requests.cpu | string | `"50m"` |  |
| tickerlight.service.extraContainers[0].resources.requests.memory | string | `"128Mi"` |  |
| tickerlight.service.podSecurityContext.fsGroup | int | `65532` |  |
| tickerlight.service.podSecurityContext.runAsGroup | int | `65532` |  |
| tickerlight.service.podSecurityContext.runAsNonRoot | bool | `true` |  |
| tickerlight.service.podSecurityContext.runAsUser | int | `65532` |  |
| tickerlight.service.port | int | `8080` |  |
| tickerlight.service.resourceLimits.memory | string | `"256Mi"` |  |
| tickerlight.service.resourceRequests.cpu | string | `"50m"` |  |
| tickerlight.service.resourceRequests.memory | string | `"128Mi"` |  |
| tickerlight.service.storage.readOnly | bool | `true` |  |
| tickerlight.service.storage.size | string | `"1Gi"` |  |
