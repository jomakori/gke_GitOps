{{- define "openagent-component.name" -}}
{{- default "openagent-component" .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "openagent-component.labels" -}}
app.kubernetes.io/name: {{ include "openagent-component.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: openagent
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "openagent-component.headroomName" -}}
openagent-headroom
{{- end -}}

{{- define "openagent-component.botName" -}}
openagent-discord
{{- end -}}
