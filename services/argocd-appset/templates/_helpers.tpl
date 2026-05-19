{{- define "argocd.application" -}}
{{- $cfg := .config -}}
{{- $root := .root -}}
{{- $name := $cfg.name | default (lower (regexReplaceAll "([a-z])([A-Z])" (toString .key) "${1}-${2}" | lower)) -}}
{{- $namespace := $cfg.argocdNamespace | default (printf "%s" ($root.Values.argoNamespace | default "argocd")) -}}
{{- $project := $cfg.argoProject | default (printf "%s" ($root.Values.argoProject | default "default")) -}}
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: {{ $name }}
  namespace: {{ $namespace }}
  {{- with $cfg.finalizers }}
  finalizers:
    {{- toYaml . | nindent 4 }}
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
    path: {{ $cfg.helmPath | quote }}
    targetRevision: {{ $root.Values.targetRevision | quote }}
    helm:
      valueFiles:
        - values.yaml
      {{- if $cfg.skipCrds }}
      skipCrds: true
      {{- end }}
      {{- with $cfg.parameters }}
      parameters:
        {{- range . }}
        - name: {{ .name }}
          value: {{ tpl .value $root }}
        {{- end }}
      {{- end }}
  destination:
    server: {{ $root.Values.destinationServer | default "https://kubernetes.default.svc" }}
    namespace: {{ $cfg.destNamespace | quote }}
  syncPolicy:
    automated:
      prune: true
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
{{- $name := printf "%s-%s" $cfg.name $env -}}
{{- $namespace := $cfg.argocdNamespace | default (printf "%s" ($root.Values.argoNamespace | default "argocd")) -}}
{{- $project := $cfg.argoProject | default (printf "%s" ($root.Values.argoProject | default "default")) -}}
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: {{ $name }}
  namespace: {{ $namespace }}
  {{- with $cfg.finalizers }}
  finalizers:
    {{- toYaml . | nindent 4 }}
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
    path: {{ $cfg.helmPath | quote }}
    targetRevision: {{ $root.Values.targetRevision | quote }}
    helm:
      valueFiles:
        - values.yaml
      {{- with $cfg.parameters }}
      parameters:
        {{- range . }}
        - name: {{ .name }}
          value: {{ tpl .value $root }}
        {{- end }}
      {{- end }}
  destination:
    server: {{ $root.Values.destinationServer | default "https://kubernetes.default.svc" }}
    namespace: {{ printf "%s-%s" $cfg.destNamespace $env | quote }}
  syncPolicy:
    automated:
      prune: true
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
