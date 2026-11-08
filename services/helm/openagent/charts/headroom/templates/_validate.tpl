{{/*
Fail fast on the one configuration that would silently expose Headroom:
remote callers allowed + no NetworkPolicy + no shared secret. The chart is
deployed by ArgoCD, so a render-time failure is the only place this can be
caught before the proxy is reachable from every pod in the cluster.
*/}}
{{- define "headroom.validate" -}}
{{- if .Values.enabled }}
{{- if .Values.proxy.auth.enabled }}
{{- if not .Values.proxy.auth.existingSecret.name }}
{{- fail "headroom.proxy.auth.enabled=true requires headroom.proxy.auth.existingSecret.name (Secret holding the shared proxy token)" }}
{{- end }}
{{- if not .Values.proxy.auth.existingSecret.key }}
{{- fail "headroom.proxy.auth.enabled=true requires headroom.proxy.auth.existingSecret.key" }}
{{- end }}
{{- else if not .Values.networkPolicy.enabled }}
{{- fail "refusing to render Headroom unauthenticated with no NetworkPolicy: set headroom.networkPolicy.enabled=true (restricts ingress to the LiteLLM pod), or enable headroom.proxy.auth with an existing Secret" }}
{{- end }}
{{- end }}
{{- end }}
