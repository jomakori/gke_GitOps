{{- /* ── Helpers ─────────────────────────────────────────────────────── */}}
{{- /*
  kebab converts CamelCase to kebab-case.
  Ex: demoApi → demo-api, notesUi → notes-ui, myAPI → my-api
*/}}
{{- define "kebab" -}}
{{- regexReplaceAll "(.)([A-Z])" . "${1}-${2}" | lower -}}
{{- end -}}


{{- /* ═════════════════════════════════════════════════════════════ */}}
{{- /* app.manifests — generates ALL Kubernetes manifests for one app  */}}
{{- /* ═════════════════════════════════════════════════════════════ */}}
{{- define "app.manifests" -}}
{{- /* ── Scoped vars ─────────────────────────────────────────────── */}}
{{- $appName   := .appName }}
{{- $kebabName := include "kebab" $appName }}
{{- $app       := .appConfig }}
{{- $root      := .root }}

{{- /* Global defaults */}}
{{- $domain       := $root.Values.clusterDomain | default "maklab.net" }}
{{- $registry     := $root.Values.registry }}
{{- $storageClass := $root.Values.storageClass | default "csi-hostpath-sc" }}
{{- $ingressCfg   := $root.Values.ingress      | default dict }}

{{- /* App-level defaults (enable_staging, enable_domain, enable_istio, scaling, service) */}}
{{- $enableStaging := ne (printf "%v" $app.enable_staging) "false" }}
{{- $enablePrivate   := ne (printf "%v" ($app.enable_private | default false)) "false" }}
{{- $createNamespace := ne (printf "%v" ($app.createNamespace | default false)) "false" }}
{{- $enableDomain  := ne (printf "%v" $app.enable_domain) "false" }}
{{- $enableIstio   := ne (printf "%v" ($app.enable_istio | default true)) "false" }}
{{- $enableScaling := $app.enable_scaling }}
{{- $hpa          := dict }}
{{- if kindIs "map" $enableScaling }}
{{-   $hpa = $enableScaling.HPA | default dict }}
{{- end }}
{{- $svc          := $app.service }}
{{- $svcReplicas  := $svc.replicas | default 1 }}
{{- $imageRepo    := ($app.image | default dict).repository | default (printf "%s/%s" $registry $kebabName) }}

{{- /* Cluster read (OKT-130). Two gates, both required: the spec opts in, and
       the environment is not a preview. `namespaceOverride` is the preview
       coordinate (apps/argocd-appset/templates/preview.yml) and it cannot be
       omitted by a preview without clobbering the prod namespace, so it is the
       gate that cannot be forgotten. A preview runs unreviewed PR code and must
       never hold cluster-wide read. */}}
{{- $clusterRead   := $app.cluster_read | default dict }}
{{- $isPreview     := ne (printf "%v" ($app.namespaceOverride | default "")) "" }}
{{- $clusterReadOn := and (ne (printf "%v" ($clusterRead.enabled | default false)) "false") (not $isPreview) }}

{{- range $envName, $env := $app.environments }}
{{- /* Skip staging when enable_staging is false */}}
{{- if or (eq $envName "production") (and $enableStaging (eq $envName "staging")) }}

{{- /* ── Env-level defaults ───────────────────────────────────── */}}
{{- $namespace   := $app.namespaceOverride | default (printf "%s-%s" $kebabName $envName) }}
{{- $defSub      := ternary $kebabName (printf "staging.%s" $kebabName) (eq $envName "production") }}
{{- $subdomain   := $env.subdomain | default $defSub }}
{{- $tag         := $env.tag | default "latest" }}
{{- $fullDomain  := printf "%s.%s" $subdomain $domain }}
{{- /* The ECR pull secret belongs to the shared registry; an app naming its own
       (public) registry gets no Secret — an empty kubernetes.io/dockercfg is rejected
       by the API, which fails the entire sync (not just that one resource). */}}
{{- $imagePullSecret := ternary (printf "%s-registry" $namespace) "" (empty (($app.image | default dict).repository)) }}

{{- /* ── Istio routing config (app-level overrides) ────────────── */}}
{{- $istioSpec     := $app.istio | default dict }}
{{- $gatewayRef    := $istioSpec.gateway | default "istio-system/maklab-gateway" }}
{{- $retryAttempts := $istioSpec.retryAttempts | default 3 }}
{{- $retryTimeout  := $istioSpec.retryTimeout | default "5s" }}
{{- $requestTimeout := $istioSpec.requestTimeout | default "30s" }}

