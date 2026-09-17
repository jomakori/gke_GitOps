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

{{- define "openagent.toolsImage" -}}
{{- $tag := required "tools.image.tag: pin a published gitopsctl commit sha" .Values.tools.image.tag -}}
{{- printf "%s:%s" .Values.tools.image.repository $tag -}}
{{- end -}}

{{/*
Environment, volumes, init containers, and mounts shared by the MCP preflight Job
and drift CronJob. The tools image provides the gitopsctl binary via install-gitopsctl
initContainer; the hermes image + PVC provide the full toolchain for servers that need
/opt/data (npx/uvx/deno/obscura/bw/mise).
*/}}
{{- define "openagent.mcpVerifyEnv" -}}
- name: HOME
  value: /opt/data/home
- name: MISE_CONFIG_FILE
  value: /mise/mise.toml
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
- name: tools
  emptyDir: {}
- name: mise-config
  configMap:
    name: openagent-hermes-mise-config
- name: mcp-verify
  configMap:
    name: openagent-mcp-manifest
{{- end -}}

{{- define "openagent.mcpVerifyInitContainers" -}}
# Copies the gitopsctl verifier binary out of the prebuilt tools image into
# the shared emptyDir (the image is FROM scratch: no shell, no cp).
- name: install-gitopsctl
  image: {{ include "openagent.toolsImage" . | quote }}
  imagePullPolicy: {{ .Values.tools.image.pullPolicy }}
  args: ["install", "/opt/tools/gitopsctl"]
  securityContext:
    runAsUser: 10000
    runAsGroup: 10000
  volumeMounts:
    - name: tools
      mountPath: /opt/tools
# PostSync can beat the gateway's first boot; wait (bounded) until the boot
# shim has pre-warm exposed the mise toolchain (npx) on the PVC.
- name: wait-for-toolchain
  image: "{{ include "openagent.hermesImage" . }}"
  imagePullPolicy: {{ (index .Values "hermes-agent" "image").pullPolicy }}
  securityContext:
    runAsUser: 10000
    runAsGroup: 10000
  env:
    {{- include "openagent.mcpVerifyEnv" . | nindent 4 }}
  command:
    - sh
    - -c
    - |
      for _ in $(seq 1 {{ div .Values.mcpVerification.preflight.toolchainWaitSeconds 5 }}); do
        [ -x /opt/data/bin/npx ] && exit 0
        sleep 5
      done
      echo "MCP toolchain not ready after {{ .Values.mcpVerification.preflight.toolchainWaitSeconds }}s" >&2
      exit 1
  volumeMounts:
    - name: data
      mountPath: /opt/data
{{- end -}}

{{- define "openagent.mcpVerifyMounts" -}}
- name: data
  mountPath: /opt/data
- name: tools
  mountPath: /opt/tools
  readOnly: true
- name: mise-config
  mountPath: /mise
  readOnly: true
- name: mcp-verify
  mountPath: /mcp-verify
  readOnly: true
{{- end -}}
