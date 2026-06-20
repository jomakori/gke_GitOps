# onedev

![Version: 15.0.8](https://img.shields.io/badge/Version-15.0.8-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 15.0.8](https://img.shields.io/badge/AppVersion-15.0.8-informational?style=flat-square)

All-In-One DevOps Platform

**Homepage:** <https://onedev.io>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| Robin Shen | <robin@onedev.io> |  |
| Abdul Khaliq | <a.khaliq@outlook.my> |  |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Configure [affinity and anti-affinity](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#affinity-and-anti-affinity). |
| args | list | `[]` | Override default image arguments. |
| command | list | `[]` | Override default image command. |
| database.external | bool | `false` | Set to **true** when using external database |
| database.host | string | `"onedev-mysql.onedev.svc.cluster.local"` | IP address or hostname of database |
| database.maximumPoolSize | string | `"25"` | Database maximum pool size |
| database.name | string | `"onedev"` | Name of the database |
| database.password | string | `"changeit"` | Database password  |
| database.port | string | `"3306"` | Port Number |
| database.type | string | `"mysql"` |  |
| database.user | string | `"root"` | User with access to database |
| dnsConfig | object | `{}` | Specify the [dnsConfig](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#pod-dns-config). |
| dnsPolicy | string | `"ClusterFirst"` | Specify the [dnsPolicy](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#pod-s-dns-policy). |
| env | list | `[]` | Define additional environment variables. |
| envFrom | list | `[]` | Define environment variables from ConfigMap or Secret data. |
| extraContainers | list | `[]` | Specify extra Containers to be added. |
| extraVolumeMounts | list | `[]` | Specify Additional VolumeMounts to use. |
| extraVolumes | list | `[]` | Specify additional Volumes to use. |
| global.commonLabels | object | `{}` | To apply labels to all resources. |
| global.fullnameOverride | string | `""` | Override the fully qualified app name. |
| global.nameOverride | string | `""` | Override the name of the app. |
| image.name | string | `"1dev/server"` | Specify the image name to use (relative to `image.repository`). |
| image.pullPolicy | string | `"IfNotPresent"` | Specify the [pullPolicy](https://kubernetes.io/docs/concepts/containers/images/#image-pull-policy). |
| image.pullSecrets | list | `[]` | Specify the [imagePullSecrets](https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod). |
| image.repository | string | `"docker.io"` | Specify the image repository to use. |
| image.tag | string | `""` | Specify the image tag to use. Leave empty to use same version as chart |
| ingress.annotations | object | `{"nginx.ingress.kubernetes.io/affinity":"cookie","nginx.ingress.kubernetes.io/affinity-mode":"persistent","nginx.ingress.kubernetes.io/proxy-body-size":"0","nginx.ingress.kubernetes.io/session-cookie-expires":"172800","nginx.ingress.kubernetes.io/session-cookie-max-age":"172800","nginx.ingress.kubernetes.io/session-cookie-name":"session-sticky"}` | Specify annotations for the Ingress. |
| ingress.className | string | `""` | Specify ingress class name. Requires Kubernetes >= 1.18. Refer to https://kubernetes.io/blog/2020/04/02/improvements-to-the-ingress-api-in-kubernetes-1.18/#specifying-the-class-of-an-ingress for details |
| ingress.enabled | bool | `false` | If **true**, create an Ingress resource. |
| ingress.host | string | `"onedev.example.com"` | Set the host name |
| ingress.tls | object | `{"acme":{"email":"you@example.com","enabled":false,"production":false,"type":"letsencrypt"},"enabled":false,"secretName":"onedev-tls"}` | Configure TLS for the Ingress. |
| ingress.tls.acme.email | string | `"you@example.com"` | Email to send certificate status notice |
| ingress.tls.acme.enabled | bool | `false` | If **true**, create certificate issuer resource |
| ingress.tls.acme.production | bool | `false` | If **true**, generate a production certificate; otherwise generate staging certificate. Set this to **false** to avoid reaching rate limit while you are testing your setup |
| ingress.tls.acme.type | string | `"letsencrypt"` | Specify ACME type. Currently only supports letsencrypt |
| initContainers | list | `[]` | Specify initContainers to be added. |
| lifecycle | object | `{}` | Specify lifecycle hooks for Containers. |
| livenessProbe | object | `{}` | Specify the livenessProbe [configuration](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/#configure-probes). |
| nodeSelector | object | `{}` | Configure [nodeSelector](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#nodeselector). |
| onedev.hibernate.queryPlanCacheMaxSize | string | `"2048"` |  |
| onedev.hibernate.queryPlanParameterMetadataMaxSize | string | `"128"` |  |
| onedev.initSettings.email | string | `""` | Administrator Email address. Leave empty to prompt |
| onedev.initSettings.password | string | `""` | Administrator Password. Leave empty to prompt |
| onedev.initSettings.serverUrl | string | `""` | Server url. Leave empty to prompt. Will be ignored if ingress.host is specified |
| onedev.initSettings.sshRootUrl | string | `""` | Root url to clone repository via SSH. Leave empty to derive from server url |
| onedev.initSettings.user | string | `""` | Administrator username. Leave empty to prompt |
| onedev.jvm.maxMemoryPercent | string | `"50"` |  |
| onedev.jvm.threadStackSize | string | `"1024k"` |  |
| onedev.replicas | int | `1` | Number of OneDev servers to run. With an enterprise license, you will be able to distribute different projects to different servers for horizontal scaling, as well as replicate project repositories and artifacts for high availability. Note that setting this param to 2 or more is only meaningful if you are connecting to external database; otherwise they simply run as independent OneDev instances |
| onedev.trustCerts.secretName | string | `"onedev-trustcerts"` | Name of an existing secret containing trusted certificates. You may create the secret with "kubectl create secret generic onedev-trustcerts -n onedev --from-file=/path/to/trust-certs", where /path/to/trust-certs is a local directory contains all certificates to be trusted. Certificate should be  of base64 encoded PEM format beginning with "-----BEGIN CERTIFICATE-----" and ending with "-----END CERTIFICATE-----" |
| onedev.updateStrategy | object | `{"type":"RollingUpdate"}` | Valid options: `RollingUpdate`, `OnDelete` |
| persistence.accessModes | string | `"ReadWriteOnce"` | Specify the accessModes for PersistentVolumeClaims. |
| persistence.selector | object | `{}` | Specify the selectors for PersistentVolumeClaims. |
| persistence.size | string | `"100Gi"` | Specify the size of PersistentVolumeClaims. |
| persistence.storageClassName | string | `""` | Specify the storageClassName for PersistentVolumeClaims. |
| podAnnotations | object | `{}` | Set annotations on Pods. |
| podHostNetwork | bool | `false` | Enable the hostNetwork option on Pods. |
| podLabels | object | `{}` | Set labels on Pods. |
| podPriorityClassName | string | `""` | Set the [priorityClassName](https://kubernetes.io/docs/concepts/scheduling-eviction/pod-priority-preemption/#priorityclass). |
| podSecurityContext | object | `{}` | Allows you to overwrite the default [PodSecurityContext](https://kubernetes.io/docs/tasks/configure-pod-container/security-context/). |
| readinessProbe | object | `{}` | Specify the readinessProbe [configuration](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/#configure-probes). |
| resources | object | `{}` | Specify resource requests and limits. |
| securityContext | object | `{}` | Specify securityContext for Containers. |
| service.annotations | object | `{}` | Specify annotations for the Service. |
| service.externalTrafficPolicy | string | `""` | Specify the [externalTrafficPolicy](https://kubernetes.io/docs/tasks/access-application-cluster/create-external-load-balancer/#preserving-the-client-source-ip). |
| service.ipFamilies | list | `[]` | Configure [IPv4/IPv6 dual-stack](https://kubernetes.io/docs/concepts/services-networking/dual-stack/). |
| service.ipFamilyPolicy | string | `""` | Configure [IPv4/IPv6 dual-stack](https://kubernetes.io/docs/concepts/services-networking/dual-stack/). |
| service.loadBalancerIP | string | `""` | Required: If service type is loadbalancer  [loadBalancerIP](https://kubernetes.io/docs/concepts/services-networking/service/#loadbalancer). |
| service.nodePort | string | `""` | Specify a nodePort for servcie |
| service.ports | object | `{"http":"","ssh":""}` | Manually change the ServicePorts |
| service.separateSSH.annotations | object | `{}` |  |
| service.separateSSH.clusterIP | string | `""` |  |
| service.separateSSH.enabled | bool | `false` | If separate SSH is enabled, a separate service is created for SSH |
| service.separateSSH.externalIPs | list | `[]` |  |
| service.separateSSH.externalTrafficPolicy | string | `""` |  |
| service.separateSSH.ipFamilies | list | `[]` |  |
| service.separateSSH.ipFamilyPolicy | string | `""` |  |
| service.separateSSH.loadBalancerIP | string | `""` |  |
| service.separateSSH.loadBalancerSourceRanges | list | `[]` |  |
| service.separateSSH.nodePort | string | `""` |  |
| service.separateSSH.port | string | `""` |  |
| service.separateSSH.topologyKeys | list | `[]` |  |
| service.separateSSH.type | string | `"ClusterIP"` |  |
| service.topologyKeys | array | `[]` | Specify the [topologyKeys](https://kubernetes.io/docs/concepts/services-networking/service-topology/#using-service-topology). |
| service.type | string | `"ClusterIP"` | Specify the type for the Service. ClusterIP, LoadBalancer |
| serviceAccount.annotations | object | `{}` | Annotations to add to the ServiceAccount, if `serviceAccount.create` is **true**. |
| serviceAccount.create | bool | `true` | If **true**, create a ServiceAccount. |
| serviceAccount.name | string | `"onedev"` | Specify a pre-existing ServiceAccount to use if `serviceAccount.create` is **false**. |
| terminationGracePeriodSeconds | int | `60` | Override terminationGracePeriodSeconds. |
| tolerations | list | `[]` | Configure [taints and tolerations](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/). |
| topologySpreadConstraints | list | `[]` | Configure [topology spread constraints](https://kubernetes.io/docs/concepts/scheduling-eviction/topology-spread-constraints/). |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
