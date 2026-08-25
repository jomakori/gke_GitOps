{{- define "hermes-webui.name" -}}
{{- default "hermes-webui" .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "hermes-webui.fullname" -}}
{{- printf "%s-%s" .Release.Name (include "hermes-webui.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "hermes-webui.labels" -}}
app.kubernetes.io/name: {{ include "hermes-webui.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: openagent
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
{{- end -}}
