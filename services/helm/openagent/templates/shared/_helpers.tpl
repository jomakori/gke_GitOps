{{- define "openagent.name" -}}
{{- default "openagent" .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "openagent.fullname" -}}
{{- $name := default "openagent" .Values.nameOverride -}}
{{- printf "%s" $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "openagent.labels" -}}
app.kubernetes.io/name: {{ include "openagent.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: openagent
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "openagent.dashboardURL" -}}
{{- $base := .Values.clusterDomain -}}
{{- printf "%s.%s" .Values.dashboard.subdomain $base -}}
{{- end -}}
