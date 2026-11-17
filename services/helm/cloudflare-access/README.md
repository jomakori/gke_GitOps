# cloudflare-access

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.0.0](https://img.shields.io/badge/AppVersion-1.0.0-informational?style=flat-square)
Cloudflare Access applications declared as CloudflareAccessApp XRs and reconciled by Crossplane

Renders one `CloudflareAccessApp` (XRD from `services/helm/crossplane`) per
private host. Nothing here talks to Cloudflare — the provider behind the
Composition does, and it writes the resulting audience to
`TrustAccessApplication.status.atProvider.aud`.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| accountId | string | `"f8caac5c12d315d3690fd6df74cacdb9"` | Cloudflare account that owns the Access applications. Not a secret; same value as CF_ACCOUNT_ID in Doppler, and still overridden by the appset from crossplaneAccess.accountId. Defaulted here because the static `apps` entries below need a renderable account outside the appset too (CI lints this chart on its own defaults). |
| apps | object | `{"tickerlight":{"appId":"52479931-1173-483a-b370-4a53e5a2c24b","host":"trade.maklab.net"},"tickerlightStaging":{"appId":"0f0346ef-04c8-4caa-ac9e-d30d753e81a2","host":"staging-trade.maklab.net"}}` | apps: host map, injected by services/argocd-appset from every service with gateways.enable_private: true. One CloudflareAccessApp per entry.  Example:   openkite:     host: openkite.maklab.net  The tickerlight console hosts are listed here explicitly: their routes come from apps/helm (the `tickerlight` Application), which the appset does not walk for `gateways.enable_private`, so nothing injects them. They are gated by services/helm/istio `privateHosts` and need an Access app per host or the DENY policy has no JWT to accept. |
| policies | list | `[]` | Pass-through Access policies. Empty means the XRD default (allow anyone who cleared the account identity provider), which matches the Terraform apps. |
| sessionDuration | string | `"24h"` | Same value the Terraform-managed apps used. |

## Why a separate app from `crossplane`

The XR schema is installed by the crossplane app at sync wave 3. This app runs
at wave 7 so the `CloudflareAccessApp` CRD exists before Argo applies an
instance; `SkipDryRunOnMissingResource` covers the first install race while the
provider package is still pulling.
