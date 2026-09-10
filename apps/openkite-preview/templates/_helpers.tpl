{{- /* ── Helpers ─────────────────────────────────────────────────────── */}}
{{- define "openkite-preview.labels" -}}
app.kubernetes.io/name: {{ .Values.appName | default "openkite-preview" }}
app.kubernetes.io/part-of: openkite
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}
