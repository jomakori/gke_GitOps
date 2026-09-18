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

Charts follow one of three patterns:

| Pattern | Shape |
|---|---|
| Thin wrapper | `Chart.yaml` with upstream `dependencies` only — no local templates |
| Hybrid | upstream dependency **plus** local templates for the extras (ExternalSecrets, ClusterSecretStores, database clusters, …) |
| Custom | local templates only, no upstream dependency |

A chart can exist in `helm/` without being registered — the registry, not this directory, decides what is deployed.

## Adding a service

1. **Create the chart** under `helm/<name>/`, using whichever pattern above fits.
2. **Register it** in `argocd-appset/values.yaml`: enablement, `syncWave` (see the wave tiers in the [root README](../README.md#how-the-loop-works)), namespace and any parameters.
3. **Wire secrets** with a `dopplerConfig` key — the ExternalSecret template pulls the whole Doppler config, so no Terraform change is needed.
4. **Expose it** (optional) with `gateways.enable_public: true`; the template derives host, destination and VirtualService name from `clusterDomain` and the entry, and `subdomain` / `destination.*` override the defaults.
5. **Validate locally** from the repo root: `./ct_check.sh --dir services/helm/<name>`.
6. **PR and merge** — ArgoCD syncs it.
