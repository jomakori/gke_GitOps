{{- define "cloudflare-tunnel.labels" -}}
app.kubernetes.io/name: cloudflare-tunnel
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: cloudflare-tunnel
{{- end }}
