{{- define "kagent-discord.labels" -}}
app.kubernetes.io/name: kagent-discord
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: kagent-discord
{{- end }}
