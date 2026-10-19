# cloudflare-access

Renders one `CloudflareAccessApp` (XRD from `services/helm/crossplane`) per
private host. Nothing here talks to Cloudflare — the provider behind the
Composition does, and it writes the resulting audience to
`TrustAccessApplication.status.atProvider.aud`.

## Values

| Key | Default | Description |
| --- | --- | --- |
| `accountId` | `""` | Cloudflare account id. Injected by the appset from `crossplaneAccess.accountId`. |
| `sessionDuration` | `24h` | Same value the Terraform-managed apps used. |
| `policies` | `[]` | Access policies passed through untouched. Empty keeps the XRD default (allow anyone who cleared the account IdP). |
| `apps` | `{}` | `name: {host}` map. Injected by `services/argocd-appset` from every service with `gateways.enable_private: true`. |

## Why a separate app from `crossplane`

The XR schema is installed by the crossplane app at sync wave 3. This app runs
at wave 7 so the `CloudflareAccessApp` CRD exists before Argo applies an
instance; `SkipDryRunOnMissingResource` covers the first install race while the
provider package is still pulling.
