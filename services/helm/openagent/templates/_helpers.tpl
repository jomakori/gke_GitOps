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

{{- define "openagent.modelList" -}}
{{- $list := list -}}
{{- range .Values.models -}}
{{- $entry := dict "model_name" .name "litellm_params" (dict "model" .litellmModel "api_key" (printf "os.environ/%s" .apiKey)) -}}
{{- if .apiBase -}}
{{- $entry = merge $entry (dict) -}}
{{- $params := dict "model" .litellmModel "api_key" (printf "os.environ/%s" .apiKey) "api_base" (printf "os.environ/%s" .apiBase) -}}
{{- $entry = dict "model_name" .name "litellm_params" $params -}}
{{- end -}}
{{- $list = append $list $entry -}}
{{- end -}}
{{- toYaml $list -}}
{{- end -}}

{{- define "openagent.fallbacks" -}}
{{- $fallbacks := list -}}
{{- range $k, $v := .Values.litellmConfig.router.fallbacks -}}
{{- $fallbacks = append $fallbacks (dict $k $v) -}}
{{- end -}}
{{- toYaml $fallbacks -}}
{{- end -}}
