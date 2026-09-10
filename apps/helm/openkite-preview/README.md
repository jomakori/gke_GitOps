# openkite-preview

Per-PR preview **workload** chart for [jomakori/openkite](https://github.com/jomakori/openkite).
Rendered once per open PR by the `openkite-preview` ApplicationSet
(`apps/openkite-preview`). It is not deployed directly.

## Injected values

The ApplicationSet passes per-PR Helm parameters:

| Parameter | Example | Purpose |
|-----------|---------|---------|
| `fullnameOverride` | `openkite-preview-42` | unique per-PR resource names |
| `image.repository` | `ghcr.io/jomakori/openkite` | preview image |
| `image.tag` | `pr-42` | PR image tag (OKT-74) |
| `ingress.host` | `pr-42.openkite.maklab.net` | preview host |
| `replicaCount` | `1` | replicas |

The chart deploys into the shared `openkite-preview` namespace (isolated by the
unique per-PR names), so the wildcard TLS Secret is issued exactly once and the
per-PR Ingress can reference it in-namespace.

## Resources

| Resource | Notes |
|----------|-------|
| `Deployment` | image `repository:tag`, `intent: apps` nodeSelector, readiness/liveness probes |
| `Service` | ClusterIP, port 80 → container port `service.targetPort` |
| `Ingress` | per-PR host + TLS `*.openkite.maklab.net` (`wildcard-openkite-maklab-net-tls`) |
| `VirtualService` | Istio routing (the cluster's ingress) to the shared `openkite-preview-gateway` |

## TLS

`*.maklab.net` does **not** match the two-level `pr-<N>.openkite.maklab.net` host,
so ingress TLS targets the dedicated `*.openkite.maklab.net` wildcard Secret
(issued once by `apps/openkite-preview/templates/certificate.yaml`).

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
