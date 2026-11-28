# apps

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![AppVersion: 1.0](https://img.shields.io/badge/AppVersion-1.0-informational?style=flat-square)

Root app for managing all workload apps in cluster

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| apps.openkite.argoProject | string | `"default"` |  |
| apps.openkite.argocdNamespace | string | `"argocd"` |  |
| apps.openkite.destNamespace | string | `"argocd"` |  |
| apps.openkite.enable | bool | `true` |  |
| apps.openkite.finalizers[0] | string | `"resources-finalizer.argocd.argoproj.io"` |  |
| apps.openkite.gateways.enable_public | bool | `true` |  |
| apps.openkite.helmPath | string | `"apps/helm/"` |  |
| apps.openkite.ignoreDifferences[0].group | string | `"apps"` |  |
| apps.openkite.ignoreDifferences[0].jsonPointers[0] | string | `"/spec/replicas"` |  |
| apps.openkite.ignoreDifferences[0].kind | string | `"Deployment"` |  |
| apps.openkite.ignoreDifferences[1].group | string | `"apps"` |  |
| apps.openkite.ignoreDifferences[1].jsonPointers[0] | string | `"/metadata/argocd.argoproj.io/sync-wave"` |  |
| apps.openkite.ignoreDifferences[1].kind | string | `"Secret"` |  |
| apps.openkite.parameters[0].name | string | `"appName"` |  |
| apps.openkite.parameters[0].value | string | `"openkite"` |  |
| apps.openkite.parameters[1].name | string | `"openkite.environments.staging.dopplerConfig"` |  |
| apps.openkite.parameters[1].value | string | `"{{ .Values.openkite.environment.staging.dopplerConfig }}"` |  |
| apps.openkite.parameters[2].name | string | `"openkite.environments.production.dopplerConfig"` |  |
| apps.openkite.parameters[2].value | string | `"{{ .Values.openkite.environment.production.dopplerConfig }}"` |  |
| apps.openkite.parameters[3].name | string | `"openkite.enable_staging"` |  |
| apps.openkite.parameters[3].value | string | `"false"` |  |
| apps.openkite.preview.repo | string | `"openkite"` |  |
| apps.openkite.selfHeal | bool | `true` |  |
| apps.openkite.staging.enable | bool | `true` |  |
| apps.openkite.staging.label | string | `"staging"` |  |
| apps.openkite.staging.tagTemplate | string | `"pr-{{ .number }}-{{ .head_short_sha_7 }}"` |  |
| apps.openkite.syncOptions[0] | string | `"CreateNamespace=true"` |  |
| apps.openkite.syncWave | string | `"6"` |  |
| apps.strands.argoProject | string | `"default"` |  |
| apps.strands.argocdNamespace | string | `"argocd"` |  |
| apps.strands.destNamespace | string | `"argocd"` |  |
| apps.strands.enable | bool | `true` |  |
| apps.strands.finalizers[0] | string | `"resources-finalizer.argocd.argoproj.io"` |  |
| apps.strands.gateways.enable_public | bool | `true` |  |
| apps.strands.helmPath | string | `"apps/helm/"` |  |
| apps.strands.ignoreDifferences[0].group | string | `"apps"` |  |
| apps.strands.ignoreDifferences[0].jsonPointers[0] | string | `"/spec/replicas"` |  |
| apps.strands.ignoreDifferences[0].kind | string | `"Deployment"` |  |
| apps.strands.parameters[0].name | string | `"appName"` |  |
| apps.strands.parameters[0].value | string | `"strands"` |  |
| apps.strands.parameters[1].name | string | `"strands.environments.staging.dopplerConfig"` |  |
| apps.strands.parameters[1].value | string | `"{{ .Values.strands.environment.staging.dopplerConfig }}"` |  |
| apps.strands.parameters[2].name | string | `"strands.environments.production.dopplerConfig"` |  |
| apps.strands.parameters[2].value | string | `"{{ .Values.strands.environment.production.dopplerConfig }}"` |  |
| apps.strands.parameters[3].name | string | `"strands.enable_staging"` |  |
| apps.strands.parameters[3].value | string | `"{{ .Values.strands.environment.staging.enable }}"` |  |
| apps.strands.selfHeal | bool | `true` |  |
| apps.strands.syncOptions[0] | string | `"CreateNamespace=true"` |  |
| apps.strands.syncWave | string | `"6"` |  |
| apps.tickerlight.argoProject | string | `"default"` |  |
| apps.tickerlight.argocdNamespace | string | `"argocd"` |  |
| apps.tickerlight.destNamespace | string | `"argocd"` |  |
| apps.tickerlight.enable | bool | `true` |  |
| apps.tickerlight.finalizers[0] | string | `"resources-finalizer.argocd.argoproj.io"` |  |
| apps.tickerlight.gateways.enable_public | bool | `true` |  |
| apps.tickerlight.helmPath | string | `"apps/helm/"` |  |
| apps.tickerlight.ignoreDifferences[0].group | string | `"apps"` |  |
| apps.tickerlight.ignoreDifferences[0].jsonPointers[0] | string | `"/spec/replicas"` |  |
| apps.tickerlight.ignoreDifferences[0].kind | string | `"Deployment"` |  |
| apps.tickerlight.parameters[0].name | string | `"appName"` |  |
| apps.tickerlight.parameters[0].value | string | `"tickerlight"` |  |
| apps.tickerlight.parameters[1].name | string | `"tickerlight.environments.staging.dopplerConfig"` |  |
| apps.tickerlight.parameters[1].value | string | `"{{ .Values.tickerlight.environment.staging.dopplerConfig }}"` |  |
| apps.tickerlight.parameters[2].name | string | `"tickerlight.environments.production.dopplerConfig"` |  |
| apps.tickerlight.parameters[2].value | string | `"{{ .Values.tickerlight.environment.production.dopplerConfig }}"` |  |
| apps.tickerlight.parameters[3].name | string | `"tickerlight.enable_staging"` |  |
| apps.tickerlight.parameters[3].value | string | `"true"` |  |
| apps.tickerlight.selfHeal | bool | `true` |  |
| apps.tickerlight.staging.enable | bool | `false` |  |
| apps.tickerlight.staging.label | string | `"staging"` |  |
| apps.tickerlight.syncOptions[0] | string | `"CreateNamespace=true"` |  |
| apps.tickerlight.syncWave | string | `"6"` |  |
| argoNamespace | string | `""` |  |
| argoProject | string | `""` |  |
| clusterDomain | string | `""` |  |
| clusterName | string | `""` |  |
| clusterServer | string | `""` |  |
| openkite.environment.production.dopplerConfig | string | `"svc_openagent"` |  |
| openkite.environment.staging.dopplerConfig | string | `"svc_openagent"` |  |
| repoUrl | string | `""` |  |
| storageClass | string | `""` |  |
| strands.environment.production.dopplerConfig | string | `"strands_prd"` |  |
| strands.environment.staging.dopplerConfig | string | `"strands_stg"` |  |
| strands.environment.staging.enable | bool | `false` |  |
| targetRevision | string | `""` |  |
| tickerlight.environment.production.dopplerConfig | string | `"tickerlight_prd"` |  |
| tickerlight.environment.staging.dopplerConfig | string | `"tickerlight_stg"` |  |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
