# argo-cd

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v3.6.1](https://img.shields.io/badge/AppVersion-v3.6.1-informational?style=flat-square)
ArgoCD managing itself — thin wrapper over the official argo-cd Helm chart, adopted from the Terraform-installed release. Production HA values live in values.yaml.

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://argoproj.github.io/argo-helm | argo-cd | 10.19.1 |

## Under the hood

A thin wrapper around the official [argo-cd Helm chart](https://github.com/argoproj/argo-helm/tree/main/charts/argo-cd): no local templates, only production overrides (HA replicas, PDBs, autoscaling, resource limits, hard pod anti-affinity, custom health checks for `ExternalSecret` and `CronJob`, `NodePort` server Service) under the upstream `argo-cd:` key. The values were ported from the Terraform install with two deliberate changes: `server.insecure` is **false** (the server terminates TLS; the Istio route must target 443 with TLS origination) and `global.domain` is `argocd.maklab.net` (drives `argocd-cm` `url`). See the cutover doc.

### Self-management

This chart is deployed by the `argoCd` registry entry in `services/argocd-appset/values.yaml` — ArgoCD syncing its own release. The entry ships **disabled**; see [`docs/argocd-self-manage.md`](../../../docs/argocd-self-manage.md) for the cutover order and the guard rails (no finalizer, no prune, `selfHeal` off until verified).

### Do not rename

The Application is named `argo-cd`, which is the Helm release name and therefore the prefix of every Service (`argo-cd-argocd-server`, ...). The Istio VirtualService for `argocd.maklab.net` and the Terraform-owned root apps and webhook pin those names.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| argo-cd.applicationSet.pdb.enabled | bool | `true` |  |
| argo-cd.applicationSet.pdb.minAvailable | int | `1` |  |
| argo-cd.applicationSet.replicas | int | `2` |  |
| argo-cd.applicationSet.resources.limits.memory | string | `"256Mi"` |  |
| argo-cd.applicationSet.resources.requests.cpu | string | `"250m"` |  |
| argo-cd.applicationSet.resources.requests.memory | string | `"192Mi"` |  |
| argo-cd.configs.cm."exec.enabled" | bool | `true` |  |
| argo-cd.configs.cm."helm.timeout" | int | `300` |  |
| argo-cd.configs.cm."resource.customizations.health.batch_CronJob" | string | `"hs = {}\n-- Suspended CronJobs do not run; surface them as Suspended.\nif obj.spec ~= nil and obj.spec.suspend == true then\n  hs.status = \"Suspended\"\n  hs.message = \"CronJob is suspended\"\n  return hs\nend\n-- No status yet: the CronJob has not been scheduled.\nif obj.status == nil or obj.status.lastScheduleTime == nil then\n  hs.status = \"Healthy\"\n  hs.message = \"CronJob has not been scheduled yet\"\n  return hs\nend\n-- The CronJob has been scheduled. If the last run did not succeed, report\n-- Healthy (not Degraded, not Progressing) so ArgoCD keeps the app in sync\n-- and keeps retrying; the drift signal stays visible in the Job's logs.\nif obj.status.lastSuccessfulTime == nil or obj.status.lastSuccessfulTime < obj.status.lastScheduleTime then\n  hs.status = \"Healthy\"\n  hs.message = \"Last scheduled run did not succeed — check Job logs (drift?)\"\n  return hs\nend\nhs.status = \"Healthy\"\nhs.message = \"CronJob completed its last scheduled run successfully\"\nreturn hs\n"` |  |
| argo-cd.configs.cm."resource.customizations.health.external-secrets.io_ExternalSecret" | string | `"hs = {}\nif obj.status ~= nil then\n  for _, condition in ipairs(obj.status.conditions or {}) do\n    if condition.type == \"Ready\" then\n      if condition.status == \"True\" then\n        hs.status = \"Healthy\"\n        hs.message = condition.message\n      else\n        hs.status = \"Degraded\"\n        hs.message = condition.message\n      end\n    end\n  end\nend\nif hs.status == nil then\n  hs.status = \"Progressing\"\n  hs.message = \"Waiting for ExternalSecret to sync\"\nend\nreturn hs\n"` |  |
| argo-cd.configs.cm.create | bool | `true` |  |
| argo-cd.configs.params."server.insecure" | bool | `false` |  |
| argo-cd.controller.pdb.enabled | bool | `true` |  |
| argo-cd.controller.pdb.minAvailable | int | `1` |  |
| argo-cd.controller.replicas | int | `2` |  |
| argo-cd.controller.resources.limits.memory | string | `"3Gi"` |  |
| argo-cd.controller.resources.requests.cpu | string | `"500m"` |  |
| argo-cd.controller.resources.requests.memory | string | `"1Gi"` |  |
| argo-cd.dex.pdb.enabled | bool | `true` |  |
| argo-cd.dex.pdb.minAvailable | int | `1` |  |
| argo-cd.dex.resources.limits.memory | string | `"256Mi"` |  |
| argo-cd.dex.resources.requests.cpu | string | `"100m"` |  |
| argo-cd.dex.resources.requests.memory | string | `"128Mi"` |  |
| argo-cd.global.domain | string | `"argocd.maklab.net"` |  |
| argo-cd.global.podAntiAffinity | string | `"hard"` |  |
| argo-cd.notifications.pdb.enabled | bool | `true` |  |
| argo-cd.notifications.pdb.minAvailable | int | `1` |  |
| argo-cd.notifications.resources.limits.memory | string | `"192Mi"` |  |
| argo-cd.notifications.resources.requests.cpu | string | `"100m"` |  |
| argo-cd.notifications.resources.requests.memory | string | `"128Mi"` |  |
| argo-cd.redis.pdb.enabled | bool | `true` |  |
| argo-cd.redis.pdb.minAvailable | int | `1` |  |
| argo-cd.redis.resources.limits.memory | string | `"256Mi"` |  |
| argo-cd.redis.resources.requests.cpu | string | `"250m"` |  |
| argo-cd.redis.resources.requests.memory | string | `"64Mi"` |  |
| argo-cd.repoServer.autoscaling.enabled | bool | `true` |  |
| argo-cd.repoServer.autoscaling.maxReplicas | int | `4` |  |
| argo-cd.repoServer.autoscaling.minReplicas | int | `2` |  |
| argo-cd.repoServer.autoscaling.targetCPUUtilizationPercentage | int | `70` |  |
| argo-cd.repoServer.autoscaling.targetMemoryUtilizationPercentage | int | `70` |  |
| argo-cd.repoServer.pdb.enabled | bool | `true` |  |
| argo-cd.repoServer.pdb.minAvailable | int | `1` |  |
| argo-cd.repoServer.replicas | int | `2` |  |
| argo-cd.repoServer.resources.limits.memory | string | `"768Mi"` |  |
| argo-cd.repoServer.resources.requests.cpu | string | `"500m"` |  |
| argo-cd.repoServer.resources.requests.memory | string | `"512Mi"` |  |
| argo-cd.server.autoscaling.enabled | bool | `true` |  |
| argo-cd.server.autoscaling.maxReplicas | int | `4` |  |
| argo-cd.server.autoscaling.minReplicas | int | `2` |  |
| argo-cd.server.autoscaling.targetCPUUtilizationPercentage | int | `70` |  |
| argo-cd.server.autoscaling.targetMemoryUtilizationPercentage | int | `70` |  |
| argo-cd.server.ingress.enabled | bool | `false` |  |
| argo-cd.server.ingressGrpc.enabled | bool | `false` |  |
| argo-cd.server.pdb.enabled | bool | `true` |  |
| argo-cd.server.pdb.minAvailable | int | `1` |  |
| argo-cd.server.replicas | int | `2` |  |
| argo-cd.server.resources.limits.memory | string | `"384Mi"` |  |
| argo-cd.server.resources.requests.cpu | string | `"250m"` |  |
| argo-cd.server.resources.requests.memory | string | `"256Mi"` |  |
| argo-cd.server.service.labels.app | string | `"argocd-server"` |  |
| argo-cd.server.service.type | string | `"NodePort"` |  |