{{- if $createNamespace }}
---
apiVersion: v1
kind: Namespace
metadata:
  name: {{ $namespace }}
  labels:
    app: {{ $appName }}
    env: {{ $envName }}
    istio.io/dataplane-mode: ambient
{{- end }}

{{- /* ═════════════════════════════════════════════════════════════ */}}
{{- /* ServiceAccount + Registry Secret                              */}}
{{- /* ═════════════════════════════════════════════════════════════ */}}
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ $namespace }}-sa
  namespace: {{ $namespace }}
  labels:
    app: {{ $appName }}
    env: {{ $envName }}
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456:role/ecr-readonly-access-allrepos
{{- if $imagePullSecret }}
secrets:
  - name: {{ $imagePullSecret }}
{{- end }}
{{- /* ── Console read RBAC (OKT-130) ──────────────────────────────── */}}
{{- /* Read-only, opt-in per app spec, NEVER for a preview ($isPreview below).
       Kinds are the union of every Api:: call site in the console, not the nav
       alone: the shell starts cluster-wide reflectors (Api::<T>::all), so a
       namespaced Role would 403 them, and nodes/namespaces are cluster-scoped
       and cannot be granted by a Role at all. `secrets` is deliberately
       excluded — `list secrets` returns the values, not just the keys, so that
       grant is its own decision (values.yaml). */}}
{{- if $clusterReadOn }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ $namespace }}-console-read
  labels:
    app: {{ $appName }}
    env: {{ $envName }}
