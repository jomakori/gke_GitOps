# external-dns

![Version: 1.16.1](https://img.shields.io/badge/Version-1.16.1-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v0.16.1](https://img.shields.io/badge/AppVersion-v0.16.1-informational?style=flat-square)
ExternalDNS — automatic DNS record management via Cloudflare for maklab.net

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://kubernetes-sigs.github.io/external-dns/ | external-dns | 1.16.1 |

## Under the hood

This chart deploys [ExternalDNS](https://kubernetes-sigs.github.io/external-dns/) configured with the **Cloudflare** provider. It watches Istio Gateway and VirtualService hosts and automatically creates the corresponding DNS records in Cloudflare.

### Hybrid chart structure

- **Upstream dependency**: `external-dns/external-dns` — the DNS controller.
- **Local template**: `cloudflare-external-secret.yaml` — creates an `ExternalSecret` pulling `CF_API_TOKEN` from Doppler (`svc_cloudflare` config).

### How it works

ExternalDNS watches three resource types specified in `sources`:
- `istio-gateway` — reads hosts from `maklab-gateway` in `istio-system`
- `istio-virtualservice` — reads hosts from each VirtualService under `*.<clusterDomain>`
- `service` — reads LoadBalancer/NodePort ingress status

For each host it discovers (e.g. `argocd.maklab.net`, `grafana.maklab.net`), ExternalDNS creates an A/AAAA/CNAME record in Cloudflare DNS. The `domainFilters` restrict management to the cluster domain, preventing accidental modification of unrelated DNS zones.

### Safety

**`upsert-only` policy** — ExternalDNS never deletes DNS records. Stale entries must be cleaned up manually in the Cloudflare dashboard. A TXT ownership registry (`external-dns-gke-maklab`) prevents conflicts between this cluster and other DNS managers.

### Doppler config

| Aspect | Detail |
|--------|--------|
| **Doppler config** | `svc_cloudflare` (shared with istio and cloudflare-tunnel) |
| **Secret consumed** | `CF_API_TOKEN` — injected as environment variable `CF_API_TOKEN` |
| **ClusterSecretStore** | `doppler-svc-cloudflare` (created by external-secrets chart at wave 1) |

### Setup

| Aspect | Detail |
|--------|--------|
| **Namespace** | `external-dns` |
| **Sync wave** | 3 (after istio at wave 2 — needs Gateway and VirtualServices to exist) |
| **Policy** | `upsert-only` |
| **Provider** | Cloudflare |
| **Domain filter** | Cluster domain from `domainFilters` |
| **Node placement** | `intent: apps` nodes |
| **Resources** | 50m CPU / 64Mi requests, 128Mi limit |

### Operational notes

- ExternalDNS discovers Istio Gateway hosts and VirtualService hosts independently. If a VirtualService host is removed, the DNS record persists (upsert-only policy).
- The `--cloudflare-dns-records-per-page=5000` extra arg ensures ExternalDNS can enumerate all records even in large zones.
- For troubleshooting: check ExternalDNS logs for API rate limiting or auth errors. The `cloudflare-api-token` Secret must contain a valid Cloudflare API token with `DNS:Edit` permission.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| dopplerConfig | string | `"svc_cloudflare"` |  |
| external-dns.domainFilters[0] | string | `"maklab.net"` |  |
| external-dns.env[0].name | string | `"CF_API_TOKEN"` |  |
| external-dns.env[0].valueFrom.secretKeyRef.key | string | `"CF_API_TOKEN"` |  |
| external-dns.env[0].valueFrom.secretKeyRef.name | string | `"cloudflare-api-token"` |  |
| external-dns.extraArgs[0] | string | `"--cloudflare-dns-records-per-page=5000"` |  |
| external-dns.logLevel | string | `"info"` |  |
| external-dns.nodeSelector.intent | string | `"apps"` |  |
| external-dns.policy | string | `"upsert-only"` |  |
| external-dns.provider.name | string | `"cloudflare"` |  |
| external-dns.rbac.create | bool | `true` |  |
| external-dns.registry | string | `"txt"` |  |
| external-dns.resources.limits.memory | string | `"128Mi"` |  |
| external-dns.resources.requests.cpu | string | `"50m"` |  |
| external-dns.resources.requests.memory | string | `"64Mi"` |  |
| external-dns.sources[0] | string | `"istio-gateway"` |  |
| external-dns.sources[1] | string | `"istio-virtualservice"` |  |
| external-dns.sources[2] | string | `"service"` |  |
| external-dns.txtOwnerId | string | `"gke-maklab"` |  |
| external-dns.txtPrefix | string | `"external-dns-"` |  |
