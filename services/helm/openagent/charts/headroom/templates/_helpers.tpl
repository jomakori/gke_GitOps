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

{{- define "headroom.fullname" -}}
openagent-headroom
{{- end -}}
