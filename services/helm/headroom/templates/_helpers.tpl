{{- define "headroom.fullname" -}}
kagent-headroom
{{- end -}}

{{- define "headroom.labels" -}}
app.kubernetes.io/name: {{ include "headroom.fullname" . }}
{{- end -}}
