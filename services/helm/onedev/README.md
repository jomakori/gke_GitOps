# onedev

![Version: 15.0.8](https://img.shields.io/badge/Version-15.0.8-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 15.0.8](https://img.shields.io/badge/AppVersion-15.0.8-informational?style=flat-square)
A Helm chart for OneDev - an all-in-one DevOps platform

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| local |  |  |

## Setup

This is a **custom chart** that combines the vendored upstream OneDev chart with local templates for database provisioning via StackGres.

### Sync wave and namespace

- **Sync wave**: 4
- **Namespace**: `onedev`

### Local templates (4)

| Template | Kind | Sync wave | Purpose |
|----------|------|-----------|---------|
| `sgpostgresconfig.yaml` | `SGPostgresConfig` | 0 | PostgreSQL config (timezone) |
| `sgpoolingconfig.yaml` | `SGPoolingConfig` | 0 | Connection pooling via pgBouncer |
| `sgcluster.yaml` | `SGCluster` | 1 | External PostgreSQL cluster `onedev-pg` |
| `externalsecret.yaml` | `ExternalSecret` | 5 | Doppler secret injection |

The StackGres config resources (SGPostgresConfig, SGPoolingConfig) are created at wave 0 so they exist before the SGCluster references them at wave 1. The ExternalSecret is at wave 5 to ensure the `onedev` deployment exists before secrets are injected.

### Doppler config

The ExternalSecret pulls `DB_PASSWORD` from the `svc_onedev` Doppler config and maps it to the `dbPassword` key in the `onedev-secrets` Secret. The upstream OneDev chart references this secret for database authentication.

| Secret Key | Doppler Source |
|------------|----------------|
| `dbPassword` | `DB_PASSWORD` |
| `dbUser` (hardcoded) | `DB_USER` (injected via values) |

### Database

OneDev uses an external PostgreSQL database managed by StackGres:

| Parameter | Value |
|-----------|-------|
| Cluster name | `onedev-pg` |
| Instances | 1 (single primary, no replicas) |
| Postgres version | 18 |
| Storage | 5Gi |
| Pooling | Transaction mode, max 100 client connections |
| Profile | `development` (no anti-affinity, no resource limits) |

The connection URL is `onedev-pg.onedev.svc.cluster.local:5432` — internal cluster DNS, no TLS.

### Ingress

OneDev does **not** expose a public ingress via the istio umbrella. Access is through `kubectl port-forward` or direct pod networking.

### Node scheduling

OneDev runs on dedicated app infrastructure:

- **Node selector**: `intent: apps`
- **Tolerations**: `dedicated=apps:NoSchedule`
- **Affinity**: `class=guaranteed`

### Persistence

| Volume | Size | Storage class |
|--------|------|---------------|
| OneDev data (repos, artifacts, config) | 50Gi | Default (cluster) |
| PostgreSQL data | 5Gi | Default (cluster) |

### Resources

| Container | Request | Limit |
|-----------|---------|-------|
| OneDev | 500m CPU, 1Gi RAM | 2 CPU, 4Gi RAM |
| PostgreSQL | 1 core, 2Gi RAM | StackGres default instance profile |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| dopplerConfig | string | `"svc_onedev"` |  |
| onedev.database.external | bool | `true` |  |
| onedev.database.host | string | `"onedev-pg.onedev.svc.cluster.local"` |  |
| onedev.database.maximumPoolSize | string | `"25"` |  |
| onedev.database.name | string | `"postgres"` |  |
| onedev.database.password | string | `"placeholder"` |  |
| onedev.database.port | string | `"5432"` |  |
| onedev.database.type | string | `"postgresql"` |  |
| onedev.database.user | string | `"postgres"` |  |
| onedev.nodeSelector.intent | string | `"apps"` |  |
| onedev.persistence.size | string | `"50Gi"` |  |
| onedev.persistence.storageClassName | string | `""` |  |
| onedev.replicas | int | `1` |  |
| onedev.resources.limits.cpu | int | `2` |  |
| onedev.resources.limits.memory | string | `"4Gi"` |  |
| onedev.resources.requests.cpu | string | `"500m"` |  |
| onedev.resources.requests.memory | string | `"1Gi"` |  |
| onedev.tolerations[0].effect | string | `"NoSchedule"` |  |
| onedev.tolerations[0].key | string | `"dedicated"` |  |
| onedev.tolerations[0].operator | string | `"Equal"` |  |
| onedev.tolerations[0].value | string | `"apps"` |  |
| onedev.updateStrategy.type | string | `"RollingUpdate"` |  |
| postgresql.clusterName | string | `"onedev-pg"` | PostgreSQL cluster name (also used as the K8s service name) |
| postgresql.instances | int | `1` | Number of Postgres instances (1 = single primary, no replicas) |
| postgresql.postgresVersion | string | `"18"` | PostgreSQL major version (must match SGPostgresConfig.postgresVersion) |
| postgresql.profile | string | `"development"` | Profile for the SGCluster (development = no anti-affinity, no resource limits) |
| postgresql.sgInstanceProfile | string | `""` | SGInstanceProfile name (empty = use StackGres default: 1 core, 2Gi RAM) |
| postgresql.sgPoolingConfig | string | `"onedev-pg-pooling"` | SGPoolingConfig resource name (not used when disableConnectionPooling=true) |
| postgresql.sgPostgresConfig | string | `"onedev-pg-config"` | SGPostgresConfig resource name |
| postgresql.storage | string | `"5Gi"` | Storage size for Postgres data |
