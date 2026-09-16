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

{{- define "openagent.hermesImage" -}}
{{- $image := index .Values "hermes-agent" "image" -}}
{{- printf "%s:%s" $image.repository $image.tag -}}
{{- end -}}

{{/*
Environment and volumes shared by the MCP preflight Job and drift CronJob.
Both run the same image as the gateway and reuse the PVC toolchain + caches the
boot pre-warm materialised, so the handshake is exactly what the gateway does.
*/}}
{{- define "openagent.mcpVerifyEnv" -}}
- name: HOME
  value: /opt/data/home
- name: MISE_DATA_DIR
  value: /opt/data/mise
- name: MISE_CACHE_DIR
  value: /opt/data/mise-cache
- name: UV_CACHE_DIR
  value: /opt/data/home/.cache/uv
- name: UV_TOOL_DIR
  value: /opt/data/home/.local/share/uv/tools
- name: UV_TOOL_BIN_DIR
  value: /opt/data/home/.local/bin
- name: NPM_CONFIG_CACHE
  value: /opt/data/home/.npm
- name: PATH
  value: /opt/data/bin:/opt/data/mise/shims:/opt/data/home/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
{{- end -}}

{{- define "openagent.mcpVerifyVolumes" -}}
- name: data
  persistentVolumeClaim:
    claimName: openagent-hermes-agent
- name: mcp-verify
  configMap:
    name: openagent-mcp-manifest
{{- end -}}
