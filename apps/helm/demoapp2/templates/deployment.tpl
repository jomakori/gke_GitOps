{{- range .Values.environments }}
{{- $env := . }}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demoapp-{{ $env.rollout }}
  namespace: {{ $env.name }}
  labels:
    app: demoapp
    env: {{ $env.rollout }}
spec:
  replicas: {{ $env.demoapp.replicas }}
  revisionHistoryLimit: 3
  selector:
    matchLabels:
      app: demoapp
      env: {{ $env.rollout }}
  template:
    metadata:
      labels:
        app: demoapp

        env: {{ $env.rollout }}
    spec:
      nodeSelector:
        intent: apps
      serviceAccountName: {{ $env.name }}-sa
      imagePullSecrets:
        - name: {{ $env.name }}-registry
      containers:
        - name: demoapp
          image: {{ $env.image.repository }}:{{ $env.image.tag }}
          ports:
          - containerPort: {{ $env.demoapp.port }}
          command:
            - /bin/sh
            - -c
            - /server
          resources:
            requests:
              memory: {{ $env.demoapp.resourceRequests.memory }}
              cpu: {{ $env.demoapp.resourceRequests.cpu }}
          envFrom:
          - secretRef:
              name: {{ $env.name }}-vars
{{- end }}
