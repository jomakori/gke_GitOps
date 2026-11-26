# strands staging

One standing staging host, `staging-strands.maklab.net`, and a splash page that answers that
host whenever nothing is pinned to it. There is no per-PR host and no per-PR namespace: the PR
that carries the `staging` label **is** the content of the host, and closing it hands the host
back to the splash.

**Scope.** This record covers the GitOps side (pin, render, splash) plus the label→deploy
workflow in `jomakori/strands`. Runtime configuration (database URL, connector keys, demo mode)
comes from Doppler, not from this repo.

## As built

| Piece | Where |
|---|---|
| Staging spec — subdomain `staging-strands`, placeholder tag `0.0.0` | `apps/helm/values.yaml` → `strands.environments.staging` |
| Render gate, `enable: false` at rest | `apps/argocd-appset/values.yaml` → `strands.environment.staging.enable` |
| Gate and Doppler configs injected into the umbrella | same file → `apps.strands.parameters` (`strands.enable_staging`, `strands.*.dopplerConfig`) |
| Application `strands` — sync wave 6, selfHeal, public gateway flag for production only | same file → `apps.strands` |
| Secrets — Doppler `apps/strands_stg` → ClusterSecretStore `doppler-strands-stg` → ExternalSecret `strands-staging-vars` | `services/helm/external-secrets/values.yaml` (`strands-stg`), `apps/helm/templates/_helpers.tpl` |
| Namespace `strands-staging` (ambient) | rendered from the `environment` block by `apps/argocd-appset/templates/namespaces.yaml` |
| Host gate — Cloudflare Access, no app accounts | `services/helm/istio/values.yaml` → `privateHosts` (`staging-strands.maklab.net`) |
| Image `ghcr.io/jomakori/strands:pr-<N>`, `linux/arm64` | `jomakori/strands/.github/workflows/staging.yml` |
| Splash chart and Application (wave 3, ns `staging-splash`) | `services/helm/staging-splash/`, `services/argocd-appset/values.yaml` → `stagingSplash` |
| Pin / unpin automation | `jomakori/strands/.github/workflows/staging.yml` |

Production is the public host `strands.maklab.net` and runs the app's own Google sign-in; the
staging host has no accounts, so it is gated by Cloudflare Access like the other staging hosts.

## How a PR gets staging

1. Open the PR in `jomakori/strands`. Only same-repo heads qualify — the workflow guards on
   `github.event.pull_request.head.repo.full_name == github.repository`, so fork PRs are skipped
   (no build, no pin).
2. A maintainer adds the `staging` label. The deploy trigger is the `labeled` event; a label
   that is already present when the PR opens never fires it, so add the label after the PR
   exists.
3. **Build** (only with the label, only while the PR is not closed): builds
   `linux/arm64` image `ghcr.io/jomakori/strands:pr-<N>` from the PR merge commit with
   `GIT_COMMIT=${{ github.sha }}`, pushes it, and gates on `docker manifest inspect`.
4. **Deploy** (`needs: build`, same-repo guard, serialized by
   `concurrency: {group: staging-deploy, cancel-in-progress: false}`): checks out
   `jomakori/gke_GitOps` (depth 1, `GITOPS_TOKEN`), rewrites the two pin lines, commits
   `chore(strands): staging pr-<N>`, and pushes to `main` — up to five attempts, each rebasing
   onto `FETCH_HEAD`.
5. ArgoCD picks up the new `main` commit and syncs Application `strands`. With the gate on, the
   umbrella renders the staging Deployment, Service, VirtualService for
   `staging-strands.maklab.net`, ExternalSecret, ServiceAccount and NetworkPolicy into
   `strands-staging`.
6. The host now has an exact-host VirtualService, which outranks the splash wildcard — the app
   serves it.

The pin commit lands on `main` directly, not through a PR: the workflow is the writer, and the
commit message is the audit trail.

## What the pin changes

Exactly two lines, in one commit:

| File | Key | Unpinned | Pinned |
|---|---|---|---|
| `apps/helm/values.yaml` | `strands.environments.staging.tag` | `"0.0.0"` | `"pr-<N>"` |
| `apps/argocd-appset/values.yaml` | `strands.environment.staging.enable` | `false` | `true` |

`tag` chooses the image. `enable` is the render gate: the appset passes it to the chart as the
`strands.enable_staging` parameter, and the umbrella renders staging objects only when that
parameter is not `false`. Neither value is a release — `pr-<N>` never collides with a semver tag,
and `0.0.0` was never pushed, so enabling without a tag fails the image pull instead of silently
serving a released build.

The flag is the switch, not the namespace: `strands-staging` exists either way (the namespace
list walks the `environment` block), it is simply empty while staging is off.

## How to unpin

Automatic, by the same workflow:

- Removing the label (`unlabeled`) or closing/merging the PR (`closed`) runs the deploy job with
  the build skipped, which takes its unpin branch: `tag: "0.0.0"`, `enable: false`, commit
  `chore(strands): unpin staging`, push.
- Ownership guard: an unpin only lands if the current tag is the PR's own. A late `unlabeled`
  from a PR that no longer owns the host prints `staging runs pr-<other>, not pr-<N>: another
  pull request owns it` and exits without committing. Newest label wins, and the serialized
  concurrency group keeps two label events from interleaving.

Manual (workflow broken, or a pin applied by hand) — both keys in the same commit:

```bash
cd gke_GitOps
yq -i '.strands.environments.staging.tag = "0.0.0"' apps/helm/values.yaml
yq -i '.strands.environment.staging.enable = false' apps/argocd-appset/values.yaml
git commit -am "chore(strands): unpin staging"
git push
```

A tag change without the flag leaves the old Deployment in place; the flag without the tag asks
for `0.0.0`.

## The splash page

### How it is served

`services/helm/staging-splash/` runs a single `nginx-unprivileged` Deployment (`staging-splash`,
namespace `staging-splash`) behind a catch-all VirtualService whose host is `*.maklab.net` on
`istio-system/maklab-gateway` (`templates/virtualservice.yaml`). Istio ranks an exact host above
a wildcard, so every host that has its own app VirtualService keeps serving the app, and only
hosts with nothing deployed reach the splash.

The app list arrives through the overlay: the registry entry
(`services/argocd-appset/values.yaml` → `stagingSplash`) loads the chart's `values.yaml` and then
`../../../apps/helm/values.yaml`, so `templates/_helpers.tpl` → `staging-splash.apps` reads the
same specs the umbrella does. The host rule (`staging-splash.hosts`) is: a non-empty
`environments.staging.subdomain` **and** either `enable_staging` not `false` or a `splash.page`
declaration of `custom` or `pr-gated`. Each such host gets its own `server_name` block in the
generated `nginx.conf`; everything else lands on the `default_server` block. `__APP__` in a page is
replaced from the request host, so `staging-openkite.maklab.net` renders `openkite`.

Pages live in `pages/`: `pr-gated.html` is one shared copy whose only per-app difference is the
link from `splash.prsUrl`; `splash.page: custom` selects `pages/<key>.html`; everything else gets
`generic.html`. A `custom` or `pr-gated` declaration whose file is missing fails the render, so a
themed app can never silently fall back to the generic page.

### The strands page

`apps/helm/values.yaml` → `strands.splash.page: custom` selects
`services/helm/staging-splash/pages/strands.html`. The page is self-contained — no image or web
font request: oklch tokens on a dark `oklch(0.16 0.02 265)` ground, warm-yellow accent
`oklch(0.86 0.14 95)`, Space Grotesk display and Inter body stacks with system fallbacks, a 20px
card radius, and the app's night-sky speckles drawn as CSS gradients; a
`prefers-color-scheme: light` variant is included. Copy: the `strands` wordmark with a `staging`
badge, `Staging environment is not up.`, `Add the staging label to deploy this env.` (the words
"staging label" link to `https://github.com/jomakori/strands/pulls?q=is%3Apr+is%3Aopen+label%3Astaging`),
and `Closing the pull request takes it down again.`