rules:
  # core/v1 — namespaced kinds the console reflects.
  - apiGroups: [""]
    resources:
      - pods
      - services
      - configmaps
      - persistentvolumeclaims
    verbs: ["get", "list", "watch"]
  # Pod logs are a subresource the console reads; nothing lists them.
  - apiGroups: [""]
    resources: ["pods/log"]
    verbs: ["get"]
  # Cluster-scoped: nodes are a nav item, namespaces drive the topbar filter.
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list"]
  # Inspector Events tab lists events; nothing gets or watches one.
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["list"]
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets", "daemonsets", "replicasets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["batch"]
    resources: ["jobs", "cronjobs"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["ingresses"]
    verbs: ["get", "list", "watch"]
  # Argo CD plugin read (the nav item `applications` is enabled).
  - apiGroups: ["argoproj.io"]
    resources: ["applications"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {{ $namespace }}-console-read
  labels:
    app: {{ $appName }}
    env: {{ $envName }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: {{ $namespace }}-console-read
subjects:
  - kind: ServiceAccount
    name: {{ $namespace }}-sa
    namespace: {{ $namespace }}
{{- end }}

{{- if $imagePullSecret }}
---
apiVersion: v1
kind: Secret
metadata:
  name: {{ $namespace }}-registry
  namespace: {{ $namespace }}
  labels:
    app: {{ $appName }}
    env: {{ $envName }}
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456:role/ecr-readonly-access-allrepos
type: kubernetes.io/dockercfg
{{- end }}

{{- /* ═════════════════════════════════════════════════════════════ */}}
{{- /* ExternalSecret — pulls from Doppler via ClusterSecretStore    */}}
{{- /* ═════════════════════════════════════════════════════════════ */}}
{{- if $env.dopplerConfig }}
---
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: {{ $namespace }}-vars
  namespace: {{ $namespace }}
  annotations:
      argocd.argoproj.io/sync-wave: "-1"
spec:
  secretStoreRef:
    kind: ClusterSecretStore
    name: doppler-{{ $env.dopplerConfig | replace "_" "-" }}
  refreshInterval: 24h
  target:
    name: {{ $namespace }}-vars
  dataFrom:
    - find:
        name:
          regexp: .*
{{- end }}

{{- /* ═════════════════════════════════════════════════════════════ */}}
{{- /* Database Resources (conditional on enable_db)                  */}}
{{- /* ═════════════════════════════════════════════════════════════ */}}
{{- $dbSpec := $app.enable_db }}
{{- if $dbSpec }}
{{- $dbType := $dbSpec.type | default "postgres" }}
{{- $dbDeployment := $dbSpec.deployment | default "db" }}
{{- $dbVersion := $dbSpec.version | default "17" }}
{{- $dbStorage := $dbSpec.storage | default "10Gi" }}
{{- $dbStorageClass := $dbSpec.storageClass | default $storageClass }}
{{- $isCluster := eq $dbDeployment "cluster" }}
{{- $dbScaling := dict }}
{{- if kindIs "map" $enableScaling }}
{{-   $dbScaling = $enableScaling.db | default dict }}
{{- end }}
{{- $dbName := printf "%s-%s-db" $kebabName $envName }}

{{- if eq $dbType "postgres" }}
---
apiVersion: stackgres.io/v1
kind: SGCluster
metadata:
  name: {{ $dbName }}
  namespace: {{ $namespace }}
  annotations:
    argocd.argoproj.io/sync-wave: "-1"
spec:
  {{- if $isCluster }}
  instances: {{ $dbScaling.minInstances | default 2 }}
  autoscaling:
    mode: horizontal
    minInstances: {{ $dbScaling.minInstances | default 2 }}
    maxInstances: {{ $dbScaling.maxInstances | default 6 }}
    horizontal:
      replicasConnectionsUsageTarget: "0.5"
  {{- else }}
  instances: 1
  {{- end }}
  postgres:
    version: {{ $dbVersion | quote }}
  pods:
    disableConnectionPooling: false
    disableMetricsExporter: false
    persistentVolume:
      storageClass: {{ $dbStorageClass | quote }}
      size: {{ $dbStorage | quote }}
  configurations:
    sgPoolingConfig: |
      connections:
        max: 200
        default: 5
{{- else if eq $dbType "mongodb" }}
---
apiVersion: psmdb.percona.com/v1
kind: PerconaServerMongoDB
metadata:
  name: {{ $dbName }}
  namespace: {{ $namespace }}
  annotations:
    argocd.argoproj.io/sync-wave: "-1"
  finalizers:
    - percona.com/delete-psmdb-pods-in-order
spec:
  crVersion: "1.22.0"
  image: percona/percona-server-mongodb:8.0.19-7
  imagePullPolicy: IfNotPresent
  unsafeFlags:
    replsetSize: true
  updateStrategy: SmartUpdate
  upgradeOptions:
    apply: disabled
  secrets:
    users: {{ $namespace }}-vars
  replsets:
    - name: rs0
      size: {{ if $isCluster }}{{ $dbScaling.minInstances | default 3 }}{{ else }}1{{ end }}
      {{- if $isCluster }}
      podDisruptionBudget:
        maxUnavailable: 1
      {{- end }}
      resources:
        limits:
          cpu: "600m"
          memory: "1Gi"
        requests:
          cpu: "300m"
          memory: "1Gi"
      volumeSpec:
        persistentVolumeClaim:
          storageClassName: {{ $dbStorageClass | quote }}
          accessModes:
            - ReadWriteOnce
          resources:
            requests:
              storage: {{ $dbStorage | quote }}
  sharding:
    enabled: false
{{- end }}
{{- end }}

{{- /* ═════════════════════════════════════════════════════════════ */}}
{{- /* Deployment                                                     */}}
{{- /* ═════════════════════════════════════════════════════════════ */}}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ $kebabName }}-{{ $envName }}
  namespace: {{ $namespace }}
  labels:
    app: {{ $appName }}
    env: {{ $envName }}
spec:
  replicas: {{ $svcReplicas }}
  revisionHistoryLimit: 3
  selector:
    matchLabels:
      app: {{ $appName }}
      env: {{ $envName }}
  template:
    metadata:
      labels:
        app: {{ $appName }}
        env: {{ $envName }}
    spec:
      nodeSelector:
        intent: apps
      serviceAccountName: {{ $namespace }}-sa
{{- if $imagePullSecret }}
      imagePullSecrets:
        - name: {{ $imagePullSecret }}
{{- end }}
      containers:
        - name: {{ $appName }}
          image: {{ $imageRepo }}:{{ $tag }}
          imagePullPolicy: Always
          ports:
          - containerPort: {{ $svc.port }}
          resources:
            requests:
              memory: {{ $svc.resourceRequests.memory }}
              cpu: {{ $svc.resourceRequests.cpu }}
          {{- with $svc.resourceLimits }}
            limits:
              {{- toYaml . | nindent 14 }}
          {{- end }}
          envFrom:
          - secretRef:
              name: {{ $namespace }}-vars
          {{- if and $svc.storage $svc.storage.size }}
          volumeMounts:
          - name: data
            mountPath: /data
          {{- end }}
      {{- if and $svc.storage $svc.storage.size }}
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: {{ $namespace }}-pvc
      {{- end }}

{{- /* ═════════════════════════════════════════════════════════════ */}}
{{- /* Service                                                        */}}
{{- /* ═════════════════════════════════════════════════════════════ */}}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ $kebabName }}-{{ $envName }}
  namespace: {{ $namespace }}
  labels:
    app: {{ $appName }}
    env: {{ $envName }}
