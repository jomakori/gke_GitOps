{{- define "openagent-headroom.fullname" -}}
openagent-headroom
{{- end -}}

{{- define "openagent-headroom.labels" -}}
app.kubernetes.io/name: {{ include "openagent-headroom.fullname" . }}
{{- end -}}
