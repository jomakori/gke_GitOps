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
| clusterSecretStores.svc_onedev.config | string | `"svc_onedev"` |  |
| clusterSecretStores.svc_onedev.project | string | `"devops"` |  |
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
