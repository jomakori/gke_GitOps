# Services

GitOps configuration for **third-party software we host** — chart source plus the registry ArgoCD reads. First-party workloads we build or test live in [`../apps/`](../apps/).

## Structure

Follows the ArgoCD [App-of-Apps](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#app-of-apps) pattern.

```text
services/
├── argocd-appset/                    ← registry + the template that renders Applications from it
│   ├── templates/_helpers.tpl
│   ├── templates/applications.yaml   ← one Application per registry entry
│   └── values.yaml                   ← the registry (enablement, wave, parameters)
└── helm/                             ← one chart per service
```

### argocd-appset

`applications.yaml` renders one `Application` per entry in `values.yaml`, with the shared logic in `_helpers.tpl`. Values injected by Terraform (`clusterDomain`, `repoUrl`, `targetRevision`, `argoProject`, `argoNamespace`, `storageClass`) are inherited by every entry. Enablement, wave and per-service parameters live in the registry and nowhere else.

### helm

Pick the chart source in this order:

1. **The project ships an official chart** → depend on it (thin wrapper), adding local templates only for what it lacks (hybrid).
2. **No official chart, but a community chart exists** → **wrap it as a values-override layer.** This is the default for services with no official chart — `excalidash` is the reference (it wraps the chart published at `https://charts.alekc.dev`).
3. **No chart anywhere** → custom chart, local templates only.

| Pattern | When | Shape |
|---|---|---|
| Thin wrapper | official chart exists | `Chart.yaml` with the upstream `dependencies` only — no local templates |
| **Values-override layer** *(default)* | no official chart, community chart exists | upstream dependency + `values.yaml` overrides under the upstream chart's key + local `templates/` for what upstream lacks |
| Hybrid | official chart exists, extras needed | upstream dependency **plus** local templates (ExternalSecrets, ClusterSecretStores, database clusters, …) |
| Custom | nothing upstream | local templates only |

#### Values-override layer recipe

```yaml
# helm/<service>/Chart.yaml
dependencies:
  - name: <upstream-chart>
    version: <pinned>                     # pin it — Renovate bumps it
    repository: https://<chart-repo>      # e.g. https://charts.alekc.dev
```

```yaml
# helm/<service>/values.yaml
dopplerConfig: svc_<service>              # consumed by templates/externalsecret.yaml

<upstream-chart>:                         # overrides live under the upstream chart's key
  ingress:
    main:
      enabled: false                      # routing is the Istio VirtualService, from the registry entry
  persistence:
    enabled: true
    size: 5Gi
    storageClass: local-path
```

Rules for this pattern:

- **Pin the upstream version** as a dependency so dependency PRs track it; never vendor a fork.
- **Override values, don't fork templates.** Add local templates only for what upstream genuinely lacks — typically the `ExternalSecret`/`dopplerConfig` wiring.
- **Disable the upstream's own ingress.** Exposure is decided by the registry entry (`gateways:`) and rendered by the istio chart.
- **Chart-managed secrets are a fallback, not the source of truth.** Let upstream generate boot-time values if it needs them; the Doppler `ExternalSecret` (`creationPolicy: Merge`) overwrites with the real ones.
- **Vendored dependencies are gitignored**, so CI and `ct_check.sh` run `helm dependency build` — do the same locally before rendering.

A chart can exist in `helm/` without being registered — the registry, not this directory, decides what is deployed.

## Adding a service

1. **Create the chart** under `helm/<name>/`, using whichever pattern above fits.
2. **Register it** in `argocd-appset/values.yaml`: enablement, `syncWave` (see the wave tiers in the [root README](../README.md#how-the-loop-works)), namespace and any parameters.
3. **Wire secrets** with a `dopplerConfig` key — the ExternalSecret template pulls the whole Doppler config, so no Terraform change is needed.
4. **Expose it** (optional) with `gateways.enable_public: true`; the template derives host, destination and VirtualService name from `clusterDomain` and the entry, and `subdomain` / `destination.*` override the defaults.
5. **Validate locally** from the repo root: `./ct_check.sh --dir services/helm/<name>`.
6. **PR and merge** — ArgoCD syncs it.
