{{- range .Values.environments }}
# Staging + Production Deployments
{{- $env := . }}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ $env.name }}-api
  namespace: {{ $env.name }}
  labels:
    app: {{ $env.name }}-api
    rev: {{ $env.tag }}
    env: {{ $env.name }}
spec:
  replicas: {{ $env.backend.replicas }}
  revisionHistoryLimit: 3
  selector:
    matchLabels:
      app: {{ $env.name }}-api
      env: {{ $env.name }}
  template:
    metadata:
      labels:
        app: {{ $env.name }}-api
        env: {{ $env.name }}
    spec:
      nodeSelector:
        intent: apps
      serviceAccountName: {{ $env.name }}-sa
      imagePullSecrets:
        - name: {{ $env.name }}-registry
      containers:
        - name: {{ $env.name }}-api
          ports:
          - containerPort: {{ $env.backend.port }}
          image: {{ $env.backend.image }}:{{ $env.tag }}
          imagePullPolicy: Always
          ## Note: already pre-started in the image
          # command:
          #   - /bin/sh
          #   - -c
          #   - /start
          resources:
            requests:
              memory: {{ $env.backend.resourceRequests.memory }}
              cpu: {{ $env.backend.resourceRequests.cpu }}
          envFrom:
          - secretRef:
              name: {{ $env.name }}-vars
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ $env.name }}-ui
  namespace: {{ $env.name }}
  labels:
    app: {{ $env.name }}-ui
    rev: {{ $env.tag }}
    env: {{ $env.name }}
spec:
  replicas: {{ $env.frontend.replicas }}
  revisionHistoryLimit: 3
  selector:
    matchLabels:
      app: {{ $env.name }}-ui
      env: {{ $env.name }}
  template:
    metadata:
      labels:
        app: {{ $env.name }}-ui
        env: {{ $env.name }}
    spec:
      nodeSelector:
        intent: apps
      serviceAccountName: {{ $env.name }}-sa
      imagePullSecrets:
        - name: {{ $env.name }}-registry
      containers:
        - name: {{ $env.name }}-ui
          ports:
          - containerPort: {{ $env.frontend.port }}
          image: {{ $env.frontend.image }}:{{ $env.tag }}
          imagePullPolicy: Always
          ## Note: already pre-started in the image
          # command:
          #   - /bin/sh
          #   - -c
          #   - /start-ui
          resources:
            requests:
              memory: {{ $env.frontend.resourceRequests.memory }}
              cpu: {{ $env.frontend.resourceRequests.cpu }}
          envFrom:
          - secretRef:
              name: {{ $env.name }}-vars
{{- end }}
