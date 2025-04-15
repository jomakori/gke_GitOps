{{- range .Values.environments }}
{{- $env := . }}
---
apiVersion: v1
kind: Service
metadata:
  name: demoapp-{{ $env.rollout }}
  namespace: {{ $env.name }}
  annotations:
  labels:
    app: demoapp
    env: {{ $env.rollout }}
spec:
  selector:
    app: demoapp
    env: {{ $env.rollout }}
  ports:
  - name: http
    protocol: TCP
    port: 80
    targetPort: {{ $env.demoapp.port }}
  - name: https
    protocol: TCP
    port: 443
    targetPort: {{ $env.demoapp.port }}
  type: NodePort
{{- end }}
