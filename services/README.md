# Services

This folder contains GitOps configuration for **3rd-party infrastructure services** that the jmak-lab Minikube cluster depends on.

## Structure

Follows the ArgoCD [App-of-Apps](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#app-of-apps) pattern.

```
services/
├── argocd-appset/          ← ArgoCD Application manifests
│   ├── Chart.yaml
│   ├── templates/
│   │   ├── _helpers.tpl     ← Template helpers (service registration logic)
│   │   └── applications.yaml ← Single template auto-generates all Applications
│   └── values.yaml           ← Service registry (enable/disable, sync waves, parameters)
└── helm/                   ← Helm chart source for each service (17 charts)
    ├── cert-manager/        ← Thin wrapper
    ├── cloudflare-tunnel/   ← Custom
    ├── external-dns/        ← Hybrid
    ├── external-secrets/    ← Hybrid
    ├── headlamp/            ← Hybrid (upstream + user SA templates)
    ├── istio/               ← Hybrid
    ├── keda/                ← Thin wrapper (disabled)
    ├── kube-prometheus-stack/ ← Hybrid
    ├── local-path/          ← Thin wrapper
    ├── metrics-server/      ← Thin wrapper
    ├── mongodb-operator/    ← Thin wrapper (disabled)
    ├── onedev/              ← Custom
    ├── openagent/           ← Custom umbrella (LiteLLM + headroom + discord bot + Sympozium CRDs in single chart)
    ├── opencost/            ← Thin wrapper
    ├── postgres-operator/   ← Hybrid
    ├── redis-operator/      ← Thin wrapper (disabled)
    └── vpa/                 ← Thin wrapper
```

### argocd-appset

A single `applications.yaml` template auto-generates all ArgoCD `Application` resources from `values.yaml`, using `_helpers.tpl` for logic. Services are toggled on/off via `values.yaml` — parameters like `clusterDomain` are injected by Terraform and propagated through ArgoCD appset values.

### helm

Charts fall into three patterns:

| Pattern | Count | Description |
|---------|-------|-------------|
| **Thin Wrapper** | 8 | `Chart.yaml` with upstream `dependencies` only, no local templates |
| **Custom** | 3 | Full local templates, no upstream dependency |
| **Hybrid** | 6 | Upstream dependency + local templates for extra resources (ExternalSecrets, ClusterSecretStores, SGCluster, etc.) |

## Adding a Service

1. **Create the Helm chart** under `helm/<service-name>/` — thin wrapper (upstream dep), hybrid (upstream + local templates), or custom (full templates).
2. **Register it** in `argocd-appset/values.yaml` with an `enable: true/false` flag, syncWave, destNamespace, and any parameters.
3. **Wire secrets** via ESO: add a `dopplerConfig` key in the values entry. No Terraform changes needed — the ExternalSecret template pulls the entire config from Doppler.
4. **If public ingress is needed**, set `gateways.enable_public: true` — the `applications.yaml` template auto-generates a VirtualService via the istio umbrella chart. For custom subdomains or non-default service names:

   ```yaml
   gateways:
     enable_public: true       # required
     subdomain: my-app          # optional — defaults to chart name
     destination:
       serviceName: my-svc      # optional — defaults to chart name
       servicePort: 8080        # optional — defaults to 80
   ```

   The template derives host → `{subdomain}.{clusterDomain}`, dest host → `{serviceName}.{destNamespace}.svc.cluster.local`, VS name → `{subdomain}`. Most services need only `enable_public: true`.
5. **Validate locally**:
   ```bash
   .useful-scripts/ct_check.sh services/helm/<name>
   ```
6. **PR and merge** — ArgoCD auto-syncs.
