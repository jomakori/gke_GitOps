{{- define "argocd.application" -}}
{{- $cfg := .config -}}
{{- $root := .root -}}
{{- /* Defaults — applied per-service so values.yaml only needs overrides */ -}}
{{- if not (hasKey $cfg "selfHeal") -}}
{{-   $_ := set $cfg "selfHeal" true -}}
{{- end -}}
{{- if not (hasKey $cfg "syncOptions") -}}
{{-   $_ := set $cfg "syncOptions" (list "CreateNamespace=true") -}}
{{- end -}}
{{- if not (hasKey $cfg "finalizer") -}}
{{-   $_ := set $cfg "finalizer" true -}}
{{- end -}}
{{- $name := $cfg.name | default (lower (regexReplaceAll "([a-z])([A-Z])" (toString .key) "${1}-${2}" | lower)) -}}
{{- $helmPath := $cfg.helmPath | default (printf "services/helm/%s" $name) -}}
{{- $destNamespace := $cfg.destNamespace | default $name -}}
{{- $dopplerConfig := $cfg.dopplerConfig | default (printf "svc_%s" $name) -}}
{{- $namespace := $cfg.argocdNamespace | default (printf "%s" ($root.Values.argoNamespace | default "argocd")) -}}
{{- $project := $cfg.argoProject | default (printf "%s" ($root.Values.argoProject | default "default")) -}}
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: {{ $name }}
  namespace: {{ $namespace }}
  {{- if $cfg.finalizer }}
  finalizers:
    - resources-finalizer.argocd.argoproj.io
  {{- end }}
  annotations:
    argocd.argoproj.io/sync-wave: {{ $cfg.syncWave | default "0" | quote }}
spec:
  project: {{ $project }}
  revisionHistoryLimit: {{ $cfg.revisionHistoryLimit | default 3 }}
  {{- with $cfg.ignoreDifferences }}
  ignoreDifferences:
    {{- toYaml . | nindent 4 }}
  {{- end }}
  source:
    repoURL: {{ $root.Values.repoUrl | quote }}
    path: {{ $helmPath | quote }}
    targetRevision: {{ $root.Values.targetRevision | quote }}
    helm:
      valueFiles:
        - values.yaml
      {{- if $cfg.skipCrds }}
      skipCrds: true
      {{- end }}
      {{- $allParams := $cfg.parameters | default list }}
      {{- if $dopplerConfig }}
      {{-   $allParams = append $allParams (dict "name" "dopplerConfig" "value" $dopplerConfig) }}
      {{- end }}
      {{- if $allParams }}
      parameters:
        {{- range $allParams }}
        - name: {{ .name }}
          value: {{ tpl .value $root | quote }}
        {{- end }}
      {{- end }}
  destination:
    server: {{ $root.Values.clusterServer | default "https://kubernetes.default.svc" }}
    namespace: {{ $destNamespace | quote }}
  syncPolicy:
    automated:
      prune: {{ $cfg.prune | default true }}
      {{- if $cfg.selfHeal }}
      selfHeal: true
      {{- end }}
    {{- with $cfg.syncOptions }}
    syncOptions:
      {{- toYaml . | nindent 6 }}
    {{- end }}
    {{- with $cfg.managedNamespaceMetadata }}
    managedNamespaceMetadata:
      {{- toYaml . | nindent 6 }}
    {{- end }}
    retry:
      limit: {{ $cfg.retryLimit | default 1 }}
      backoff:
        duration: 5s
        factor: 2
        maxDuration: {{ $cfg.retryMaxDuration | default "1m" }}
{{- end -}}

{{- define "argocd.application.multi" -}}
{{- $cfg := .config -}}
{{- $root := .root -}}
{{- $env := .env -}}
{{- /* Defaults — applied per-service so values.yaml only needs overrides */ -}}
{{- if not (hasKey $cfg "selfHeal") -}}
{{-   $_ := set $cfg "selfHeal" true -}}
{{- end -}}
{{- if not (hasKey $cfg "syncOptions") -}}
{{-   $_ := set $cfg "syncOptions" (list "CreateNamespace=true") -}}
{{- end -}}
{{- if not (hasKey $cfg "finalizer") -}}
{{-   $_ := set $cfg "finalizer" true -}}
{{- end -}}
{{- $baseName := $cfg.name | default (lower (regexReplaceAll "([a-z])([A-Z])" (toString .key) "${1}-${2}" | lower)) -}}
{{- $name := printf "%s-%s" $baseName $env -}}
{{- $helmPath := $cfg.helmPath | default (printf "services/helm/%s" $baseName) -}}
{{- $destNamespace := $cfg.destNamespace | default $baseName -}}
{{- $dopplerConfig := $cfg.dopplerConfig | default (printf "svc_%s" $baseName) -}}
{{- $namespace := $cfg.argocdNamespace | default (printf "%s" ($root.Values.argoNamespace | default "argocd")) -}}
{{- $project := $cfg.argoProject | default (printf "%s" ($root.Values.argoProject | default "default")) -}}
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: {{ $name }}
  namespace: {{ $namespace }}
  {{- if $cfg.finalizer }}
  finalizers:
    - resources-finalizer.argocd.argoproj.io
  {{- end }}
  {{- if $cfg.syncWave }}
  annotations:
    argocd.argoproj.io/sync-wave: {{ $cfg.syncWave | quote }}
  {{- end }}
spec:
  project: {{ $project }}
  revisionHistoryLimit: {{ $cfg.revisionHistoryLimit | default 3 }}
  source:
    repoURL: {{ $root.Values.repoUrl | quote }}
    path: {{ $helmPath | quote }}
    targetRevision: {{ $root.Values.targetRevision | quote }}
    helm:
      valueFiles:
        - values.yaml
      {{- $allParams := $cfg.parameters | default list }}
      {{- if $dopplerConfig }}
      {{-   $allParams = append $allParams (dict "name" "dopplerConfig" "value" $dopplerConfig) }}
      {{- end }}
      {{- if $allParams }}
      parameters:
        {{- range $allParams }}
        - name: {{ .name }}
          value: {{ tpl .value $root | quote }}
        {{- end }}
      {{- end }}
  destination:
    server: {{ $root.Values.clusterServer | default "https://kubernetes.default.svc" }}
    namespace: {{ printf "%s-%s" $destNamespace $env | quote }}
  syncPolicy:
    automated:
      prune: {{ $cfg.prune | default true }}
      {{- if $cfg.selfHeal }}
      selfHeal: true
      {{- end }}
    {{- with $cfg.syncOptions }}
    syncOptions:
      {{- toYaml . | nindent 6 }}
    {{- end }}
    retry:
      limit: {{ $cfg.retryLimit | default 1 }}
      backoff:
        duration: 5s
        factor: 2
        maxDuration: {{ $cfg.retryMaxDuration | default "1m" }}
{{- end -}}
