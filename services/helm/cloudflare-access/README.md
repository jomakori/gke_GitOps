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
| accountId | string | `""` | Cloudflare account that owns the Access applications. Not a secret; same value as CF_ACCOUNT_ID in Doppler. Injected by the appset from crossplaneAccess.accountId, and required once the app is enabled. |
| apps | object | `{}` | apps: host map, injected by services/argocd-appset from every service with gateways.enable_private: true. One CloudflareAccessApp per entry. |
| policies | list | `[]` | Pass-through Access policies. Empty means the XRD default (allow anyone who cleared the account identity provider), which matches the Terraform apps. |
| sessionDuration | string | `"24h"` | Same value the Terraform-managed apps used. |

## Why a separate app from `crossplane`

The XR schema is installed by the crossplane app at sync wave 3. This app runs
at wave 7 so the `CloudflareAccessApp` CRD exists before Argo applies an
instance; `SkipDryRunOnMissingResource` covers the first install race while the
provider package is still pulling.
