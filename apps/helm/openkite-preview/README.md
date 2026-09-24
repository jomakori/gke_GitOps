# openkite-preview

Per-PR preview **workload** chart for [jomakori/openkite](https://github.com/jomakori/openkite).
Rendered once per eligible (image-gated, see `apps/openkite-preview`) PR by the
`openkite-preview` ApplicationSet. It is not deployed directly.

## Injected values

The ApplicationSet passes per-PR Helm parameters:

| Parameter | Example | Purpose |
|-----------|---------|---------|
| `fullnameOverride` | `openkite-preview-42` | unique per-PR resource names |
| `image.repository` | `ghcr.io/jomakori/openkite` | preview image |
| `image.tag` | `pr-42` | PR image tag (OKT-74) |
| `ingress.host` | `pr-42.openkite.maklab.net` | preview host |
| `replicaCount` | `1` | replicas |
| `namespace.create` | `true` (per-PR) | render the per-PR Namespace |

## Resources

| Resource | Notes |
|----------|-------|
| `Namespace` | only when `namespace.create`; per-PR, `istio.io/dataplane-mode: ambient`, sync-wave `-1` |
| `Deployment` | image `repository:tag`, `intent: apps` nodeSelector, readiness/liveness probes |
| `Service` | ClusterIP, port 80 → container port `service.targetPort` |
| `VirtualService` | Istio routing (the cluster's ingress) to the shared `openkite-preview-gateway` |
| `Ingress` | only when `ingress.enabled`; a release in a namespace holding the wildcard TLS Secret |

The ApplicationSet sets `namespace.create=true`, so each preview owns its
namespace and closing the PR prunes the namespace with the Deployment, Service and
VirtualService. A release that lands in a namespace another Application owns (the
shared preview namespace) leaves `namespace.create=false` and adopts nothing.

## Per-PR namespace

One namespace per PR, not one shared namespace with per-PR resource names: the
namespace is what makes teardown complete. It is declared in the chart rather than
delegated to ArgoCD's `CreateNamespace=true` sync option, because CreateNamespace
creates a namespace ArgoCD does **not** track — deleting the Application then
prunes the namespaced resources and strands the namespace. Verified on the live
cluster: a throwaway Application with `CreateNamespace=true` and the
`resources-finalizer` left its namespace `Active` after the Application was gone.

## TLS

`*.maklab.net` does **not** match the two-level `pr-<N>.openkite.maklab.net` host,
so TLS comes from the dedicated `*.openkite.maklab.net` wildcard Secret issued once
by `apps/openkite-preview/templates/certificate.yaml` into the shared preview
namespace, where the shared `openkite-preview-gateway` presents it via
`credentialName`. The Gateway terminates TLS; a per-PR `VirtualService` only binds
its host to that Gateway.

A per-PR `Ingress` is therefore **not** rendered (`ingress.enabled=false` from the
ApplicationSet): an Ingress's TLS Secret has to live in the Ingress's own
namespace, and the wildcard Secret deliberately exists only in the shared one.

## Values

| Key | Default | Description |
|-----|---------|-------------|
| `nameOverride` | `""` | override chart name segment |
| `fullnameOverride` | `""` | override full resource name (per-PR) |
| `replicaCount` | `1` | Deployment replicas |
| `image.repository` | `ghcr.io/jomakori/openkite` | image repository |
| `image.tag` | `latest` | image tag (per-PR `pr-<N>`) |
| `image.pullPolicy` | `IfNotPresent` | image pull policy |
| `imagePullSecrets` | `[]` | image pull secrets |
| `namespace.create` | `false` | render the Namespace this release deploys into (per-PR `true`) |
| `service.type` | `ClusterIP` | Service type |
| `service.port` | `80` | Service port |
| `service.targetPort` | `8080` | container port |
| `ingress.enabled` | `true` | render the Ingress |
| `ingress.host` | `openkite.maklab.net` | per-PR host |
| `ingress.className` | `""` | ingress class |
| `ingress.path` / `ingress.pathType` | `/` / `Prefix` | path rule |
| `ingress.annotations` | `{}` | Ingress annotations |
| `ingress.tls` | wildcard entry | TLS hosts + Secret |
| `istio.enabled` | `true` | render the VirtualService |
| `istio.gateway` | `openkite-preview/openkite-preview-gateway` | shared preview Gateway |
| `istio.timeout` / `retryAttempts` / `retryTimeout` | `30s` / `3` / `5s` | routing policy |
| `resources` | requests/limits | container resources |
| `nodeSelector` | `intent: apps` | node placement |
| `podAnnotations` / `podLabels` | `{}` | pod metadata |

## Validate

```bash
helm template apps/helm/openkite-preview \
  --set fullnameOverride=openkite-preview-42 \
  --set image.tag=pr-42 \
  --set ingress.host=pr-42.openkite.maklab.net
helm unittest apps/helm/openkite-preview
```
