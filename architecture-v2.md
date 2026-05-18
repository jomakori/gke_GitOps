# Spec-Driven GitOps Architecture (v2)

## Architecture Flow

```mermaid
flowchart TB
    subgraph Spec["📐 SPEC LAYER (CUE)"]
        S1["schemas.cue
        ────────────
        #Service schema
        #App schema  
        #MeshConfig schema
        #Environment schema"]
        S2["defaults.cue
        ────────────
        Istio mesh defaults
        Resource defaults
        Label conventions
        mTLS mode: STRICT"]
        S3["cluster/istio.cue
        ────────────
        IstioOperator
        MeshConfig
        Mesh-wide PeerAuth"]
        S4["services/*.cue
        ────────────
        kube-prometheus-stack
        external-secrets
        keda, mongodb ..."]
        S5["apps/*.cue
        ────────────
        demo-app
        notes-app"]
    end

    subgraph Gen["⚙️ CUE GENERATION"]
        G1["cue eval gen/ ✦
        ────────────────
        Validates schemas
        Applies defaults
        Resolves overrides"]
        G2["Generated YAML:
        ────────────────
        ├── services/argocd-appset/
        ├── apps/argocd-appset/
        ├── apps/helm/*/values.yaml
        └── istio/*.yaml"]
    end

    subgraph Git["📦 GITOPS REPO (gke_GitOps)"]
        H1["Generated + committed
        ────────────────
        Pull request → merge
        Renovate auto-updates
        Pre-commit hooks validate"]
    end

    subgraph Argo["🔄 ARGOCD"]
        A1["App-of-Apps syncs
        ────────────────
        Watches Git
        Auto-syncs changes
        Prunes drift"]
    end

    subgraph K8s["☸️ KUBERNETES"]
        K1["Istio Mesh
        ────────────
        Control Plane: istiod
        Data Plane: Envoy sidecars
        Ingress Gateway"]
        K2["Infra Services
        ────────────
        kube-prometheus-stack
        external-secrets, keda ..."]
        K3["App Workloads
        ────────────
        demo-app, notes-app ..."]
    end

    S1 --> G1
    S2 --> G1
    S3 --> G1
    S4 --> G1
    S5 --> G1
    G1 --> G2
    G2 --> H1
    H1 --> A1
    A1 --> K1
    A1 --> K2
    A1 --> K3
    K1 --> K2
    K1 --> K3
```

---

## Repository Structure

```mermaid
flowchart LR
    subgraph Root["gke_GitOps/"]
        direction TB
        CUE["cue/
        ├── cue.mod/ ← Istio/K8s types
        ├── schemas/
        │   ├── service.cue   ← #Service
        │   ├── app.cue       ← #App
        │   ├── istio.cue     ← #MeshConfig
        │   └── defaults.cue  ← shared defaults
        ├── cluster/
        │   └── istio.cue     ← mesh-level config
        ├── services/
        │   ├── kube-prometheus-stack.cue
        │   ├── external-secrets.cue
        │   ├── keda.cue
        │   └── ... 
        ├── apps/
        │   ├── demo-app.cue
        │   └── notes-app.cue
        └── gen/  ← CUE output
            ├── services/argocd-appset/
            ├── apps/argocd-appset/
            ├── apps/helm/*/values.yaml
            └── istio/
                ├── mesh-config.yaml
                ├── peer-authentications.yaml
                ├── destination-rules.yaml
                ├── authorization-policies.yaml
                └── gateways/
                    ├── ingress-gateway.yaml
                    └── http-routes.yaml"]

        SERVICES["services/ (Helm)
        ├── argocd-appset/ ← GENERATED
        └── helm/
            ├── kube-prometheus-stack/
            ├── external-secrets/
            └── ... "]

        APPS["apps/ (Helm)
        ├── argocd-appset/ ← GENERATED
        └── helm/
            ├── demo-app/
            └── notes-app/"]
    end

    CUE --> SERVICES
    CUE --> APPS
```

