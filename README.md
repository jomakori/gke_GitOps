<div align="center">

<img src="docs/assets/banner.svg" alt="gke_GitOps" width="60%">

<em>Declarative delivery for our Kubernetes clusters — merge to deploy.</em>

[![CI](https://img.shields.io/github/actions/workflow/status/jomakori/gke_GitOps/helm_lint-test.yaml?label=CI&logo=githubactions&logoColor=white&color=4169E1)](https://github.com/jomakori/gke_GitOps/actions/workflows/helm_lint-test.yaml)
[![Argo CD](https://img.shields.io/badge/Argo%20CD-4169E1?style=for-the-badge&logo=argo&logoColor=white)](https://argo-cd.readthedocs.io)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-8A2BE2?style=for-the-badge&logo=kubernetes&logoColor=white)](https://kubernetes.io)
[![Helm](https://img.shields.io/badge/Helm-4169E1?style=for-the-badge&logo=helm&logoColor=white)](https://helm.sh)
[![GitHub Actions](https://img.shields.io/badge/GitHub%20Actions-8A2BE2?style=for-the-badge&logo=githubactions&logoColor=white)](https://github.com/features/actions)
[![Terraform](https://img.shields.io/badge/Terraform-4169E1?style=for-the-badge&logo=terraform&logoColor=white)](https://developer.hashicorp.com/terraform)

</div>

<img src="docs/assets/line-gradient.svg" alt="divider" width="100%" height="3px">

<!--TOC-->

- [How the loop works](#how-the-loop-works)
- [Quickstart](#quickstart)
- [Layout](#layout)
- [Services](#services)
- [Apps](#apps)
- [Secrets](#secrets)
- [CI](#ci)
- [Troubleshooting](#troubleshooting)

<!--TOC-->

Terraform provisions the cluster and bootstraps ArgoCD; **everything after that bootstrap is owned here** — the services and apps, their ingress and secret wiring — and reconciled by ArgoCD from this repository rather than applied by hand. Merging is deploying: ArgoCD prunes and self-heals.

## How the loop works

```text
Terraform (devops_Terraform)
  ├─ provisions the cluster + ArgoCD
  └─ creates the root Application ─┬─▶ services/argocd-appset ──┐
                                   │                             ├─▶ one Application
                                   └─▶ apps/argocd-appset      ──┘     per registry entry
                                                                             │
                                                               ArgoCD applies in waves
                                                               (prune + self-heal)
```

- **Terraform owns the bootstrap** — the cluster, ArgoCD, and the root `Application` that points here. It also injects the values every entry inherits (`repoUrl`, `targetRevision`, `clusterDomain`, `argoProject`, `argoNamespace`).
- **App-of-Apps** — each appset is a chart whose single template renders one `Application` per entry in its `values.yaml`. That file **is the registry**: adding a workload is an entry, not a hand-written manifest.
- **Waves are dependency tiers**, applied in order, so the ordering rules hold by construction — secrets before consumers, an operator before its custom resources, a CRD before anything using it, shared platform before workloads:

  | Wave | Tier | Carries |
  |---|---|---|
  | 0 | Foundation + early AI platform | storage class, certificate manager, metrics server, VPA — and the AI platform, bootstrapped here so it is ready to serve before the operators of wave 4 arrive |
  | 1 | Secrets | external-secrets (ClusterSecretStores) |
  | 2 | Core networking | the service mesh (CRDs → control plane → ingress gateway) |
  | 3 | Edge ingress | the public tunnel, plus the AI platform's chat entry |
  | 4 | Operators & DNS | DNS automation, database operators |
  | 5 | Data, observability & platform | monitoring stack, data and cost services |
  | 6 | Consumer workloads | first-party apps |

- **Wiring is generated from the same entry** — `gateways:` produces the ingress objects and `dopplerConfig` produces the ExternalSecret, so a workload and its exposure are declared once.
- **Merging is deploying.** ArgoCD prunes and self-heals; nothing is applied imperatively.

## Quickstart

```bash
./ct_check.sh --dir services/helm/<chart>       # lint + template + dry-run, the way CI does it
.useful-scripts/validate_litellm_config.sh      # LLM routing config sanity (openagent values)
pre-commit install && pre-commit run --all-files  # the hooks CI also runs
```

Prerequisites: `kubectl`, `helm`, chart-testing (`ct`), `yamllint`.

Charts are validated locally and never applied by hand — merging is what deploys. For local access without exposing anything:

```bash
kubectl port-forward -n <namespace> svc/<service> 8080:80
```

## Layout

```text
.
├── services/           ← third-party software we host (charts + registry)
├── apps/               ← first-party workloads we build or test (charts + registry)
├── .useful-scripts/    ← validation and cluster helpers
├── .github/workflows/  ← CI: chart lint/test
├── ct_check.sh         ← chart lint/dry-run entrypoint
├── renovate.json       ← dependency updates
├── devbox.json         ← reproducible shell for image tag bumps
└── .pre-commit-config.yaml
```

[`services/README.md`](services/README.md) and [`apps/README.md`](apps/README.md) cover their registries, chart patterns and gateway/secret wiring in detail.

## Services

Third-party software we host, registered in [`services/argocd-appset/values.yaml`](services/argocd-appset/values.yaml). The registry — not this README — owns enablement, wave and parameters; its entries are grouped by the wave tiers above.

Each service chart takes its upstream from the project's official chart when one exists; where it does not, we wrap a maintained community chart as a values-override layer (see [`services/README.md`](services/README.md#helm)).

The AI platform has its own documentation: [`services/helm/openagent/README.md`](services/helm/openagent/README.md).

## Apps

First-party workloads we build or test, registered in [`apps/argocd-appset/values.yaml`](apps/argocd-appset/values.yaml). Most use the parameterized chart in `apps/helm/`; workloads needing their own resources carry their own chart — the registry entry names which, so both shapes register the same way, and the entry generates the app's gateway and Doppler wiring like any service.

## Secrets

Nothing sensitive lives in this repository.

1. Values live in **Doppler** (project + config pairs).
2. **Terraform** stores the ESO token as a Kubernetes Secret.
3. A **ClusterSecretStore** per Doppler config references that token.
4. Each workload's **ExternalSecret** pulls that whole config (`dataFrom.extract`) — Kubernetes key names match Doppler key names.
5. Pods consume via `envFrom` / `secretKeyRef`.

| Doppler config | Used by |
|---|---|
| `svc_grafana` | monitoring / Grafana |
| `svc_cloudflare` | service mesh ingress, DNS automation, the tunnel |
| `svc_postgres_operator` | the database operator |
| `svc_openagent` | the AI platform |

Add a new secret in Doppler; the ExternalSecret syncs the whole config on its refresh interval.

## CI

| Workflow | What it checks |
|---|---|
| `helm_lint-test` | chart lint + template dry-run on changed charts (via `ct_check.sh`), LiteLLM routing config validation when the openagent values change |

Runs on pull requests and on demand. Renovate opens dependency-update PRs. The same validations run locally via pre-commit, so a CI failure should be reproducible before pushing.

## Troubleshooting

- **A failed sync does not retry by itself** — ArgoCD spends the retry budget on the failure; re-sync or push a change.
- **A manual secret edit disappears** — ExternalSecrets own their keys; change the value in Doppler.
- **A resource stays OutOfSync after a sync** — something on the live object is owned outside the chart (operator-managed fields, an `ignoreDifferences` override that no longer matches). Fix the drift source; if the field is legitimately managed elsewhere, move it into the entry's `ignoreDifferences`.
- **Ingress does not route** — host matching is SNI-based; a host outside the Certificate/Gateway never matches, even when DNS resolves.
- **A custom resource fails to apply** — its operator or CRD sits in a later wave than the resource; move the entry up a tier.

Sibling repository: [devops_Terraform](https://github.com/jomakori/devops_Terraform) provisions the clusters and the ArgoCD bootstrap.
