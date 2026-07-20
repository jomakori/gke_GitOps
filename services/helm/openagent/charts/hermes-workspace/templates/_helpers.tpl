{{- define "hermes-workspace.name" -}}
{{- default "hermes-workspace" .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "hermes-workspace.labels" -}}
app.kubernetes.io/name: {{ include "hermes-workspace.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: openagent
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "hermes-workspace.fullname" -}}
{{- printf "%s-%s" .Release.Name (include "hermes-workspace.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
