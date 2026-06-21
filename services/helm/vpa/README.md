# vpa

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.2.0](https://img.shields.io/badge/AppVersion-1.2.0-informational?style=flat-square)

Vertical Pod Autoscaler - automatically adjusts CPU and memory requests for pods based on actual usage.

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| local |  |  |

## Overview

VPA automatically adjusts the CPU and memory resource requests of running pods. It's ideal for:

- Single-pod deployments where HPA (horizontal scaling) doesn't apply
- Workloads with variable resource needs
- Right-sizing containers based on actual usage

## Components

- **Recommender**: Monitors resource usage and generates recommendations
- **Updater**: Evicts pods to apply new recommendations
- **Admission Controller**: Injects recommended resources into new pods

## Usage

Other charts can create VPA resources by including the VPA CRD:

```yaml
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: my-app
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-app
  updatePolicy:
    updateMode: Auto
  resourcePolicy:
    containerPolicies:
      - containerName: my-container
        minAllowed:
          cpu: 100m
          memory: 256Mi
        maxAllowed:
          cpu: 2000m
          memory: 2Gi
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| vpa.admissionController.enabled | bool | `true` | Enable admission controller |
| vpa.admissionController.extraArgs.v | string | `"2"` | Log level |
| vpa.image.tag | string | `"1.2.0"` | VPA image tag |
| vpa.recommender.enabled | bool | `true` | Enable recommender |
| vpa.recommender.extraArgs.v | string | `"2"` | Log level |
| vpa.replicaCount | int | `1` | Replica count |
| vpa.resources.limits.cpu | string | `"200m"` | CPU limit |
| vpa.resources.limits.memory | string | `"512Mi"` | Memory limit |
| vpa.resources.requests.cpu | string | `"50m"` | CPU request |
| vpa.resources.requests.memory | string | `"128Mi"` | Memory request |
| vpa.updater.enabled | bool | `true` | Enable updater |
| vpa.updater.extraArgs.v | string | `"2"` | Log level |
