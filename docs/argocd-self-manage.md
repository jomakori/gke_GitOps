# ArgoCD self-management

ArgoCD syncs its own Helm release from `services/helm/argo-cd/` via the `argoCd` entry in `services/argocd-appset/values.yaml`. Until now Terraform installed it (`helm_release "argocd"`, `devops_Terraform/k8s-maklab-cluster/3-managed_services.tf`, values in `helm/argocd-values.yaml`).

**Scope.** This repo holds the chart and the registry entry only. The Terraform side (dropping the `helm_release` and re-pointing `depends_on`) lives in the sibling `devops_Terraform` repo and is **not** part of this change. The root apps (`apps`, `services`) and the GitHub webhook stay Terraform-owned.

## What ships

| Piece | Where |
|---|---|
| Wrapper chart, upstream `argo-cd` pinned in `Chart.yaml` | `services/helm/argo-cd/` |
| Production values, ported verbatim from the Terraform values, under the `argo-cd:` key | `services/helm/argo-cd/values.yaml` |
| Registry entry, **`enable: false`** | `services/argocd-appset/values.yaml` → `argoCd` |
| `noPrune` opt-out (the generic `prune: false` is swallowed by `default true` in `_helpers.tpl`; fixing that would also change `excalidash`/`opendesign`) | `services/argocd-appset/templates/_helpers.tpl` |

Guard rails on the entry: `name: argo-cd`, `finalizer: false`, `selfHeal: false`, `noPrune: true`, `syncWave: "0"`, `ServerSideApply` + `RespectIgnoreDifferences`, and `ignoreDifferences` on `/data` of `argocd-secret` and `argocd-notifications-secret` (both rendered empty by the chart and written by ArgoCD at runtime).

## Cutover

Order matters: two owners on one release fight each other.

1. **Merge this PR.** The entry is disabled, so the appset renders identically to before.
2. **Hand the release off in Terraform** (`devops_Terraform`, workspace `k8s-maklab-cluster` is local-execution):
   - `terraform state rm helm_release.argocd` — **never destroy**; destroy uninstalls ArgoCD. (Or a `removed { from = helm_release.argocd  lifecycle { destroy = false } }` block.)
   - Remove the resource block and re-point the `depends_on = [helm_release.argocd]` on the root apps and the webhook in `4-gitops.tf`.
   - `terraform plan` must show no changes to the cluster.
3. **Review the first sync before it happens.** The pin is the latest upstream chart, ahead of what runs live (`argo-cd` 10.11.7 / v3.5.7), so enabling is also an upgrade (CRDs, controller `Service`, redis-secret-init hook). Diff it:
   ```sh
   helm -n argocd get manifest argo-cd > /tmp/live.yaml
   helm dependency build services/helm/argo-cd
   helm template argo-cd services/helm/argo-cd -n argocd > /tmp/new.yaml
   ```
4. **Flip `argoCd.enable: true`** in a follow-up PR. Watch `kubectl -n argocd get app argo-cd` and the rollout. The controller restarts mid-sync; the retry (`retryLimit: 3`) absorbs it.
5. **Verify** (wait for a full reconcile cycle, not a quick glance): `argo-cd` Synced/Healthy, all argocd pods Ready, `https://argocd.maklab.net` loads, a GitHub webhook delivery returns 200, and `apps`/`services` still reconcile.
6. **Then** set `selfHeal: true` in the registry. `prune` and the finalizer stay off permanently: ArgoCD must not prune itself or be cascade-deleted with its Application.
7. Optionally delete the stale `sh.helm.release.v1.argo-cd.*` Secrets once adoption is verified (they are inert; do **not** `helm uninstall`).

Rollback before step 6: delete the Application (no finalizer, so the workloads stay) and `terraform import helm_release.argocd argocd/argo-cd`.

## Gotchas

- **The Application name is load-bearing.** It is the Helm release name, which prefixes every Service (`argo-cd-argocd-server`). The Istio VirtualService (`services.istio.parameters`) and the webhook pin it. Renaming breaks routing.
- **Secrets are not in git.** `argocd-secret` (admin hash, `server.secretkey`, tokens) is written by ArgoCD itself, and `argocd-initial-admin-secret`, `argocd-redis` and `argocd-github-token-openkite` are created out-of-band. The webhook secret is the Doppler `svc_argocd` config, read by Terraform. The chart consumes no Doppler config; the appset helper passes the parameter regardless.
- **Server-side apply adopts the Helm-installed objects.** Several argo-cd CRDs exceed the 256KB last-applied annotation, so `ServerSideApply=true` is required.
- **Ignore-differences only apply with `RespectIgnoreDifferences=true`** on automated syncs; both are set.
- **Pre-existing state:** at audit time the Terraform-owned `services` app was `OutOfSync` and `apps` was `Degraded`. Understand why before cutover, so their state is not blamed on the migration.
- **Renovate** bumps the `argo-cd` dependency pin like any other chart; with self-heal on, a merged bump upgrades ArgoCD itself.
