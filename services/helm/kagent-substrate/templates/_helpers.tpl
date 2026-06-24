{{- define "kagent-substrate.labels" -}}
helm.sh/chart: {{ include "kagent-substrate.name" . }}-{{ .Chart.Version | replace "+" "_" }}
{{ include "kagent-substrate.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "kagent-substrate.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "kagent-substrate.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kagent-substrate.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
