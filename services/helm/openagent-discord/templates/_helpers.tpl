{{- define "openagent-discord.labels" -}}
app.kubernetes.io/name: openagent-discord
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: openagent-discord
{{- end }}
