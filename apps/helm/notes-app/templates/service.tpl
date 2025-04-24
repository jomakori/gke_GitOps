{{- range .Values.environments }}
{{- $env := . }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ $env.name }}-api
  namespace: {{ $env.name }}
  annotations:
  labels:
    app: {{ $env.name }}-api
    env: {{ $env.name }}
spec:
  selector:
    app: {{ $env.name }}-api
    env: {{ $env.name }}
  ports:
  - name: http
    protocol: TCP
    port: 80
    targetPort: {{ $env.backend.port }}
  - name: https
    protocol: TCP
    port: 443
    targetPort: {{ $env.backend.port }}
  type: ClusterIP
---
apiVersion: v1
kind: Service
metadata:
  name: {{ $env.name }}-ui
  namespace: {{ $env.name }}
  annotations:
  labels:
    app: {{ $env.name }}-ui
    env: {{ $env.name }}
spec:
  externalTrafficPolicy: Local # TODO: Confirm if this is needed
  selector:
    app: {{ $env.name }}-ui
    env: {{ $env.name }}
  ports:
  - name: http
    protocol: TCP
    port: 80
    targetPort: {{ $env.frontend.port }}
  - name: https
    protocol: TCP
    port: 443
    targetPort: {{ $env.frontend.port }}
  type: ClusterIP
{{- end }}
