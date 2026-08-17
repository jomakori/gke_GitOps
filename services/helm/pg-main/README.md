# pg-main — centralized PostgreSQL (StackGres)

One shared StackGres cluster (`pg-main`, namespace `data`) hosting a database + role per service,
replacing per-service SGClusters (e.g. `openagent-pg`).

## Ownership model (user-directed: "the db should be in gitops")

- **DB definition = gitops** (`gke_GitOps`, ArgoCD-managed): cluster, per-service DBs/roles, and
  backup schedules all live in this chart + `services/argocd-appset/values.yaml`. No Terraform, no
  ad-hoc kubectl for DB resources.
- **Terraform (`devops_Terraform`)** only provisions cloud-side: the R2 bucket (`stackgres-pg-main`)
  and Doppler secret population.
- **Secrets** (per-service passwords, R2 keys) live in Doppler config `svc_postgres_operator`
  (devops project) → ExternalSecret `pg-secrets` (wave -3) → cluster.
- Per-service onboarding = one gitops PR (users entry + Doppler password). Nothing else.

## Onboarding a new service `<svc>`

1. **Doppler** (`devops` project, config `svc_postgres_operator`): add `<SVC>_PG_PASSWORD` (generated once, strong).
2. **values.yaml** — add to `postgres.users`:
   ```yaml
   users:
     - name: <svc>
       passwordKey: <SVC>_PG_PASSWORD
       databases: [<svc>]
   ```
3. **Authz** (when `authz.enabled: true`): add `cluster.local/ns/<svc>/sa/<svc-sa>` to `authz.principals`.
4. **App env**:
   `DATABASE_URL: postgresql://<svc>:<pw>@pg-main-primary.data.svc.cluster.local:5432/<svc>`
   (or `pg-main.data.svc.cluster.local` — bare name resolves to primary).
5. Verify: `psql` from the consumer pod; cross-namespace non-listed SAs denied once authz is on.

## Backups

- Weekly base backup + continuous WAL archiving to Cloudflare R2 (`sgobjectstorage` v1beta1, s3Compatible).
- Manual backup: create an `SGBackup` referencing `pg-main-storage`, or run
  `kubectl exec -n data pg-main-0 -- /opt/stackgres/bin/pgbackrest ...` for ad-hoc restores.
- Restore drill: create a throwaway SGCluster with `spec.initialData.restore` pointing at the backup.

## Known limitations

- `authz` disabled by default: the StackGres operator (non-ambient `postgres-operator` ns) must reach
  the cluster; principal-based policies would block non-mesh management traffic. Revisit when
  hardening the mesh.
- SGBackupConfig CRD is absent in this StackGres version — schedules are inline in `SGCluster.spec.backups`.
