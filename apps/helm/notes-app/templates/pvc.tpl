{{- range .Values.environments }}
{{- $env := . }}
{{- range $serviceName, $service := .Values.services }}
{{- if $service.storage.enabled }}
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{ $env.name }}-{{ $serviceName }}-pvc
  namespace: {{ $env.name }}
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: {{ $service.storage.size }}
  storageClassName: {{ .Values.storageClass }}
{{- end }}
{{- end }}
{{- end }}
