# litellm-helm

![Version: 1.92.0](https://img.shields.io/badge/Version-1.92.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.92.0](https://img.shields.io/badge/AppVersion-1.92.0-informational?style=flat-square)

Call all LLM APIs using the OpenAI format

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| oci://registry-1.docker.io/bitnamicharts | postgresql | >=13.3.0 |
| oci://registry-1.docker.io/bitnamicharts | redis | >=18.0.0 |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` |  |
| args | object | `{}` |  |
| autoscaling.enabled | bool | `false` |  |
| autoscaling.maxReplicas | int | `100` |  |
| autoscaling.minReplicas | int | `1` |  |
| autoscaling.targetCPUUtilizationPercentage | int | `80` |  |
| command | object | `{}` |  |
| db.database | string | `"litellm"` |  |
| db.deployStandalone | bool | `true` |  |
| db.endpoint | string | `"localhost"` |  |
| db.readReplicaUrl | string | `""` |  |
| db.secret.endpointKey | string | `""` |  |
| db.secret.name | string | `"postgres"` |  |
| db.secret.passwordKey | string | `"password"` |  |
| db.secret.readReplicaUrlKey | string | `""` |  |
| db.secret.usernameKey | string | `"username"` |  |
| db.url | string | `"postgresql://$(DATABASE_USERNAME):$(DATABASE_PASSWORD)@$(DATABASE_HOST)/$(DATABASE_NAME)"` |  |
| db.useExisting | bool | `false` |  |
| db.useStackgresOperator | bool | `false` |  |
| deploymentAnnotations | object | `{}` |  |
| deploymentLabels | object | `{}` |  |
| deploymentMinReadySeconds | int | `0` |  |
| envVars | object | `{}` |  |
| environmentConfigMaps | list | `[]` |  |
| environmentSecrets | list | `[]` |  |
| extraEnvVars | object | `{}` |  |
| extraResources | list | `[]` |  |
| fullnameOverride | string | `""` |  |
| image.pullPolicy | string | `"Always"` |  |
| image.repository | string | `"ghcr.io/berriai/litellm-database"` |  |
| image.tag | string | `""` |  |
| imagePullSecrets | list | `[]` |  |
| ingress.annotations | object | `{}` |  |
| ingress.className | string | `"nginx"` |  |
| ingress.enabled | bool | `false` |  |
| ingress.hosts[0].host | string | `"api.example.local"` |  |
| ingress.hosts[0].paths[0].path | string | `"/"` |  |
| ingress.hosts[0].paths[0].pathType | string | `"ImplementationSpecific"` |  |
| ingress.labels | object | `{}` |  |
| ingress.tls | list | `[]` |  |
| keda.behavior | object | `{}` |  |
| keda.cooldownPeriod | int | `300` |  |
| keda.enabled | bool | `false` |  |
| keda.maxReplicas | int | `100` |  |
| keda.minReplicas | int | `1` |  |
| keda.pollingInterval | int | `30` |  |
| keda.restoreToOriginalReplicaCount | bool | `false` |  |
| keda.scaledObject.annotations | object | `{}` |  |
| keda.triggers | list | `[]` |  |
| lifecycle | object | `{}` |  |
| livenessProbe.failureThreshold | int | `5` |  |
| livenessProbe.initialDelaySeconds | int | `0` |  |
| livenessProbe.path | string | `"/health/liveliness"` |  |
| livenessProbe.periodSeconds | int | `15` |  |
| livenessProbe.successThreshold | int | `1` |  |
| livenessProbe.timeoutSeconds | int | `5` |  |
| logLevel | string | `"INFO"` |  |
| masterkeySecretKey | string | `""` |  |
| masterkeySecretName | string | `""` |  |
| migrationJob.annotations | object | `{}` |  |
| migrationJob.backoffLimit | int | `4` |  |
| migrationJob.disableSchemaUpdate | bool | `false` |  |
| migrationJob.enabled | bool | `true` |  |
| migrationJob.extraContainers | list | `[]` |  |
| migrationJob.extraInitContainers | list | `[]` |  |
| migrationJob.hooks.argocd.enabled | bool | `true` |  |
| migrationJob.hooks.helm.enabled | bool | `false` |  |
| migrationJob.resources | object | `{}` |  |
| migrationJob.retries | int | `3` |  |
| migrationJob.serviceAccountName | string | `""` |  |
| migrationJob.ttlSecondsAfterFinished | int | `120` |  |
| nameOverride | string | `"litellm"` |  |
| nodeSelector | object | `{}` |  |
| pdb.annotations | object | `{}` |  |
| pdb.enabled | bool | `false` |  |
| pdb.labels | object | `{}` |  |
| pdb.maxUnavailable | string | `nil` |  |
| pdb.minAvailable | string | `nil` |  |
| podAnnotations | object | `{}` |  |
| podLabels | object | `{}` |  |
| podSecurityContext | object | `{}` |  |
| postgresql.architecture | string | `"standalone"` |  |
| postgresql.auth.database | string | `"litellm"` |  |
| postgresql.auth.password | string | `"NoTaGrEaTpAsSwOrD"` |  |
| postgresql.auth.postgres-password | string | `"NoTaGrEaTpAsSwOrD"` |  |
| postgresql.auth.username | string | `"litellm"` |  |
| proxyConfigMap.create | bool | `true` |  |
| proxy_config.general_settings.master_key | string | `"os.environ/PROXY_MASTER_KEY"` |  |
| proxy_config.model_list[0].litellm_params.api_key | string | `"eXaMpLeOnLy"` |  |
| proxy_config.model_list[0].litellm_params.model | string | `"gpt-3.5-turbo"` |  |
| proxy_config.model_list[0].model_name | string | `"gpt-3.5-turbo"` |  |
| proxy_config.model_list[1].litellm_params.api_base | string | `"https://exampleopenaiendpoint-production.up.railway.app/"` |  |
| proxy_config.model_list[1].litellm_params.api_key | string | `"fake-key"` |  |
| proxy_config.model_list[1].litellm_params.model | string | `"openai/fake"` |  |
| proxy_config.model_list[1].model_name | string | `"fake-openai-endpoint"` |  |
| readinessProbe.failureThreshold | int | `3` |  |
| readinessProbe.initialDelaySeconds | int | `0` |  |
| readinessProbe.path | string | `"/health/readiness"` |  |
| readinessProbe.periodSeconds | int | `10` |  |
| readinessProbe.successThreshold | int | `1` |  |
| readinessProbe.timeoutSeconds | int | `5` |  |
| redis.architecture | string | `"standalone"` |  |
| redis.enabled | bool | `false` |  |
| replicaCount | int | `1` |  |
| resources | object | `{}` |  |
| securityContext | object | `{}` |  |
| service.port | int | `4000` |  |
| service.type | string | `"ClusterIP"` |  |
| serviceAccount.annotations | object | `{}` |  |
| serviceAccount.automount | bool | `true` |  |
| serviceAccount.create | bool | `false` |  |
| serviceAccount.name | string | `""` |  |
| serviceMonitor.annotations | object | `{}` |  |
| serviceMonitor.enabled | bool | `false` |  |
| serviceMonitor.interval | string | `"15s"` |  |
| serviceMonitor.labels | object | `{}` |  |
| serviceMonitor.namespaceSelector.matchNames | list | `[]` |  |
| serviceMonitor.relabelings | list | `[]` |  |
| serviceMonitor.scrapeTimeout | string | `"10s"` |  |
| startupProbe.failureThreshold | int | `30` |  |
| startupProbe.initialDelaySeconds | int | `0` |  |
| startupProbe.path | string | `"/health/readiness"` |  |
| startupProbe.periodSeconds | int | `10` |  |
| startupProbe.successThreshold | int | `1` |  |
| startupProbe.timeoutSeconds | int | `5` |  |
| strategy | object | `{}` | Deployment strategy configuration Example:   type: RollingUpdate   rollingUpdate:     maxUnavailable: 0     maxSurge: 1 |
| terminationGracePeriodSeconds | int | `90` |  |
| tolerations | list | `[]` |  |
| topologySpreadConstraints | list | `[]` |  |
| volumeMounts | list | `[]` |  |
| volumes | list | `[]` |  |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