spec:
  selector:
    app: {{ $appName }}
    env: {{ $envName }}
  ports:
  - name: http
    protocol: TCP
    port: 80
    targetPort: {{ $svc.port }}
  type: {{ ternary "ClusterIP" "NodePort" $enableIstio }}

{{- /* ═════════════════════════════════════════════════════════════ */}}
{{- /* VirtualService — Istio routing (replaces ALB Ingress)           */}}
{{- /* Conditional on enable_domain AND enable_istio                   */}}
{{- /* ═════════════════════════════════════════════════════════════ */}}
{{- if and $enableDomain $enableIstio }}
---
apiVersion: networking.istio.io/v1
kind: VirtualService
metadata:
  name: {{ $namespace }}-vs
  namespace: {{ $namespace }}
  labels:
    app: {{ $appName }}
    env: {{ $envName }}
spec:
  hosts:
    - {{ $fullDomain | quote }}
  gateways:
    - {{ $gatewayRef }}
  http:
    - timeout: {{ $requestTimeout }}
      retries:
        attempts: {{ $retryAttempts }}
        perTryTimeout: {{ $retryTimeout }}
        retryOn: gateway-error,connect-failure,retriable-4xx
      route:
        - destination:
            host: {{ $kebabName }}-{{ $envName }}.{{ $namespace }}.svc.cluster.local
            port:
              number: 80
{{- end }}

{{- /* Cloudflare Access gate: same rule the istio chart renders for enablePrivate,
       declared per exact host because a generated host has no route to hang off. */}}
{{- if and $enableDomain $enableIstio $enablePrivate }}
---
apiVersion: security.istio.io/v1
kind: AuthorizationPolicy
metadata:
  name: require-cf-access-{{ printf "%s-%s" $subdomain (replace "." "-" $domain) }}
  namespace: {{ index (splitList "/" $gatewayRef) 0 }}
  labels:
    app: {{ $appName }}
    env: {{ $envName }}
  annotations:
    argocd.argoproj.io/sync-wave: "2"
spec:
  selector:
    matchLabels:
      istio: ingressgateway
  action: DENY
  rules:
    - from:
        - source:
            notRequestPrincipals: ["*"]
      to:
        - operation:
            hosts:
              - {{ $fullDomain | quote }}
{{- end }}

{{- /* ═════════════════════════════════════════════════════════════ */}}
{{- /* HPA (conditional on enable_scaling)                            */}}
{{- /* ═════════════════════════════════════════════════════════════ */}}
{{- if $enableScaling }}
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ $namespace }}-hpa
  namespace: {{ $namespace }}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {{ $kebabName }}-{{ $envName }}
  minReplicas: {{ $hpa.minReplicas | default $svcReplicas }}
  maxReplicas: {{ $hpa.maxReplicas }}
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 75
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 75
{{- end }}

{{- /* ═════════════════════════════════════════════════════════════ */}}
{{- /* PVC (conditional on storage.size existing)                     */}}
{{- /* ═════════════════════════════════════════════════════════════ */}}
{{- if and $svc.storage $svc.storage.size }}
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{ $namespace }}-pvc
  namespace: {{ $namespace }}
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: {{ $svc.storage.size }}
  storageClassName: {{ $storageClass }}
{{- end }}

{{- end }} {{- /* if env enabled */}}
{{- end }} {{- /* range environments */}}
{{- end }} {{- /* define app.manifests */}}
