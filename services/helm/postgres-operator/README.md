# postgres-operator

![Version: 1.18.6](https://img.shields.io/badge/Version-1.18.6-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.18.6](https://img.shields.io/badge/AppVersion-1.18.6-informational?style=flat-square)
A Helm chart for StackGres - a PostgreSQL operator

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://stackgres.io/downloads/stackgres-k8s/stackgres/helm/ | stackgresOperator(stackgres-operator) | 1.18.6 |

## Under the hood

This chart deploys [StackGres](https://stackgres.io) — a full-featured PostgreSQL operator — as a hybrid chart wrapping the upstream `stackgres-operator` dependency with a local ExternalSecret template.

### Doppler config

The ExternalSecret (`stackgres-restapi-admin`) pulls two keys from the `svc_postgres_operator` Doppler config:

| Secret | Purpose |
|--------|---------|
| `ADMIN_USER` | StackGres REST API admin username (`k8sUsername`) |
| `ADMIN_PASSWORD` | StackGres REST API admin password (`clearPassword`) |

### CRD handling

CRDs are installed via the operator's `INSTALL_CRDS=true` env var, not Helm — the chart sets `skipCrds: true` to prevent ArgoCD normalization errors on CRD resources. This is reflected in the ArgoCD Application configuration with `ignoreDifferences` for the `apiextensions.k8s.io/CustomResourceDefinition` group.

### How apps consume this operator

App charts provision PostgreSQL clusters via `enable_db.type: postgres` in their values. The `_helpers.tpl` template in `apps/helm/` generates an `SGCluster` (StackGres custom resource) with:

- **`deployment: db`** — single instance, fixed at 1 replica
- **`deployment: cluster`** — multi-instance with StackGres built-in connection-based horizontal autoscaling (`replicasConnectionsUsageTarget: 0.5`), min/max instances from `enable_scaling.db`

StackGres also provides `SGPostgresConfig`, `SGPoolingConfig`, and supporting CRDs used by the onedev chart and any app with Postgres enabled.

### Operational notes

- Sync wave 3, namespace `postgres-operator`
- Certificate auto-approval enabled, self-signed certs with 730d duration
- Operator extensions cache uses `local-path` storage class with 1Gi volume
- Grafana auto-embed enabled (discovers Grafana instance in cluster)
- Backup storage disabled by default
- No admin UI exposed (service type ClusterIP, exposeHTTP: false)

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| dopplerConfig | string | `"svc_postgres_operator"` |  |
| postgres.clusters.pg-main.backups.bucket | string | `"stackgres-pg-main"` |  |
| postgres.clusters.pg-main.backups.createStorageAccountSecret | bool | `false` |  |
| postgres.clusters.pg-main.backups.credentialsSecret | string | `"pg-secrets"` |  |
| postgres.clusters.pg-main.backups.credentialsSecretManagedByOperator | bool | `false` |  |
| postgres.clusters.pg-main.backups.cronSchedule | string | `"0 2 */2 * *"` |  |
| postgres.clusters.pg-main.backups.enabled | bool | `true` |  |
| postgres.clusters.pg-main.backups.endpoint | string | `"https://f8caac5c12d315d3690fd6df74cacdb9.r2.cloudflarestorage.com"` |  |
| postgres.clusters.pg-main.backups.region | string | `"auto"` |  |
| postgres.clusters.pg-main.backups.retention | int | `4` |  |
| postgres.clusters.pg-main.configName | string | `"pg-main-config"` |  |
| postgres.clusters.pg-main.disableConnectionPooling | bool | `false` |  |
| postgres.clusters.pg-main.disableMetricsExporter | bool | `true` |  |
| postgres.clusters.pg-main.instances | int | `1` |  |
| postgres.clusters.pg-main.managedSqlScript | string | `"plane-pg-init"` |  |
| postgres.clusters.pg-main.namespace | string | `"data"` |  |
| postgres.clusters.pg-main.poolingConfigName | string | `"pg-main-pooling"` |  |
| postgres.clusters.pg-main.profile | string | `"production"` |  |
| postgres.clusters.pg-main.resources.cluster-controller.cpu | string | `"200m"` |  |
| postgres.clusters.pg-main.resources.cluster-controller.memory | string | `"384Mi"` |  |
| postgres.clusters.pg-main.resources.patroni.cpu | string | `"1000m"` |  |
| postgres.clusters.pg-main.resources.patroni.memory | string | `"2Gi"` |  |
| postgres.clusters.pg-main.resources.pgbouncer.cpu | string | `"250m"` |  |
| postgres.clusters.pg-main.resources.pgbouncer.memory | string | `"128Mi"` |  |
| postgres.clusters.pg-main.resources.postgres-util.cpu | string | `"100m"` |  |
| postgres.clusters.pg-main.resources.postgres-util.memory | string | `"128Mi"` |  |
| postgres.clusters.pg-main.storage | string | `"15Gi"` |  |
| postgres.clusters.pg-main.storageClass | string | `"local-path"` |  |
| postgres.clusters.pg-main.users[0].databases[0].name | string | `"plane"` |  |
| postgres.clusters.pg-main.users[0].initSqlKey | string | `"PLANE_INIT_SQL"` |  |
| postgres.clusters.pg-main.users[0].name | string | `"plane"` |  |
| postgres.clusters.pg-main.users[0].passwordKey | string | `"PLANE_PG_PASSWORD"` |  |
| postgres.clusters.pg-main.users[1].databases[0].name | string | `"litellm"` |  |
| postgres.clusters.pg-main.users[1].initSqlKey | string | `"OPENAGENT_INIT_SQL"` |  |
| postgres.clusters.pg-main.users[1].name | string | `"openagent"` |  |
| postgres.clusters.pg-main.users[1].passwordKey | string | `"OPENAGENT_PG_PASSWORD"` |  |
| postgres.clusters.pg-main.version | string | `"18"` |  |
| renderClusters | bool | `true` |  |
| renderOperator | bool | `true` |  |
| stackgresOperator.adminui.service.exposeHTTP | bool | `false` |  |
| stackgresOperator.adminui.service.type | string | `"ClusterIP"` |  |
| stackgresOperator.authentication.createAdminSecret | bool | `false` |  |
| stackgresOperator.authentication.type | string | `"jwt"` |  |
| stackgresOperator.cert.autoapprove | bool | `true` |  |
| stackgresOperator.cert.certDuration | int | `730` |  |
| stackgresOperator.cert.createForCollector | bool | `true` |  |
| stackgresOperator.cert.createForOperator | bool | `true` |  |
| stackgresOperator.cert.createForWebApi | bool | `true` |  |
| stackgresOperator.cert.regenerateCert | bool | `true` |  |
| stackgresOperator.cert.regenerateWebCert | bool | `true` |  |
| stackgresOperator.cert.regenerateWebRsa | bool | `true` |  |
| stackgresOperator.enabled | bool | `true` |  |
| stackgresOperator.extensions.cache.enabled | bool | `true` |  |
| stackgresOperator.extensions.cache.persistentVolume.size | string | `"1Gi"` |  |
| stackgresOperator.extensions.cache.persistentVolume.storageClass | string | `"local-path"` |  |
| stackgresOperator.extensions.cache.preloadedExtensions[0] | string | `"x86_64/linux/timescaledb-1\\.7\\.4-pg12"` |  |
| stackgresOperator.extensions.repositoryUrls[0] | string | `"https://extensions.stackgres.io/postgres/repository"` |  |
| stackgresOperator.grafana.autoEmbed | bool | `true` |  |
| stackgresOperator.grafana.password | string | `nil` |  |
| stackgresOperator.grafana.schema | string | `"http"` |  |
| stackgresOperator.grafana.user | string | `nil` |  |
| stackgresOperator.operator.resources.limits.memory | string | `"1Gi"` |  |
| stackgresOperator.operator.resources.requests.cpu | string | `"100m"` |  |
| stackgresOperator.operator.resources.requests.memory | string | `"256Mi"` |  |
| stackgresOperator.rbac.create | bool | `true` |  |
