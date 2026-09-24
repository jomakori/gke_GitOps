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
| `ingress.host` | `pr42-openkite.maklab.net` | preview host |
| `replicaCount` | `1` | replicas |
| `namespace.create` | `true` (per-PR) | render the per-PR Namespace |

## Resources

| Resource | Notes |
|----------|-------|
| `Namespace` | only when `namespace.create`; per-PR, `istio.io/dataplane-mode: ambient`, sync-wave `-1` |
| `Deployment` | image `repository:tag`, `intent: apps` nodeSelector, readiness/liveness probes |
| `Service` | ClusterIP, port 80 → container port `service.targetPort` |
| `VirtualService` | Istio routing (the cluster's ingress) to `istio-system/maklab-gateway` |
| `AuthorizationPolicy` | Cloudflare Access DENY rule for this preview's host, in `istio-system` next to the gateway it selects |
| `Ingress` | only when `ingress.enabled`; a release in a namespace holding the wildcard TLS Secret |

The ApplicationSet sets `namespace.create=true`, so each preview owns its
namespace and closing the PR prunes the namespace with the Deployment, Service,
VirtualService and AuthorizationPolicy. A release that lands in a namespace another
Application owns (the shared preview namespace) leaves `namespace.create=false` and
adopts nothing.

## Per-PR namespace

One namespace per PR, not one shared namespace with per-PR resource names: the
namespace is what makes teardown complete. It is declared in the chart rather than
delegated to ArgoCD's `CreateNamespace=true` sync option, because CreateNamespace
creates a namespace ArgoCD does **not** track — deleting the Application then
prunes the namespaced resources and strands the namespace. Verified on the live
cluster: a throwaway Application with `CreateNamespace=true` and the
`resources-finalizer` left its namespace `Active` after the Application was gone.

## TLS

TLS is terminated once, by the cluster's own gateway. Previews use a **one-label**
host (`pr42-openkite.maklab.net`) precisely so the existing
`istio-system/maklab-gateway` (hosts `*.maklab.net`) and its cert-manager
`wildcard-maklab-net-tls` Certificate cover them: this chart declares no Gateway,
no Certificate and no TLS Secret. A per-PR `VirtualService` only binds its host to
that gateway.

That single label is load-bearing. Universal SSL covers the zone apex plus one
subdomain level and Total TLS skips Tunnel hostnames, so a two-level host
(`pr-42.openkite.maklab.net`) is served no certificate at the Edge at all — the
handshake fails before the Access gate or the mesh policy sees anything.

A per-PR `Ingress` is **not** rendered (`ingress.enabled=false` from the
ApplicationSet). Two reasons, both structural: an Ingress's TLS Secret must live in
the Ingress's own namespace, and the wildcard Secret exists only in `istio-system`;
and a plain Ingress would reach the pod without passing the Cloudflare Access
challenge or the mesh DENY policy, i.e. it would serve the preview unauthenticated.

## The Cloudflare Access gate

`templates/authorizationpolicy.yaml` renders a DENY-except-valid-JWT policy for the
preview host, with the same rule the cluster's static private hosts use. It is
rendered per preview rather than added to the istio chart's `virtualServices`
entries because the host's route is not declared there, and it cannot be a wildcard
because `pr42-openkite.maklab.net` is one label under the zone — the only suffix it
shares with other hosts is the whole zone, and Istio matches policy hosts as exact
or leading-`*.` suffix only.

It lives in `istio-system` (`istio.policyNamespace`) because an AuthorizationPolicy
binds to the pods in its own namespace, and the ingress gateway pods are there. The
ApplicationSet's Applications run under the `default` AppProject, which permits any
destination namespace.

The matching Cloudflare side is the Access application in `devops_Terraform`
(`6-cloudflare-access.tf`, domain `pr*-openkite.maklab.net`). Both halves are
required, and so is the audience handshake: the application's AUD must reach the
istio chart's `cloudflare.access.audienceTag` (Doppler `CF_ACCESS_AUDS`), or the
JWT a browser obtains fails validation and this policy denies it.

## Values

| Key | Default | Description |
|-----|---------|-------------|
| `nameOverride` | `""` | override chart name segment |
| `fullnameOverride` | `""` | override full resource name (per-PR) |
| `replicaCount` | `1` | Deployment replicas |
| `image.repository` | `ghcr.io/jomakori/openkite` | image repository |
| `image.tag` | `latest` | image tag (per-PR `pr-<N>`) |
| `image.pullPolicy` | `Always` | image pull policy |
| `imagePullSecrets` | `[]` | image pull secrets |
| `namespace.create` | `false` | render the Namespace this release deploys into (per-PR `true`) |
| `service.type` | `ClusterIP` | Service type |
| `service.port` | `80` | Service port |
| `service.targetPort` | `8080` | container port |
| `ingress.enabled` | `true` | render the Ingress |
| `ingress.host` | `pr1-openkite.maklab.net` | per-PR host (one label under the zone) |
| `ingress.className` | `""` | ingress class |
| `ingress.path` / `ingress.pathType` | `/` / `Prefix` | path rule |
| `ingress.annotations` | `{}` | Ingress annotations |
| `ingress.tls` | `[]` | empty: the cluster gateway holds the TLS Secret |
| `istio.enabled` | `true` | render the VirtualService and the Access policy |
| `istio.gateway` | `istio-system/maklab-gateway` | cluster gateway serving `*.maklab.net` |
| `istio.policyNamespace` | `istio-system` | namespace the DENY policy is rendered into |
| `istio.gatewaySelector` | `istio: ingressgateway` | pods the DENY policy selects |
| `istio.timeout` / `retryAttempts` / `retryTimeout` | `30s` / `3` / `5s` | routing policy |
| `resources` | requests/limits | container resources |
| `nodeSelector` | `intent: apps` | node placement |
| `podAnnotations` / `podLabels` | `{}` | pod metadata |

## Validate

```bash
helm template apps/helm/openkite-preview \
  --set fullnameOverride=openkite-preview-42 \
  --set image.tag=pr-42 \
  --set ingress.host=pr42-openkite.maklab.net
helm unittest apps/helm/openkite-preview
```
