{{- define "openagent.modelList" -}}
{{- $list := list -}}
{{- range .Values.models -}}
{{- $params := dict "model" .litellmModel "api_key" (printf "os.environ/%s" .apiKey) -}}
{{- if .apiBase -}}
{{- $params = set $params "api_base" (printf "os.environ/%s" .apiBase) -}}
{{- end -}}
{{- $list = append $list (dict "model_name" .name "litellm_params" $params) -}}
{{- end -}}
{{- toYaml $list -}}
{{- end -}}

{{- define "openagent.fallbacks" -}}
{{- $fallbacks := list -}}
{{- range .Values.litellmConfig.router.fallbacks -}}
{{- $fallbacks = append $fallbacks (dict .key .value) -}}
{{- end -}}
{{- toYaml $fallbacks -}}
{{- end -}}