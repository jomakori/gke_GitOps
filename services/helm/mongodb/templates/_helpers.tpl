{{- define "mongodb.fullname" -}}
{{- printf "%s" .Release.Name -}}
{{- end -}}

{{- define "mongodb.name" -}}
{{- default "mongodb" .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "mongodb.labels" -}}
app.kubernetes.io/name: {{ include "mongodb.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: Helm
{{- end -}}
