{{- define "litellm.labels" -}}
app.kubernetes.io/name: litellm
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: openagent
{{- end }}

{{- define "litellm.fullname" -}}
{{ .Release.Name }}-litellm
{{- end }}
