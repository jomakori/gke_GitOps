# openkite-preview

Per-PR preview infrastructure for [jomakori/openkite](https://github.com/jomakori/openkite).
One ArgoCD `ApplicationSet` (GitHub **Pull Request generator**) renders one preview
`Application` per **labelled** open PR, deploying `apps/helm/openkite-preview` into
that PR's OWN namespace at `pr-<num>.openkite.maklab.net`. When a PR closes, merges
or loses the label, the generated Application is deleted and its
`resources-finalizer` cascades cleanup of the namespace and everything in it.
Nothing is ever written back to the repo.

## Layout

```
apps/openkite-preview/
├── Chart.yaml
├── values.yaml
├── templates/
│   ├── _helpers.tpl
│   ├── namespaces.yaml        ← shared preview namespace (ambient)
│   ├── certificate.yaml       ← cert-manager wildcard *.openkite.maklab.net
│   ├── gateway.yaml           ← Istio Gateway serving the 2-level wildcard
│   └── applicationset.yaml    ← PR-generator ApplicationSet
└── tests/
    └── applicationset_test.yaml
```

The per-PR workload chart lives at `apps/helm/openkite-preview/`, following the
repo convention for app charts (`apps/helm/<name>/`).

## How it registers with the root app-of-apps

`apps/openkite-preview` is registered as an entry in the `apps` app-of-apps
(`apps/argocd-appset/values.yaml`), the same registry that holds every other
application. The app-of-apps renders an ArgoCD `Application` whose source path is
`apps/openkite-preview` and injects the globals this chart expects
(`argoNamespace`, `argoProject`, `clusterDomain`, `clusterName`, `clusterServer`,
`repoUrl`, `storageClass`, `targetRevision`) from its own values contract — the
same globals the app-of-apps itself receives from `devops_Terraform`.

The ApplicationSet it renders then owns the preview `Application`s directly.

## Data flow

```
GitHub PR (jomakori/openkite)
  → pr-image.yml builds + pushes ghcr.io/jomakori/openkite:pr-<N>   (OKT-74)
      └─ on success only: applies the `preview` label to the PR
  → ApplicationSet pullRequest generator (token: argocd-github-token, OKT-76)
      └─ filters on that label — unlabelled PRs are skipped entirely
    → Application openkite-preview-<N>   (helm: apps/helm/openkite-preview)
      image:     ghcr.io/jomakori/openkite:pr-<N>
      host:      pr-<N>.openkite.maklab.net      (Gateway + Certificate, this chart)
      namespace: openkite-preview-<N>            (its own, pruned with the preview)
```

The shared `openkite-preview` namespace keeps only what must exist exactly once and
outlive every PR: the wildcard TLS Secret, the Istio Gateway and the generator's
token.

## The image gate

A preview is only deployed for a PR whose image was built and published
successfully. `previewLabel` (default `preview`) is the whole switch: the
generator's `github.labels` filter admits a PR only when it carries the label, and
[jomakori/openkite](https://github.com/jomakori/openkite)'s
`.github/workflows/pr-image.yml` is the only thing that applies it — after the
`docker push`, never on a failed build, and it withdraws the label when a later
head's build fails so a stale preview is torn down.

Consequences worth knowing:

- `pr-image.yml` runs on every PR, so every PR publishes `pr-<N>` and every PR is
  eligible for a preview. The gate is about *proof*, not about filtering PRs: a
  build that fails withdraws/withholds the label, so the failure surfaces as a red
  check on the PR instead of a deployed preview stuck in `ImagePullBackOff` — the
  state PRs 132/133 were left in, when the workflow's `paths` filter skipped them
  entirely and the then-unfiltered generator deployed them anyway.
- The filter is client-side over the open-PR list
  (`applicationset/services/pull_request/github.go` → `containLabels`): every
  label listed is required. Unlabelling a PR removes its Application on the next
  reconcile (`requeueAfterSeconds`), and the `resources-finalizer` prunes the
  namespace, Deployment, Service and VirtualService with it.
- Teardown lag is bounded by `requeueAfterSeconds` (60s) plus the prune. There is
  no second teardown mechanism: closing a PR and unlabelling one take the same
  path.
- A label applied by hand bypasses the gate. The gate is only as strong as the
  workflow being the one thing that sets the label.

## Secrets

The generator authenticates with the repo-scoped GitHub token provisioned by
**OKT-76** as an `ExternalSecret` named `argocd-github-token` (key `token`) in
the `argocd` namespace. This chart only references it — no secret value is ever
committed here.

## Domain and TLS

`pr-<num>.openkite.maklab.net` is a **two-level** subdomain. The cluster's
`*.maklab.net` wildcard matches only one label, so it does **not** cover preview
hosts. This chart therefore issues a dedicated cert-manager `Certificate` for
`*.openkite.maklab.net` and serves it from a dedicated Istio `Gateway`
(`openkite-preview-gateway`). ExternalDNS watches the Gateway host and creates
the wildcard record in Cloudflare.

## Lifecycle

| Event | Result |
|-------|--------|
| PR opened / updated, image published | workflow labels the PR → generator adds it → Application created/updated → preview deployed |
| PR updated, image build fails | workflow withdraws the label → generator drops the PR → Application deleted → finalizer prunes resources |
| PR closed / merged | generator drops the PR → Application deleted → finalizer prunes namespace + Deployment + Service + VirtualService |
| PR never built an image | no label → generator never adds it → nothing is deployed |

`spec.syncPolicy.preserveResourcesOnDeletion: false` plus the
`resources-finalizer.argocd.argoproj.io` finalizer on each generated Application
are what make cleanup automatic.

## Cleanup

Nothing to run in the normal case: closing or merging the PR (or unlabelling it)
drops the generated Application on the next reconcile and the finalizer prunes the
namespace with it.

To remove a preview immediately — or to clear the previews that predate this
lifecycle, which were generated into the shared namespace by the unfiltered spec
and have no per-PR namespace behind them — delete the Application and let the
finalizer do the rest:

```bash
# All three pre-lifecycle previews (PRs 131, 132 and 133):
kubectl -n argocd delete application openkite-preview-131 openkite-preview-132 openkite-preview-133

# One PR, generally:
kubectl -n argocd delete application openkite-preview-<N>
```

Do NOT delete the `openkite-preview` namespace with them: it holds the wildcard
Certificate, the Gateway and the generator token, and is owned by this chart's
Application, so ArgoCD would recreate it. Deleting the Applications removes only
what each preview owns.

## Values

| Key | Description |
|-----|-------------|
| `appName` | ApplicationSet name (default `openkite-preview`) |
| `namespace` | shared preview INFRASTRUCTURE namespace — wildcard Certificate, Gateway, token (default `openkite-preview`) |
| `namespaceTemplate` | destination namespace per PR (default `openkite-preview-{{ .number }}`) |
| `github.owner` / `github.repo` | PR source repository (`jomakori/openkite`) |
| `github.tokenSecretName` / `github.tokenSecretKey` | `argocd-github-token` / `token` |
| `previewLabel` | the only label that makes a PR eligible for a preview (default `preview`) |
| `chartPath` | per-PR chart (`apps/helm/openkite-preview`) |
| `previewDomain` | `openkite.maklab.net` |
| `nameTemplate` / `hostTemplate` / `tagTemplate` | goTemplate strings (`.number` = PR id) |
| `image.repository` | `ghcr.io/jomakori/openkite` |
| `replicaCount` | preview replicas |
| `requeueAfterSeconds` | PR poll interval |
| `tls.*` | wildcard Certificate name / secret / issuer |
| `gateway.*` | shared Gateway name and ingress selector |

## Validate

```bash
helm template apps/openkite-preview \
  --set repoUrl=https://github.com/jomakori/gke_GitOps.git
helm unittest apps/openkite-preview
```