---

## Spec Schemas

### #Service — for infrastructure (3rd-party Helm charts)

```cue
// cue/schemas/service.cue
#Service: {
  name:       string
  namespace?: string | *name   // defaults to name
  chart: {
    repository: string          // helm repo URL
    name:       string
    version?:   string
    alias?:     string
  }
  enabled:     bool | *true
  syncWave:    int | *0
  values: {...}                 // arbitrary helm values
  mesh?: {                     // optional Istio integration
    mtls?:                     // per-service mTLS override
    authorization?: {...}
  }
}
```

### #App — for workload applications

```cue
// cue/schemas/app.cue
#App: {
  name:        string
  namespace:   string
  enabled:     bool | *true
  ports: [...#Port]
  mesh: {                              // Istio config (required for apps)
    mtls:           "STRICT" | "PERMISSIVE" | "DISABLE" | *"STRICT"
    retries:        int | *3
    timeout:        string | *"30s"
    circuitBreaker?: #CircuitBreaker
    loadBalancer?:   "ROUND_ROBIN" | "LEAST_CONN" | "RANDOM" | *"ROUND_ROBIN"
    trafficRouting?: #TrafficRouting   // canary, A/B, mirroring
    authorization?: [#AuthorizationPolicy]
  }
  ingress?: {                          // north-south exposure
    host:     string
    paths: [...#IngressPath]
    tls?:     #TLS
  }
  environments: [...#Environment]
  resources: #Resources
}

#Resources: {
  requests?: {
    cpu:    string | *"256m"
    memory: string | *"256Mi"
  }
  limits?: {
    cpu:    string | *"500m"
    memory: string | *"512Mi"
  }
}
```

### Defaults — cluster-wide

```cue
// cue/schemas/defaults.cue
#IstioDefaults: {
  meshConfig: {
    enableTracing:          true
    defaultConfig: {
      terminationDrainDuration: "30s"
      proxyMetadata: {
        ISTIO_META_DNS_CAPTURE: "true"
      }
    }
  }
  // Applied to ALL namespaces unless overridden
  peerAuthentication: {
    mtls: mode: "STRICT"
  }
}
```

---

## Per-Component Spec Examples

### Service spec (kube-prometheus-stack)

```cue
// cue/services/kube-prometheus-stack.cue
kubePrometheusStack: #Service & {
  name:    "kube-prometheus-stack"
  enabled: true
  syncWave: 1
  
  chart: {
    repository: "https://prometheus-community.github.io/helm-charts"
    name:       "kube-prometheus-stack"
    version:    "62.0.0"
  }
  
  values: {
    defaultRules: {
      create: true
      rules: { etcd: false, kubeScheduler: false }
    }
    alertmanager: { service: { port: 15010 } }
    // ... rest of values
  }
}
```

### App spec (demo-app)

```cue
// cue/apps/demo-app.cue
demoApp: #App & {
  name:      "demoapp"
  namespace: "demo-app"
  enabled:   true
  
  ports: [{ port: 3000, protocol: "HTTP" }]
  
  mesh: {
    mtls:             "STRICT"
    retries:          3
    timeout:          "15s"
    circuitBreaker: {
      maxConnections: 100
      maxRequests:    1000
      maxRetries:     3
    }
    loadBalancer: "LEAST_CONN"
  }
  
  ingress: {
    host: "demo.jmak.dev"
    paths: [{ path: "/", type: "PathPrefix", backend: 3000 }]
    tls: { secretName: "demo-tls" }
  }
  
  environments: [
    {
      name:       "staging"
      namespace:  "staging-demo-app"
      host:       "staging.demo.jmak.dev"
      dopplerToken: ""  // injected by Terraform
      image: {
        repository: "123456.dkr.ecr.us-east-2.amazonaws.com/demoapp"
        tag: "latest"
      }
    },
    {
      name:       "production"
      namespace:  "prod-demo-app"
      host:       "demo.jmak.dev"
      dopplerToken: ""
      image: {
        repository: "123456.dkr.ecr.us-east-2.amazonaws.com/demoapp"
        tag: "latest"
      }
    }
  ]
  
  resources: {
    requests: { cpu: "256m", memory: "256Mi" }
    limits:   { cpu: "500m", memory: "512Mi" }
  }
}
```

