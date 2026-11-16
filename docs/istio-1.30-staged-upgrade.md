# Istio 1.25 → 1.30 — staged upgrade plan

Tracks Renovate PR #473 (`update istio monorepo to v1.30.8`, listed in Dependency Dashboard #9).
**Do not merge #473 as-is.** It jumps `base`/`istiod`/`gateway` straight to `1.30.8` in one sync,
which Istio does not support (one minor per in-place upgrade) and which leaves `appVersion`
stale. This document is the replacement path. Nothing in this PR changes `Chart.yaml`.

## Where we are (verified on `origin/main`)

| Item | Value |
|---|---|
| `base` | `1.25.2` |
| `istiod` | `1.25.2` |
| `gateway` (alias `ingress-gateway`) | `1.28.0` — **three minors ahead of the control plane** |
| `appVersion` | `1.25.2` (tracks base/istiod; Renovate never touches it) |
| Target | `1.30.8` (latest `1.30.x` in the Istio chart index when checked) |

Latest patch per minor in `https://istio-release.storage.googleapis.com/charts` (identical for
`base`, `istiod`, `gateway`): `1.26.8` · `1.27.9` · `1.28.10` · `1.29.10` · `1.30.8`.

How it is deployed: the `istio` entry in `services/argocd-appset/values.yaml` (wave 2) renders one
ArgoCD Application from `services/helm/istio`. The three Istio sub-charts carry **no sync-wave
annotations**, so they sync together in wave 0 of that Application (ArgoCD's kind ordering applies
CRDs first); only the local templates use waves `-1…2`. `base.enableCRDTemplates: true` puts the CRDs
in `templates/`, so ArgoCD upgrades them with the chart (no manual `kubectl apply` of CRDs needed).

### Findings from the audit

1. **Gateway/istiod skew is real but runtime-benign.** The gateway chart renders the container as
   `image: auto` with `inject.istio.io/templates: gateway`, so the proxy image is filled in by the
   *istiod* injection webhook at pod creation. The running gateway proxy is therefore whatever
   istiod injects (1.25.x today); only the Deployment template comes from the 1.28.0 chart. Confirm
   on the cluster before stage 1 (see *Preflight*). Moving gateway to the same version as istiod at
   each stage removes the skew.
2. **Ambient is configured but the data plane is not deployed.** `istiod.profile: ambient` and the
   `istio.io/dataplane-mode: ambient` namespace label exist, but the umbrella chart contains no
   `ztunnel` or `istio-cni` chart (rendered output has neither), and the openagent chart docs state
   no ztunnel sits in any pod's path. This upgrade therefore only moves istiod + gateway; do not
   add ztunnel/CNI as part of it. Ingress policy is enforced by the gateway's Envoy
   (`RequestAuthentication` / `AuthorizationPolicy` on `istio-ingressgateway`).
3. **`appVersion` is not bumped by Renovate.** Bump it by hand in each stage PR to the `base`/`istiod`
   version (it feeds the README badge and `helm-docs` output; rerun `helm-docs`).
4. **Render check, done now (no cluster).** With the umbrella chart's current `values.yaml`, `helm
   dependency build && helm template` succeeds for all of `1.26.8, 1.27.9, 1.28.10, 1.29.10, 1.30.8`
   (all three deps at the same version) and `helm unittest` passes (2/2) at `1.30.8`. So the values
   still parse; this does **not** prove behaviour. Diffs seen in the rendered output:
   - `istiod` `PodDisruptionBudget` is rendered through 1.27.9 but **gone at 1.28.10** — the 1.28 chart
     template only creates it when `replicaCount > 1` (or `autoscaleMin > 1`), and we run one
     replica. ArgoCD (`prune: true`) will delete it in stage 3; the gateway PDB (local template) is
     unaffected. Intentional upstream change, benign for a single replica.
   - A new `ConfigMap/values` appears in `istio-system` from 1.26.8 onward.
   - `1.30.x` adds one CRD: `trafficextensions.extensions.istio.io` (14 → 15 CRDs).
   - Keys in our `values.yaml` to re-validate per stage against upstream release notes:
     `istiod.telemetry.v2.prometheus` (legacy values), `istiod.autoscaleEnabled`, `istiod.profile`,
     `meshConfig.defaultConfig.proxyStatsMatcher`, `meshConfig.enablePrometheusMerge`.

## Strategy

One PR per minor, **all three dependencies at the same version, `appVersion` set to it**, merged one
at a time with a soak and the verification gate below before opening the next. Five PRs:

| Stage | `base` / `istiod` / `gateway` | `appVersion` | Notes |
|---|---|---|---|
| 1 | `1.26.8` | `1.26.8` | Also realigns gateway (`1.28.0` → `1.26.8` is a chart *downgrade*; Deployment template changes only, proxy image stays injector-driven). Biggest diff — watch the gateway rollout. |
| 2 | `1.27.9` | `1.27.9` | |
| 3 | `1.28.10` | `1.28.10` | |
| 4 | `1.29.10` | `1.29.10` | |
| 5 | `1.30.8` | `1.30.8` | Adds `TrafficExtension` CRD. Supersedes #473 — close #473 once this merges. |

Why not split istiod and gateway into separate PRs per minor: with `image: auto` the gateway proxy is
chosen by istiod at pod creation, so even if the gateway Deployment rolls before the new istiod is
Ready it just gets an N−1 proxy against an N control plane, which is a supported skew. Only split if
the preflight shows the gateway image is *not* injector-driven.

### Per-stage procedure

1. Branch from `origin/main`: `chore/istio-<minor>`.
2. Edit `services/helm/istio/Chart.yaml`: three dependency versions + `appVersion`.
3. `./ct_check.sh --dir services/helm/istio` (runs dependency build, lint, template). Run `helm-docs`
   so `README.md` badge/requirements match. Chart `.tgz` deps are gitignored.
4. Read the upgrade notes for that minor and adjust `values.yaml` in the same PR if a key is removed
   or renamed: `https://istio.io/latest/news/releases/<minor>.x/announcing-<minor>/upgrade-notes/`
   (e.g. `1.27.x/announcing-1.27`).
