{{- define "opendesign.name" -}}
{{- default "open-design" .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "opendesign.fullname" -}}
{{- $name := default "open-design" .Values.nameOverride -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "opendesign.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/name: {{ include "opendesign.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/environment: production
{{- end -}}

{{- define "opendesign.domain" -}}
{{- .Values.domain | required "domain is required" -}}
{{- end -}}