### Cluster-level Istio config

```cue
// cue/cluster/istio.cue
istio: {
  // ── Installation ──
  operator: {
    profile: "default"
    components: {
      ingressGateways: [{ name: "istio-ingressgateway", enabled: true }]
      egressGateways:  [{ name: "istio-egressgateway", enabled: false }]
    }
    meshConfig: #IstioDefaults.meshConfig
  }
  
  // ── Mesh-wide policies ──
  defaultPeerAuthentication: {
    apiVersion: "security.istio.io/v1beta1"
    kind:       "PeerAuthentication"
    metadata: {
      name:      "default"
      namespace: "istio-system"
    }
    spec: { mtls: { mode: "STRICT" } }
  }
  
  // ── Shared ingress gateway ──
  ingressGateway: {
    apiVersion: "gateway.networking.k8s.io/v1"
    kind:       "Gateway"
    metadata: { name: "shared-ingress", namespace: "istio-ingress" }
    spec: {
      gatewayClassName: "istio"
      listeners: [{
        name:     "http"
        port:     80
        protocol: "HTTP"
        allowedRoutes: { namespaces: { from: "All" } }
      }, {
        name:     "https"
        port:     443
        protocol: "HTTPS"
        tls: { mode: "Terminate", credentialName: "shared-cert" }
        allowedRoutes: { namespaces: { from: "All" } }
      }]
    }
  }
}
```

---

## What Gets Generated

| Spec Input | Generated Output | Target |
|---|---|---|
| `cluster/istio.cue` | `istio/mesh-config.yaml` | IstioOperator CR |
| `cluster/istio.cue` | `istio/peer-authentications.yaml` | Mesh-wide PeerAuthentication |
| `cluster/istio.cue` | `istio/gateways/shared-ingress.yaml` | Ingress Gateway |
| `apps/demo-app.cue` | `apps/argocd-appset/templates/demo-app.yaml` | ArgoCD Application |
| `apps/demo-app.cue` | `apps/helm/demo-app/values.yaml` | Helm values |
| `apps/demo-app.cue` | `istio/destination-rules/demo-app.yaml` | DestinationRule |
| `apps/demo-app.cue` | `istio/peer-authentications/demo-app.yaml` | Namespace PeerAuth |
| `apps/demo-app.cue` | `istio/authorization-policies/demo-app.yaml` | AuthorizationPolicy |
| `apps/demo-app.cue` | `istio/virtual-services/demo-app.yaml` | VirtualService/HTTPRoute |
| `services/kube-prometheus-stack.cue` | `services/argocd-appset/templates/kube-prometheus-stack.yaml` | ArgoCD Application |

---

## Incremental Migration Path

```mermaid
gantt
    title Migration to Spec-Driven GitOps
    dateFormat  YYYY-MM-DD
    section Phase 1
        Set up CUE module + schemas      :p1a, 2026-05-18, 3d
        Define #Service + #App schemas   :p1b, 2026-05-19, 3d
        Define #IstioDefaults + validators:p1c, 2026-05-20, 3d
    section Phase 2
        Migrate infra services (one by one):p2a, 2026-05-21, 7d
        Migrate apps (demo-app, notes-app) :p2b, 2026-05-23, 5d
    section Phase 3
        Cluster Istio config in CUE       :p3a, 2026-05-26, 3d
        Generation pipeline (make/gen.sh) :p3b, 2026-05-27, 3d
        CI check: cue vet on every PR     :p3c, 2026-05-28, 2d
    section Phase 4
        Backstage integration (shared schemas):p4a, 2026-06-02, 5d
        Software Templates scaffold from specs :p4b, 2026-06-05, 5d
```
