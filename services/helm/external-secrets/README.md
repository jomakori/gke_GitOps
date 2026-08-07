# external-secrets

![Version: 2.5.0](https://img.shields.io/badge/Version-2.5.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v2.5.0](https://img.shields.io/badge/AppVersion-v2.5.0-informational?style=flat-square)
A Helm chart for the External Secrets operator

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://charts.external-secrets.io/ | external-secrets | 2.5.0 |

## Under the hood

This chart deploys the [External Secrets Operator](https://charts.external-secrets.io) and generates `ClusterSecretStore` resources for each Doppler config defined in the `clusterSecretStores` values.

### Hybrid chart structure

- **Upstream dependency**: `external-secrets/external-secrets` — the operator itself.
- **Local templates**: `cluster-secret-store.yaml` (generates one `ClusterSecretStore` per entry in `clusterSecretStores`), `selfsigned-issuer.yaml` (self-signed `ClusterIssuer` for the webhook cert).

### Secrets chain

```
Doppler
  → doppler-machine-token Secret (created by Terraform, stored in external-secrets namespace)
    → ClusterSecretStore (one per Doppler config, references the machine token)
      → ExternalSecret (per app/service, pulls all keys matching ".*")
        → K8s Secret (keys match Doppler key names)
          → Pod secretKeyRef
```

### ClusterSecretStore auto-generation

Each entry in `clusterSecretStores` becomes a `ClusterSecretStore` named `doppler-{config}` (underscores replaced with dashes):
```yaml
clusterSecretStores:
  svc_grafana:
    project: devops
    config: svc_grafana
```
→ `ClusterSecretStore` named `doppler-svc-grafana`, pointing at project `devops`, config `svc_grafana`.

The store references `doppler-machine-token` in the `external-secrets` namespace. Adding a new Doppler config is done by adding an entry here — no Terraform changes required; the `ClusterSecretStore` template auto-generates from the values.

| Store Name | Project | Config | Used By |
|-----------|---------|--------|---------|
| `doppler-svc-grafana` | devops | svc_grafana | kube-prometheus-stack |
| `doppler-svc-cloudflare` | devops | svc_cloudflare | istio, external-dns, cloudflare-tunnel |
| `doppler-svc-postgres-operator` | devops | svc_postgres_operator | postgres-operator |
| `doppler-svc-onedev` | devops | svc_onedev | onedev |
| `doppler-svc-mongodb` | devops | svc_mongodb | mongodb-operator, app PerconaServerMongoDB CRs |
| `doppler-svc-excalidash` | devops | svc_excalidash | excalidash |
| `doppler-svc-openclaw` | devops | svc_openclaw | openclaw |
| `doppler-zurabase-dev` | zurabase | dev | future zurabase service |
| `doppler-zurabase-stg` | zurabase | stg | future zurabase service |
| `doppler-zurabase-prd` | zurabase | prd | future zurabase service |

### Setup

| Aspect | Detail |
|--------|--------|
| **Namespace** | `external-secrets` |
| **Sync wave** | 1 (after cert-manager at wave 0) |
| **CRDs** | Installed via chart (`installCRDs: true`) |
| **Webhook cert** | Self-signed `ClusterIssuer` (`eso-selfsigned`) — does not depend on cert-manager |
| **Doppler config** | None — the machine token is pre-seeded by Terraform |
| **Ingress** | None — operator is internal-only |

### Dependency consumers

Any chart that needs secrets must be in wave ≥ 2 (after ClusterSecretStores exist). The retry limit is set to 5 with a 3-minute max duration to handle transient network issues with Doppler API.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| clusterSecretStores.svc_cloudflare.config | string | `"svc_cloudflare"` |  |
| clusterSecretStores.svc_cloudflare.project | string | `"devops"` |  |
| clusterSecretStores.svc_grafana.config | string | `"svc_grafana"` |  |
| clusterSecretStores.svc_grafana.project | string | `"devops"` |  |
| clusterSecretStores.svc_mongodb.config | string | `"svc_mongodb"` |  |
| clusterSecretStores.svc_mongodb.project | string | `"devops"` |  |
| clusterSecretStores.svc_openagent.config | string | `"svc_openagent"` |  |
| clusterSecretStores.svc_openagent.project | string | `"devops"` |  |
| clusterSecretStores.svc_postgres_operator.config | string | `"svc_postgres_operator"` |  |
| clusterSecretStores.svc_postgres_operator.project | string | `"devops"` |  |
| clusterSecretStores.zurabase-dev.config | string | `"dev"` |  |
| clusterSecretStores.zurabase-dev.project | string | `"zurabase"` |  |
| clusterSecretStores.zurabase-prd.config | string | `"prd"` |  |
| clusterSecretStores.zurabase-prd.project | string | `"zurabase"` |  |
| clusterSecretStores.zurabase-stg.config | string | `"stg"` |  |
| clusterSecretStores.zurabase-stg.project | string | `"zurabase"` |  |
| external-secrets.installCRDs | bool | `true` | If set, install and upgrade CRDs through helm chart. |
| external-secrets.replicaCount | int | `2` |  |
| external-secrets.serviceAccount.create | bool | `true` |  |
| external-secrets.webhook.certManager.enabled | bool | `false` |  |

## Webhook cert lifecycle (`Replace=true` footgun)

The external-secrets Application syncs with the `Replace=true` sync option
(needed for CRD upgrades). `Replace` bypasses `ignoreDifferences`, so **any
re-sync of this chart replaces** the helm-rendered `external-secrets-webhook`
Secret and both ValidatingWebhookConfigurations with the chart's empty
template — wiping the ESO-generated webhook CA/certs and the injected
`caBundle`. The webhook then crash-loops (`stat /tmp/certs/tls.crt: no such
file`) and **every ExternalSecret write fails cluster-wide** (`failed calling
webhook ... no endpoints available` / `x509: certificate signed by unknown
authority`).

This is what happened on 2026-08-07: adding `svc_excalidash` to
`clusterSecretStores` triggered an auto-sync that wiped the certs.

### Protection

`helm.sh/resource-policy: keep` alone is **not enough** — ArgoCD honors it only
for pruning, not for `Replace` syncs. The durable fix (git):

1. **No `Replace=true`** on the external-secrets Application (appset entry) —
   `Replace` bypasses `ignoreDifferences` and clobbers ESO-managed state.
2. **`ignoreDifferences`** for the webhook Secret (`/data`, `/type`) and both
   ValidatingWebhookConfigurations (`.webhooks[].clientConfig.caBundle`, ...) —
   so apply-mode syncs never revert ESO's cert data or the injected caBundle.

Live annotations on the three resources are belt-and-suspenders and must be
re-applied if they are ever recreated: `helm.sh/resource-policy: keep` +
`argocd.argoproj.io/sync-options: Replace=false` on:
- Secret `external-secrets/external-secrets-webhook`
- ValidatingWebhookConfiguration `externalsecret-validate`
- ValidatingWebhookConfiguration `secretstore-validate`

### Recovery

1. Restart the cert-controller — `kubectl delete pod -n external-secrets -l app.kubernetes.io/name=external-secrets-cert-controller`. It regenerates the CA + webhook certs and injects the `caBundle` into both webhook configs.
2. If the webhook Secret was deleted, recreate it empty first (chart shape: labels + `type: Opaque`, no data). ESO writes certs into an existing secret; it does not create it.
3. Re-apply `helm.sh/resource-policy: keep` on the Secret and both ValidatingWebhookConfigurations.
4. Verify: webhook pod Ready, `clientConfig.caBundle` present on both ValidatingWebhookConfigurations, and `kubectl get externalsecret` no longer reports webhook errors.
