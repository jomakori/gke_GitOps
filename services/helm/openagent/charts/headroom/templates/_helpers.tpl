{{- define "headroom.name" -}}
{{- default "headroom" .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "headroom.labels" -}}
app.kubernetes.io/name: {{ include "headroom.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: openagent
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
{{- end -}}

{{/*
Selector labels — the STABLE subset. Deliberately excludes helm.sh/chart and
app.kubernetes.io/managed-by: those carry the chart version, and a
Deployment's spec.selector is immutable, so any chart version bump would make
ArgoCD's patch fail with "spec.selector: Invalid value: ... field is immutable".
Kept a subset of headroom.labels so the pod template still matches.
*/}}
{{- define "headroom.selectorLabels" -}}
app.kubernetes.io/name: {{ include "headroom.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
