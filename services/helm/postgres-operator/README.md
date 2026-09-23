# postgres-operator

![Version: 1.18.6](https://img.shields.io/badge/Version-1.18.6-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.18.6](https://img.shields.io/badge/AppVersion-1.18.6-informational?style=flat-square)
A Helm chart for StackGres - a PostgreSQL operator

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://stackgres.io/downloads/stackgres-k8s/stackgres/helm/ | stackgres-operator | 1.18.6 |

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
| postgres.clusters.openagent-pg.backups.bucket | string | `"stackgres-pg-main"` |  |
| postgres.clusters.openagent-pg.backups.createStorageAccountSecret | bool | `true` |  |
| postgres.clusters.openagent-pg.backups.credentialsSecret | string | `"pg-secrets"` |  |
| postgres.clusters.openagent-pg.backups.credentialsSecretManagedByOperator | bool | `true` |  |
| postgres.clusters.openagent-pg.backups.cronSchedule | string | `"0 2 */2 * *"` |  |
| postgres.clusters.openagent-pg.backups.enabled | bool | `true` |  |
| postgres.clusters.openagent-pg.backups.endpoint | string | `"https://f8caac5c12d315d3690fd6df74cacdb9.r2.cloudflarestorage.com"` |  |
| postgres.clusters.openagent-pg.backups.region | string | `"auto"` |  |
| postgres.clusters.openagent-pg.backups.retention | int | `4` |  |
| postgres.clusters.openagent-pg.configName | string | `"openagent-pg-config"` |  |
| postgres.clusters.openagent-pg.disableConnectionPooling | bool | `true` |  |
| postgres.clusters.openagent-pg.disableMetricsExporter | bool | `true` |  |
| postgres.clusters.openagent-pg.instances | int | `1` |  |
| postgres.clusters.openagent-pg.managedSqlScript | string | `"openagent-pg-init"` |  |
| postgres.clusters.openagent-pg.namespace | string | `"openagent"` |  |
| postgres.clusters.openagent-pg.poolingConfigName | string | `"openagent-pg-pooling"` |  |
| postgres.clusters.openagent-pg.profile | string | `"development"` |  |
| postgres.clusters.openagent-pg.resources.cluster-controller.cpu | string | `"200m"` |  |
| postgres.clusters.openagent-pg.resources.cluster-controller.memory | string | `"384Mi"` |  |
| postgres.clusters.openagent-pg.resources.patroni.cpu | string | `"500m"` |  |
| postgres.clusters.openagent-pg.resources.patroni.memory | string | `"1Gi"` |  |
| postgres.clusters.openagent-pg.resources.postgres-util.cpu | string | `"100m"` |  |
| postgres.clusters.openagent-pg.resources.postgres-util.memory | string | `"128Mi"` |  |
| postgres.clusters.openagent-pg.storage | string | `"5Gi"` |  |
| postgres.clusters.openagent-pg.storageClass | string | `"local-path"` |  |
| postgres.clusters.openagent-pg.syncWave | string | `"-1"` |  |
| postgres.clusters.openagent-pg.version | string | `"18"` |  |
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
| postgres.clusters.pg-main.profile | string | `"development"` |  |
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
| stackgres-operator.adminui.service.exposeHTTP | bool | `false` |  |
| stackgres-operator.adminui.service.type | string | `"ClusterIP"` |  |
| stackgres-operator.authentication.createAdminSecret | bool | `false` |  |
| stackgres-operator.authentication.type | string | `"jwt"` |  |
| stackgres-operator.cert.autoapprove | bool | `true` |  |
| stackgres-operator.cert.certDuration | int | `730` |  |
| stackgres-operator.cert.createForCollector | bool | `true` |  |
| stackgres-operator.cert.createForOperator | bool | `true` |  |
| stackgres-operator.cert.createForWebApi | bool | `true` |  |
| stackgres-operator.cert.regenerateCert | bool | `true` |  |
| stackgres-operator.cert.regenerateWebCert | bool | `true` |  |
| stackgres-operator.cert.regenerateWebRsa | bool | `true` |  |
| stackgres-operator.enabled | bool | `true` |  |
| stackgres-operator.extensions.cache.enabled | bool | `true` |  |
| stackgres-operator.extensions.cache.persistentVolume.accessModes[0] | string | `"ReadWriteOnce"` |  |
| stackgres-operator.extensions.cache.persistentVolume.size | string | `"1Gi"` |  |
| stackgres-operator.extensions.cache.persistentVolume.storageClass | string | `"local-path"` |  |
| stackgres-operator.extensions.cache.preloadedExtensions[0] | string | `"x86_64/linux/timescaledb-1\\\\.7\\\\.4-pg12"` |  |
| stackgres-operator.extensions.repositoryUrls[0] | string | `"https://extensions.stackgres.io/postgres/repository"` |  |
| stackgres-operator.grafana.autoEmbed | bool | `true` |  |
| stackgres-operator.grafana.password | string | `nil` |  |
| stackgres-operator.grafana.schema | string | `"http"` |  |
| stackgres-operator.grafana.user | string | `nil` |  |
| stackgres-operator.operator.resources.limits.memory | string | `"1Gi"` |  |
| stackgres-operator.operator.resources.requests.cpu | string | `"100m"` |  |
| stackgres-operator.operator.resources.requests.memory | string | `"256Mi"` |  |
| stackgres-operator.rbac.create | bool | `true` |  |