5. PR body: opener states the *why* (stage N of 5 of the 1.25→1.30 path, link this doc), bullets for
   the version moves. Merge, let ArgoCD sync `istio` (automated, `prune: true`, `selfHeal: true` from
   `services/argocd-appset/templates/_helpers.tpl`), then run the gate.
6. Do not start stage N+1 until stage N passes the gate and has soaked at least one ArgoCD
   self-heal cycle (suggest ≥ 1 day per stage).

### Preflight (once, before stage 1 — needs cluster access)

```bash
# 1. Is the gateway proxy injector-driven (i.e. what version is actually running)?
kubectl -n istio-system get deploy istio-ingressgateway -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'   # expect "auto" in the spec
kubectl -n istio-system get pod -l app=istio-ingressgateway -o jsonpath='{.items[*].spec.containers[0].image}{"\n"}'     # expect 1.25.x proxyv2
# 2. Control plane / proxy versions and sync state
istioctl version
istioctl proxy-status
istioctl x precheck
# 3. Backup of everything stage N could touch (store outside the repo)
kubectl get crd -o yaml | grep -c istio.io     # sanity
kubectl get gateways,virtualservices,destinationrules,authorizationpolicies,peerauthentications,requestauthentications,envoyfilters,serviceentries,sidecars,telemetries -A -o yaml > istio-resources-backup.yaml
kubectl get crd -o jsonpath='{range .items[?(@.spec.group=="networking.istio.io")]}{.metadata.name}{" "}{.status.storedVersions}{"\n"}{end}'
```

### Verification gate (after every stage syncs)

- `argocd app get istio` → `Synced` / `Healthy`; no `OutOfSync` residue beyond the already-ignored
  webhook fields in `ignoreDifferences`.
- `kubectl -n istio-system rollout status deploy/istiod deploy/istio-ingressgateway`
- `istioctl version` shows control plane = stage version; `istioctl proxy-status` shows the gateway
  `SYNCED` on all xDS types.
- `kubectl get validatingwebhookconfiguration,mutatingwebhookconfiguration | grep istio` present and
  `istioctl analyze -A` reports no new errors.
- Edge smoke test: one public host answers through the tunnel/gateway; one `enablePrivate` host
  answers `302 → jmaklab.cloudflareaccess.com` without a token (proves `RequestAuthentication` +
  `AuthorizationPolicy` still bind to the gateway); `certificate wildcard-maklab-net` still `Ready`.
- Workloads still served by pre-existing sidecars/ambient labels keep working (no restarts forced).

### Rollback

- Revert the stage PR and let ArgoCD resync; istiod rolls back with the chart. Rolling back across a
  minor is **not** an Istio-tested path, so treat the stage-1 backup and the storedVersions check as
  the safety net rather than as a guarantee. CRDs are additive across these minors; do not delete any.
- If the gateway is wedged, scale `istio-ingressgateway` and re-sync; the PDB (`maxUnavailable: 1`) is
  a local template and survives every stage.

## Safe now vs needs staged cluster rollout

| Safe now (this PR) | Needs the staged rollout |
|---|---|
| This plan; no chart/values/appset change | Every version bump above, in order |
| Pin git in devbox → PR #456 (independent, see below) | `appVersion` bumps (paired with the version they describe) |
| | Realigning gateway `1.28.0` → stage version |
| | Any `values.yaml` change forced by upgrade notes |

Optional follow-up (not in this PR, untested here): once stage 5 lands, add a Renovate rule so the
three charts group into one PR and stop proposing multi-minor jumps, e.g.
`{"matchManagers":["helmv3"],"matchPackageNames":["base","istiod","gateway"],"matchDatasources":["helm"],"groupName":"istio","separateMinorPatch":true,"separateMultipleMinor":true}`
(validate with `renovate-config-validator` before use).

## Related: the other half of the dashboard's "Open" list

`devbox.json` `git@latest` → `git@2.56.0` is Renovate PR **#456**. It is already complete: it changes
`devbox.json` and regenerates `devbox.lock` (`git@2.56.0`, nixpkgs `e126e9e…`, resolved 2026-11-13),
mergeable, `lint-test` green. Merge it directly; no separate branch is needed. (Separate observation:
the lock resolves `helm@latest` to `0.9.0` for linux — that is nixpkgs' `helm` audio synth, not
Kubernetes Helm, whose attr is `kubernetes-helm`. Worth its own fix; out of scope here.)
