{{- define "kagent-headroom.fullname" -}}
kagent-headroom
{{- end -}}

{{- define "kagent-headroom.labels" -}}
app.kubernetes.io/name: {{ include "kagent-headroom.fullname" . }}
{{- end -}}
