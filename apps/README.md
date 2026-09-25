# Apps

GitOps configuration for **first-party workloads we build or test** — chart source plus the registry ArgoCD reads. Third-party software we merely host belongs in [`../services/`](../services/).

## Structure

Follows the ArgoCD [App-of-Apps](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#app-of-apps) pattern.

```text
apps/
├── argocd-appset/          ← registry + template (+ namespaces)
│   ├── templates/applications.yaml
│   ├── templates/namespaces.yaml
│   └── values.yaml         ← the registry (enablement, environments, chart path)
├── helm/                   ← the parameterized app chart, plus a chart per workload that needs its own
└── openkite-preview/       ← per-PR preview ApplicationSet
```

### argocd-appset

One template renders an `Application` per entry in `values.yaml`. An entry names the chart it deploys (`helmPath`) along with its environments and `dopplerConfig`, so the parameterized chart and a workload-specific chart register identically.

### helm

`apps/helm/` holds the parameterized chart (`name: apps`): a single `_helpers.tpl` define emits every resource, driven by the entry's flags.

| Resource | Created when |
|---|---|
| ServiceAccount + image pull Secret | always |
| ClusterRole + ClusterRoleBinding (read-only) | `cluster_read.enabled` (and never for a preview) |
| ExternalSecret | the environment sets `dopplerConfig` |
| Deployment | always |
| Service | always (ClusterIP for the mesh; NodePort fallback) |
| VirtualService | `enable_domain` + `enable_istio` |
| HPA | `enable_scaling` |
| PVC | `storage.size` is set |

Multi-environment (staging + production) is supported per app. A workload that needs more than this shape keeps its own chart under `apps/helm/<name>/`, named by the entry's `helmPath`.

### openkite-preview

A per-PR preview system registered like any other app. It renders an `ApplicationSet` using the GitHub Pull Request generator to create one preview `Application` per open PR on `jomakori/openkite` **that carries the `preview` label** (applied by that repo's image workflow only once the `pr-<N>` image is published), at `pr-<num>.<clusterDomain>`, and deletes it when the PR closes or loses the label. It also owns the wildcard TLS `Certificate` and Istio `Gateway` for that host, and consumes the GitHub token ExternalSecret. See [openkite-preview/README.md](openkite-preview/README.md).

## Adding an app

1. **Add an entry** to `argocd-appset/values.yaml` — app key, environments, `dopplerConfig`, and the chart to deploy.
2. **Add a namespace** in `argocd-appset/templates/namespaces.yaml`.
3. **Validate locally** from the repo root: `./ct_check.sh --dir <chart>`, then **PR and merge** — ArgoCD syncs it. Enablement is a registry change, not a code change.
