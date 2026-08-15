# cert-manager

![Version: 1.17.2](https://img.shields.io/badge/Version-1.17.2-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v1.17.2](https://img.shields.io/badge/AppVersion-v1.17.2-informational?style=flat-square)
cert-manager — automated TLS certificate management via Let's Encrypt

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://charts.jetstack.io | cert-manager | 1.17.2 |

## Under the hood

This chart wraps [jetstack/cert-manager](https://charts.jetstack.io) — the automated TLS certificate management controller for Kubernetes. It is a **thin wrapper** chart with no local templates. All configuration is passed through to the upstream dependency.

### TLS chain

cert-manager issues wildcard TLS certificates via **Let's Encrypt** using the **Cloudflare DNS-01** challenge. The full chain:

1. cert-manager validates domain ownership by creating a `_acme-challenge` TXT record in Cloudflare (requires `CF_API_TOKEN`).
2. On success, Let's Encrypt issues a wildcard certificate `*.<clusterDomain>` (default domain configured via `clusterDomain`).
3. The certificate is stored in a `Secret` in `istio-system` and consumed by the Istio ingress gateway.
4. cert-manager's built-in renewal controller re-issues certificates before expiry.

Cert-manager uses public DNS resolvers (`8.8.8.8:53`, `1.1.1.1:53`) exclusively for DNS-01 validation — no cluster DNS to avoid split-brain.

### Setup

| Aspect | Detail |
|--------|--------|
| **Namespace** | `cert-manager` |
| **Sync wave** | 0 (before all services that need TLS) |
| **CRDs** | Installed via chart (`crds.enabled: true`) |
| **Node placement** | Components on `intent: apps` nodes |
| **Doppler config** | None — reads `CF_API_TOKEN` from `cloudflare-api-token` `Secret` in `cert-manager` namespace, created by the Istio umbrella chart's ExternalSecret (wave -1) |

### Dependencies

cert-manager must be healthy before any chart that creates `Certificate` or `ClusterIssuer` resources (notably the Istio umbrella chart at wave 2). The `letsencrypt-prod` ClusterIssuer and the wildcard Certificate are created by the Istio chart, not by this chart.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| cert-manager.cainjector.nodeSelector.intent | string | `"apps"` |  |
| cert-manager.cainjector.replicaCount | int | `2` |  |
| cert-manager.crds.enabled | bool | `true` |  |
| cert-manager.extraArgs[0] | string | `"--dns01-recursive-nameservers=8.8.8.8:53,1.1.1.1:53"` |  |
| cert-manager.extraArgs[1] | string | `"--dns01-recursive-nameservers-only"` |  |
| cert-manager.nodeSelector.intent | string | `"apps"` |  |
| cert-manager.prometheus.enabled | bool | `true` |  |
| cert-manager.prometheus.servicemonitor.enabled | bool | `false` |  |
| cert-manager.replicaCount | int | `2` |  |
| cert-manager.webhook.nodeSelector.intent | string | `"apps"` |  |
| cert-manager.webhook.replicaCount | int | `2` |  |
