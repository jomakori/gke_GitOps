{{- range .Values.environments }}
{{- $env := . }}
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ $env.name }}-api-ingress
  namespace: {{ $env.name }}
  annotations:
    {{- /* Ingress Core Settings */}}
    kubernetes.io/ingress.class: alb
    alb.ingress.kubernetes.io/target-type: ip
    alb.ingress.kubernetes.io/scheme: internet-facing
    {{- /* Ingress SSL/TlS Settings */}}
    alb.ingress.kubernetes.io/certificate-arn: "arn:aws:acm:us-east-2:123456:certificate/6e1523fd-ced9-4d7e-abe8-0ed7e0d63fac"
    alb.ingress.kubernetes.io/ssl-policy: "ELBSecurityPolicy-TLS13-1-2-2021-06"
    alb.ingress.kubernetes.io/listen-ports: '[{"HTTP": 80}, {"HTTPS": 443}]'
    alb.ingress.kubernetes.io/ssl-redirect: '443'
  labels:
    app: {{ $env.name }}-api
    env: {{ $env.name }}
spec:
  rules:
    - host: {{ $env.apiHost }}
      http:
        paths:
        - path: /
          pathType: Prefix
          backend:
            service:
              name: {{ $env.name }}-api
              port:
                number: 80
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ $env.name }}-ui-ingress
  annotations:
    {{- /* Ingress Core Settings */}}
    kubernetes.io/ingress.class: alb
    alb.ingress.kubernetes.io/target-type: ip
    alb.ingress.kubernetes.io/scheme: internet-facing
    {{- /* Ingress SSL/TlS Settings */}}
    alb.ingress.kubernetes.io/certificate-arn: "arn:aws:acm:us-east-2:123456:certificate/6e1523fd-ced9-4d7e-abe8-0ed7e0d63fac"
    alb.ingress.kubernetes.io/ssl-policy: "ELBSecurityPolicy-TLS13-1-2-2021-06"
    alb.ingress.kubernetes.io/listen-ports: '[{"HTTP": 80}, {"HTTPS": 443}]'
    alb.ingress.kubernetes.io/ssl-redirect: '443'
    # health checks
    alb.ingress.kubernetes.io/success-codes: 200,401
  namespace: {{ $env.name }}
  labels:
    app: {{ $env.name }}-ui
    env: {{ $env.name }}
spec:
  rules:
    - host: {{ $env.uiHost }}
      http:
        paths:
        - path: /
          pathType: Prefix
          backend:
            service:
              name: {{ $env.name }}-ui
              port:
                number: 80
{{- end }}
