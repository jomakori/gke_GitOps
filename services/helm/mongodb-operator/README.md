# mongodb-operator

![Version: 1.22.0](https://img.shields.io/badge/Version-1.22.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.22.0](https://img.shields.io/badge/AppVersion-1.22.0-informational?style=flat-square)
A Helm chart for Percona Operator for MongoDB - a MongoDB operator

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://percona.github.io/percona-helm-charts/ | psmdb-operator | 1.22.0 |

## Under the hood

This chart deploys the [Percona Operator for MongoDB](https://docs.percona.com/percona-operator-for-mongodb/) (PSMDB) as a thin wrapper around the upstream `psmdb-operator` chart. The operator manages `PerconaServerMongoDB` custom resources — replica sets with automated operations, backups, and TLS.

### Current status

**Disabled by default** (`enable: false` in the appset registry). Enable in `services/argocd-appset/values.yaml` when MongoDB workloads are ready. Sync wave 3, namespace `mongodb-operator`.

The operator runs with `watchAllNamespaces: true` so it can see `PerconaServerMongoDB` CRs in app namespaces without per-namespace configuration.

### How apps consume this operator

App charts provision MongoDB clusters via `enable_db.type: mongodb` in their values. The `_helpers.tpl` template in `apps/helm/` generates a `PerconaServerMongoDB` custom resource with:

- **`deployment: db`** — single-node replica set (`unsafeFlags.replsetSize: true`, rs0 size 1)
- **`deployment: cluster`** — multi-node replica set (rs0 size from `enable_scaling.db.minInstances`, default 3, includes PDB)

### Doppler config requirements

No local ExternalSecret — the app's own Doppler config must contain these keys for Percona to connect:

| Secret | Purpose |
|--------|---------|
| `MONGODB_USER` | Database user |
| `MONGODB_PASSWORD` | Database password |
| `MONGODB_DATABASE` | Default database name |

The `PerconaServerMongoDB` CR references `secrets.users: <namespace>-vars` which is the app's ExternalSecret target name.

### Scaling limitations

Unlike StackGres (connection-based autoscaling), MongoDB replica set size is **static** — change it via `enable_scaling.db.minInstances` and ArgoCD syncs the change. There is no KEDA integration for `PerconaServerMongoDB`. For true horizontal scaling, MongoDB sharding (multiple replica sets) is available for production but not configured through the `enable_db` spec.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| psmdb-operator.disableTelemetry | bool | `false` |  |
| psmdb-operator.logLevel | string | `"INFO"` |  |
| psmdb-operator.rbac.create | bool | `true` |  |
| psmdb-operator.serviceAccount.create | bool | `true` |  |
| psmdb-operator.watchAllNamespaces | bool | `true` |  |
