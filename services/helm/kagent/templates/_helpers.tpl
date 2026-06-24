{{- define "kagent.labels" -}}
app.kubernetes.io/name: kagent
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: kagent
{{- end }}
