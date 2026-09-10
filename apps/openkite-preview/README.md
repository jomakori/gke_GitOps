# openkite-preview

Per-PR preview infrastructure for [jomakori/openkite](https://github.com/jomakori/openkite).
One ArgoCD `ApplicationSet` (GitHub **Pull Request generator**) renders one preview
`Application` per open PR, deploying `apps/helm/openkite-preview` at
`pr-<num>.openkite.maklab.net`. When a PR closes, the generated Application is
deleted and its `resources-finalizer` cascades cleanup of every preview resource.
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

`apps/openkite-preview` is a **top-level app**, exactly like `apps/argocd-appset`.
Terraform (`devops_Terraform`) creates an ArgoCD `Application` whose source path
is `apps/openkite-preview` and injects the same globals that `apps/argocd-appset`
receives (`argoNamespace`, `argoProject`, `clusterServer`, `repoUrl`,
`targetRevision`, ...). This chart does **not** add an entry to the apps
app-of-apps values (that root chart is out of scope for this change).

The ApplicationSet it renders then owns the preview `Application`s directly.

## Data flow

```
GitHub PR (jomakori/openkite)
  → ApplicationSet pullRequest generator (token: argocd-github-token, OKT-76)
    → Application openkite-preview-<N>   (helm: apps/helm/openkite-preview)
      image: ghcr.io/jomakori/openkite:pr-<N>   (published by OKT-74)
      host:  pr-<N>.openkite.maklab.net          (Gateway + Certificate, this chart)
```

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
| PR opened / updated | generator adds the PR → Application created/updated → preview deployed |
| PR closed / merged | generator drops the PR → Application deleted → finalizer prunes all resources |

`spec.syncPolicy.preserveResourcesOnDeletion: false` plus the
`resources-finalizer.argocd.argoproj.io` finalizer on each generated Application
are what make cleanup automatic.

## Values

| Key | Description |
|-----|-------------|
| `appName` | ApplicationSet name (default `openkite-preview`) |
| `namespace` | shared preview namespace (default `openkite-preview`) |
| `github.owner` / `github.repo` | PR source repository (`jomakori/openkite`) |
| `github.tokenSecretName` / `github.tokenSecretKey` | `argocd-github-token` / `token` |
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
