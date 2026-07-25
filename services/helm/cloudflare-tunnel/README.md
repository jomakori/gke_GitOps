# cloudflare-tunnel

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: latest](https://img.shields.io/badge/AppVersion-latest-informational?style=flat-square)
Cloudflare Zero Trust tunnel for public access to *.maklab.net

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| oci://ghcr.io/lexfrei/charts | cloudflare-tunnel | 0.16.1 |

## Under the hood

This is a **custom chart** (no upstream dependencies) that deploys a `cloudflared` daemon connecting the cluster to Cloudflare's edge via a Zero Trust tunnel. It provides public ingress for `*.<clusterDomain>` without opening firewall ports.

### Custom chart structure

Full local templates (4 files):

| Template | Purpose |
|----------|---------|
| `deployment.yaml` | cloudflared daemon, reads `TUNNEL_TOKEN` from env var |
| `external-secret.yaml` | Pulls `TUNNEL_TOKEN` from Doppler `svc_cloudflare` config |
| `serviceaccount.yaml` | Minimal SA with `automountServiceAccountToken: false` |
| `_helpers.tpl` | Label helpers for resource metadata |

### How the tunnel works

1. **Terraform** creates the Cloudflare Zero Trust tunnel and pushes the tunnel token to Doppler (`devops` project, `svc_cloudflare` config, key `TUNNEL_TOKEN`).
2. The `ExternalSecret` (sync wave -1) pulls `TUNNEL_TOKEN` from Doppler into a K8s `Secret` named `cloudflare-tunnel-auth`.
3. The `cloudflared` pod reads `TUNNEL_TOKEN` and establishes an outbound connection to the Cloudflare edge.
4. Tunnel config (ingress rules) is managed by the Terraform `cloudflare_zero_trust_tunnel_cloudflared_config` resource, not by this chart. The daemon auto-polls for config changes.

### Tunnel ingress path

```
Internet → Cloudflare edge (CDN, WAF, DDOS protection)
  → Cloudflare Tunnel (cloudflared daemon in-cluster)
    → Istio ingress gateway (NodePort)
      → VirtualService routing
        → Service → Pod
```

### Known issues

- **SNI mismatch**: The tunnel `origin_request` must set `match_sni_to_host: true` (configured in Terraform). Without this, Istio returns `filter_chain_not_found` → HTTP 502.
- **Config destruction**: The tunnel config resource cannot be destroyed from Terraform — manual cleanup in the Cloudflare dashboard is required.

### Doppler config

| Aspect | Detail |
|--------|--------|
| **Doppler config** | `svc_cloudflare` |
| **Secret consumed** | `TUNNEL_TOKEN` |
| **ClusterSecretStore** | `doppler-svc-cloudflare` (created by external-secrets at wave 1) |

### Setup

| Aspect | Detail |
|--------|--------|
| **Namespace** | `default` (shared with cluster infrastructure) |
| **Sync wave** | 4 (after istio at wave 2, external-dns at wave 3) |
| **Tunnel name** | `maklab-cluster` (must match Terraform-created tunnel) |
| **Replicas** | 2 (HA — each pod independently connects to Cloudflare) |
| **Resources** | 50m CPU / 64Mi requests, 128Mi memory limit |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| cloudflare.mode | string | `"remote"` |  |
| cloudflare.tunnelToken | string | `""` |  |
| cloudflare.tunnelTokenSecretName | string | `"cloudflare-tunnel-auth"` |  |
| dopplerConfig | string | `"svc_cloudflare"` |  |
| image.pullPolicy | string | `"IfNotPresent"` |  |
| image.repository | string | `"cloudflare/cloudflared"` |  |
| image.tag | string | `""` |  |
| replicaCount | int | `2` |  |
| tunnelName | string | `"maklab-cluster"` |  |
