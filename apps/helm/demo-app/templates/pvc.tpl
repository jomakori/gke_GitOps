{{- range .Values.environments }}
{{- $env := . }}
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{ $env.name }}-pvc
  namespace: {{ $env.name }}
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: {{ $env.demoapp.storage.size }}
  storageClassName: {{ .Values.storageClass }}
{{- end }}