### The unpinned host

The host rule used to skip apps whose spec sets `enable_staging: false`, and strands' spec is
`false` exactly while nothing is pinned — so the render emitted **no** `staging-strands` server
block and the host fell through to the catch-all `generic.html`: `pages/strands.html` sat in the
ConfigMap unserved. Fixed by making a `splash.page` declaration count as participation: a themed
page is what the host shows while nothing is pinned, and the pin's exact-host VirtualService
outranks the splash wildcard anyway, so the block is only reachable while staging is off. The
render now lists `staging-openkite.maklab.net`, `staging-strands.maklab.net` and
`staging-trade.maklab.net`; `services/helm/staging-splash/tests/staging_splash_test.yaml` asserts
the strands block with `enable_staging: false`.

## Verification

Assumes a kubeconfig for the lab cluster.

Pin written (right after the workflow push):

```bash
git -C gke_GitOps fetch origin main
git -C gke_GitOps log -1 --format=%s origin/main          # chore(strands): staging pr-<N>
yq '.strands.environments.staging.tag' apps/helm/values.yaml
yq '.strands.environment.staging.enable' apps/argocd-appset/values.yaml
```

Render check, no cluster needed — staging objects appear only with the gate on:

```bash
helm template strands apps/helm --set appName=strands --set strands.enable_staging=true \
  --set strands.environments.staging.tag=pr-<N> | grep -E "kind: (Deployment|VirtualService)"
```

Cluster side:

```bash
kubectl -n argocd get application strands -o jsonpath='{.status.sync.status} {.status.health.status}{"\n"}'
kubectl -n strands-staging get deploy,svc,vs,pods
kubectl -n strands-staging get deploy strands-staging -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
kubectl -n strands-staging get externalsecret strands-staging-vars -o jsonpath='{.status.conditions[0].reason}{"\n"}'
```

The image must end in `:pr-<N>`; an older tag means ArgoCD has not rendered the new revision yet —
`argocd app get strands` shows which commit it synced.

Served:

```bash
curl -sS -o /dev/null -w '%{http_code}\n' https://staging-strands.maklab.net/
curl -sS https://staging-strands.maklab.net/healthz | jq -r '.commit'
```

`/healthz` answers without touching Postgres (`crates/server/src/app.rs`) and returns
`{status, version, commit}`; `commit` is the PR merge commit the image was built from.

Unpinned:

```bash
git -C gke_GitOps log -1 --format=%s origin/main          # chore(strands): unpin staging
kubectl -n strands-staging get vs                         # No resources found — the app VS is pruned
curl -sS https://staging-strands.maklab.net/ | grep -o '<h1>[^<]*'
```

That last line reads `<h1>Staging environment is not up.` from `strands.html` — `generic.html`
would answer `No <span>staging-strands</span> environment is running`, which means the host block
is missing.

The splash itself:

```bash
kubectl -n argocd get application staging-splash
kubectl -n staging-splash get deploy,svc,vs
kubectl -n staging-splash get cm staging-splash -o jsonpath='{.data.nginx\.conf}' | grep server_name
kubectl -n staging-splash port-forward svc/staging-splash 8080:80 &
curl -sS -H 'Host: staging-strands.maklab.net' localhost:8080/ | grep -o '<h1>[^<]*'
curl -sS -H 'Host: staging-strands.maklab.net' localhost:8080/healthz   # ok
```

## Limits

- `staging-strands.maklab.net` sits behind Cloudflare Access (`privateHosts`), so an
  unauthenticated `curl` from outside gets the Access challenge, not the app. Run the curl checks
  from the tailnet, from a pod, or with an Access service token.
- The pin lands only if `GITOPS_TOKEN` is valid; an expired token leaves the PR built and the host
  unchanged, visible as failed pushes in the workflow log.
- `__APP__` is substituted from the request host at serve time, so a request that reaches the
  splash with an unrelated host renders `generic.html` labelled with that host's first label.
