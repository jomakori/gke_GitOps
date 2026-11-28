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
| apps | object | `{}` | apps: host map, injected by services/argocd-appset — from every service with gateways.enable_private: true, and from its `accessHosts` for hosts whose route is declared outside the appset (the tickerlight console, the openkite preview wildcard). One CloudflareAccessApp per entry. Do not list entries here: the appset owns the list, and a second copy is how a host ends up gated but unregistered.  Example of what the appset injects:   apps.openkite.host   = openkite.maklab.net   apps.openkite.appId  = e47467cc-… |
| policies | list | `[]` | Pass-through Access policies. Empty means the XRD default (allow anyone who cleared the account identity provider), which matches the Terraform apps. |
| sessionDuration | string | `"24h"` | Same value the Terraform-managed apps used. |

## Why a separate app from `crossplane`

The XR schema is installed by the crossplane app at sync wave 3. This app runs
at wave 7 so the `CloudflareAccessApp` CRD exists before Argo applies an
instance; `SkipDryRunOnMissingResource` covers the first install race while the
provider package is still pulling.
