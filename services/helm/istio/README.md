# istio

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.25.2](https://img.shields.io/badge/AppVersion-1.25.2-informational?style=flat-square)
Istio umbrella chart — base CRDs, control plane, ingress gateway, and config (Gateway, ClusterIssuer, Certificate, VirtualServices)

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://istio-release.storage.googleapis.com/charts | base | 1.25.2 |
| https://istio-release.storage.googleapis.com/charts | ingress-gateway(gateway) | 1.28.0 |
| https://istio-release.storage.googleapis.com/charts | istiod | 1.25.2 |

## Under the hood

This umbrella chart bundles 3 upstream Istio dependencies — **base** (CRDs), **istiod** (control plane), and **ingress-gateway** — plus 9 local templates for networking, TLS, and auth configuration. It is the single entry point for all cluster ingress.

### Hybrid chart structure

**Upstream dependencies:**
- `base` — Istio CRDs (Gateway, VirtualService, PeerAuthentication, etc.)
- `istiod` — Control plane in ambient mode, STRICT mTLS
- `gateway` (aliased `ingress-gateway`) — Ingress gateway deployment with NodePort service

**Local templates (9):**

| Template | Purpose |
|----------|---------|
| `gateway.yaml` | `maklab-gateway` — wildcard `*.<clusterDomain>`, TLS passthrough, HTTP→HTTPS redirect |
| `cluster-issuer.yaml` | `letsencrypt-prod` — Let's Encrypt ACME via Cloudflare DNS-01 |
| `certificate.yaml` | `wildcard-maklab-net` — wildcard TLS cert for `*.<clusterDomain>` |
| `virtual-services.yaml` | One `VirtualService` per entry in `virtualServices` with `enabled: true` |
| `peer-authentication.yaml` | STRICT mTLS mesh-wide |
| `request-authentication.yaml` | CF Access JWT validation (conditional on `cloudflare.access.audienceTag`) |
| `authorization-policy-private.yaml` | DENY rules per private VS (conditional on `enablePrivate: true`) |
| `cloudflare-external-secret.yaml` | ExternalSecret for `CF_API_TOKEN` in `istio-system` |
| `cloudflare-external-secret-certmanager.yaml` | ExternalSecret for `CF_API_TOKEN` in `cert-manager` namespace |

### Ingress and TLS

The Gateway `maklab-gateway` in `istio-system` listens on ports 80 and 443:
- Port 443 terminates TLS using the wildcard certificate (`wildcard-maklab-net-tls`).
- Port 80 issues a 308 redirect to HTTPS.
- Hosts match `*.<clusterDomain>` (configured via `clusterDomain`).

The istiod control plane runs in **ambient mode** — no sidecars. Namespaces opt in via `istio.io/dataplane-mode: ambient`. Internal mesh traffic uses STRICT mTLS.

### Cloudflare Access Zero Trust

Services that require authenticated access opt in via `enablePrivate: true` in their VirtualService config. The pipeline:

1. The `applications.yaml` template (in argocd-appset) detects `gateways.enable_private` and injects the `enablePrivate` flag into the istio chart parameters, along with the CF Access team domain and AUD tag from Terraform/Doppler.
2. `request-authentication.yaml` validates the CF Access JWT at the ingress gateway using the `Cf-Access-Jwt-Assertion` header.
3. `authorization-policy-private.yaml` creates a DENY rule per private host — traffic without a valid JWT is rejected.

`enable_public` and `enable_private` are mutually exclusive. The argocd-appset template fails if both are set on the same service.

### VirtualServices

Two layers of VirtualServices exist:
1. **This chart** (istio umbrella) — spec-driven entries in `virtualServices`, primarily for infrastructure services (argocd, headlamp, opencost). Registered explicitly in the values.
2. **App chart** (apps/helm) — per-app per-environment VS for application workloads, generated from the parameterized app chart.

### Doppler config

| Aspect | Detail |
|--------|--------|
| **Doppler config** | `svc_cloudflare` |
| **Secrets consumed** | `CF_API_TOKEN` (for Let's Encrypt DNS-01 and CF Access JWKS) |
| **ExternalSecrets** | Two copies of `cloudflare-api-token`: one in `istio-system`, one in `cert-manager` |

### Setup

| Aspect | Detail |
|--------|--------|
| **Namespace** | `istio-system` |
| **Sync wave** | 2 (after cert-manager wave 0, external-secrets wave 1) |
| **Gateway type** | NodePort (ports 30670/30160 — for local testing only) |
| **mTLS** | STRICT mesh-wide |
| **Certificates** | Let's Encrypt via Cloudflare DNS-01, auto-renewed |
| **HPA** | ingress gateway: min 2, max 5, CPU target 80% |
| **Node placement** | All components on `intent: apps` nodes |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| base.enableCRDTemplates | bool | `true` |  |
| certEmail | string | `"admin@maklab.net"` |  |
| certificate.duration | string | `"2160h"` |  |
| certificate.name | string | `"wildcard-maklab-net"` |  |
| certificate.renewBefore | string | `"720h"` |  |
| certificate.secretName | string | `"wildcard-maklab-net-tls"` |  |
| cloudflare.access.audienceTag | string | `""` |  |
| cloudflare.access.teamDomain | string | `""` |  |
| cloudflare.apiTokenSecretKey | string | `"CF_API_TOKEN"` |  |
| cloudflare.apiTokenSecretName | string | `"cloudflare-api-token"` |  |
| clusterDomain | string | `"maklab.net"` |  |
| dopplerConfig | string | `"svc_cloudflare"` |  |
| gateway.name | string | `"maklab-gateway"` |  |
| gateway.namespace | string | `"istio-system"` |  |
| ingress-gateway.autoscaling.enabled | bool | `true` |  |
| ingress-gateway.autoscaling.maxReplicas | int | `5` |  |
| ingress-gateway.autoscaling.minReplicas | int | `2` |  |
| ingress-gateway.autoscaling.targetCPUUtilizationPercentage | int | `80` |  |
| ingress-gateway.labels.app | string | `"istio-ingressgateway"` |  |
| ingress-gateway.labels.istio | string | `"ingressgateway"` |  |
| ingress-gateway.name | string | `"istio-ingressgateway"` |  |
| ingress-gateway.nodeSelector.intent | string | `"apps"` |  |
| ingress-gateway.resources.limits.memory | string | `"256Mi"` |  |
| ingress-gateway.resources.requests.cpu | string | `"100m"` |  |
| ingress-gateway.resources.requests.memory | string | `"128Mi"` |  |
| ingress-gateway.service.ports[0].name | string | `"http2"` |  |
| ingress-gateway.service.ports[0].nodePort | int | `32718` |  |
| ingress-gateway.service.ports[0].port | int | `80` |  |
| ingress-gateway.service.ports[0].protocol | string | `"TCP"` |  |
| ingress-gateway.service.ports[0].targetPort | int | `80` |  |
| ingress-gateway.service.ports[1].name | string | `"https"` |  |
| ingress-gateway.service.ports[1].nodePort | int | `30451` |  |
| ingress-gateway.service.ports[1].port | int | `443` |  |
| ingress-gateway.service.ports[1].protocol | string | `"TCP"` |  |
| ingress-gateway.service.ports[1].targetPort | int | `443` |  |
| ingress-gateway.service.type | string | `"NodePort"` |  |
| istiod.meshConfig.accessLogFile | string | `"/dev/stdout"` |  |
| istiod.meshConfig.defaultConfig.proxyStatsMatcher.inclusionRegexps[0] | string | `".*outlier_detection.*"` |  |
| istiod.meshConfig.defaultConfig.proxyStatsMatcher.inclusionRegexps[1] | string | `".*circuit_breakers.*"` |  |
| istiod.meshConfig.defaultConfig.proxyStatsMatcher.inclusionRegexps[2] | string | `".*upstream_rq_retry.*"` |  |
| istiod.meshConfig.defaultConfig.proxyStatsMatcher.inclusionRegexps[3] | string | `".*upstream_cx_.*"` |  |
| istiod.meshConfig.enablePrometheusMerge | bool | `true` |  |
| istiod.meshConfig.trustDomain | string | `"cluster.local"` |  |
| istiod.pilot.nodeSelector.intent | string | `"apps"` |  |
| istiod.pilot.resources.limits.memory | string | `"512Mi"` |  |
| istiod.pilot.resources.requests.cpu | string | `"100m"` |  |
| istiod.pilot.resources.requests.memory | string | `"256Mi"` |  |
| istiod.profile | string | `"ambient"` |  |
| istiod.telemetry.enabled | bool | `true` |  |
| istiod.telemetry.v2.prometheus.enabled | bool | `true` |  |
| virtualServices.argocd.destination.host | string | `"argo-cd-argocd-server.argocd.svc.cluster.local"` |  |
| virtualServices.argocd.destination.port | int | `80` |  |
| virtualServices.argocd.enabled | bool | `true` |  |
| virtualServices.argocd.host | string | `"argocd.maklab.net"` |  |
| virtualServices.headlamp.destination.host | string | `"headlamp.headlamp.svc.cluster.local"` |  |
| virtualServices.headlamp.destination.port | int | `80` |  |
| virtualServices.headlamp.enabled | bool | `false` |  |
| virtualServices.headlamp.host | string | `"headlamp.maklab.net"` |  |
| virtualServices.opencost.destination.host | string | `"opencost.opencost.svc.cluster.local"` |  |
| virtualServices.opencost.destination.port | int | `9090` |  |
| virtualServices.opencost.enabled | bool | `false` |  |
| virtualServices.opencost.host | string | `"opencost.maklab.net"` |  |
