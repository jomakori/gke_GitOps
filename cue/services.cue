package main

// ── Raw service data (name, namespace, chart.name, chart.alias derived from key) ──

_serviceData: [string]: {
	enabled?:    bool
	syncWave?:   int
	description?: string
	chart: {
		repository: string
		name?:      string
		version?:   string
		alias?:     string
	}
	values?: {...}
	mesh?: { mtls: "STRICT" | "PERMISSIVE" | "DISABLE" }
}

_serviceData: "external-secrets": {
	syncWave: 1
	chart: {
		repository: "https://charts.external-secrets.io/"
		version:    "2.5.0"
	}
	values: {
		installCRDs: true
		replicaCount: 2
		serviceAccount: create: true
	}
}

_serviceData: "kube-prometheus-stack": {
	syncWave: 1
	chart: {
		repository: "https://prometheus-community.github.io/helm-charts"
		version:    "85.0.3"
	}
	values: {
		defaultRules: {
			create: true
			rules: etcd: false, kubeScheduler: false
		}
		alertmanager: service: port: 15010
		kubeControllerManager: enabled: false
		kubeEtcd:               enabled: false
		kubeSchedulerAlerting:  enabled: false
		kubeSchedulerRecording: enabled: false
	}
}

_serviceData: "keda": {
	chart: {
		repository: "https://kedacore.github.io/charts"
		version:    "2.19.0"
	}
	values: keda: {
		serviceAccount: { create: true, name: "keda" }
		metricsServer:  enabled: true
		webhooks:       enabled: true
	}
}

_serviceData: "metrics-server": {
	chart: {
		repository: "https://kubernetes-sigs.github.io/metrics-server"
		version:    "3.13.0"
	}
	values: "metrics-server": {
		replicas:       3
		containerPort:  10250
		podDisruptionBudget: { enabled: true, maxUnavailable: 1 }
		args: ["--kubelet-insecure-tls"]
	}
}

_serviceData: "mongodb": {
	chart: { repository: "" }
	let _sc = storageClass
	values: storageClass: _sc
}

_serviceData: "headlamp": {
	chart: {
		repository: "https://kubernetes-sigs.github.io/headlamp/"
		version:    "0.42.0"
	}
	values: headlamp: {
		clusterRoleBinding: { create: true, clusterRoleName: "cluster-admin" }
		initContainers: [{
			command: ["/bin/sh", "-c", "mkdir -p /build/plugins && cp -r /plugins/* /build/plugins/"]
			image:   "quay.io/kubescape/headlamp-plugin:latest"
			name:    "kubescape-plugin"
			volumeMounts: [{ mountPath: "/build/plugins", name: "headlamp-plugins" }]
		}]
		volumeMounts: [{ name: "headlamp-plugins", mountPath: "/build/plugins" }]
		volumes:     [{ name: "headlamp-plugins", emptyDir: {} }]
	}
}

_serviceData: "opencost": {
	chart: {
		repository: "https://opencost.github.io/opencost-helm-chart"
		version:    "2.5.15"
	}
	values: opencost: {
		networkPolicies: {
			prometheus: {
				namespace: "kube-prometheus-stack"
				port:      9090
				labels:    "app.kubernetes.io/name": "prometheus"
			}
		}
		exporter: {
			defaultClusterId: null
			extraEnv: {
				EMIT_KSM_V1_METRICS:      "false"
				EMIT_KSM_V1_METRICS_ONLY: "true"
				LOG_LEVEL:                 "warn"
			}
		}
		prometheus: internal: {
			enabled:       true
			serviceName:   "kube-prometheus-stack-prometheus"
			namespaceName: "kube-prometheus-stack"
		}
		ui:       enabled: true
		metrics:  serviceMonitor: enabled: false
		nodeSelector: intent: "apps"
	}
}

_serviceData: "redis-operator": {
	enabled: false
	chart: {
		repository: "https://charts.bitnami.com/bitnami"
		name:       "redis-cluster"
		version:    "13.0.4"
		alias:      "redis-operator"
	}
	values: "redis-cluster": {
		global: redis: password: ""
		cluster: nodes: 6
		redis: {
			resourcesPreset: "none"
			resources: requests: { cpu: "512m", memory: "1Gi" }
			nodeSelector: intent: "apps"
		}
		updateJob: nodeSelector: intent: "apps"
		persistence: {
			storageClass: null
			size:         "50Gi"
		}
	}
}

_serviceData: "generic-device-plugin": {
	chart: {
		repository: "https://charts.gabe565.com"
		version:    "0.1.3"
	}
	values: "generic-device-plugin": {
		image: { repository: "ghcr.io/squat/generic-device-plugin", pullPolicy: "IfNotPresent", tag: "latest" }
		controller: type: "daemonset"
		securityContext: privileged: true
		env: DOMAIN: "squat.ai"
		persistence: {
			"device-plugins": { enabled: true, type: "hostPath", hostPath: "/var/lib/kubelet/device-plugins" }
			dev:           { enabled: true, type: "hostPath", hostPath: "/dev" }
		}
		config: {
			enabled: true
			data: """
				devices:
				  - name: dri
				    groups:
				      - count: 4
				        paths:
				          - path: /dev/dri
				"""
		}
		service: main: ports: http: port: 8080
		probes: {
			liveness:  { type: "HTTP", path: "/health" }
			readiness: { type: "HTTP", path: "/health" }
			startup:   { type: "HTTP", path: "/health" }
		}
	}
}

_serviceData: "ramalama": {
	chart: { repository: "" }
	values: {
		image:  "quay.io/ramalama/ramalama"
		tag:    "latest"
		replicas: 1
		model: {
			url:   "https://huggingface.co/bartowski/Qwen_Qwen3-4B-GGUF/resolve/main/Qwen_Qwen3-4B-Q4_K_M.gguf"
			alias: "qwen3"
		}
		gpuResourceLimit: 1
		service: port: 8080
	}
}

_serviceData: "db-operator": {
	chart: {
		repository: "https://stackgres.io/downloads/stackgres-k8s/stackgres/helm/"
		name:       "stackgres-operator"
		version:    "1.18.6"
		alias:      "db-operator"
	}
	values: stackgresOperator: {
		adminui: service: { exposeHTTP: false, type: "ClusterIP" }
		cert: {
			autoapprove:         true
			createForOperator:   true
			createForWebApi:     true
			createForCollector:  true
			regenerateCert:      true
			certDuration:        730
			regenerateWebCert:   true
			regenerateWebRsa:    true
		}
		rbac: create: true
		authentication: {
			type: "jwt"
			createAdminSecret: true
			user:   null
			password: null
		}
		grafana: {
			autoEmbed: true
			schema:    "http"
			user:      null
			password:  null
		}
		extensions: {
			repositoryUrls: ["https://extensions.stackgres.io/postgres/repository"]
			cache: {
				enabled: true
				preloadedExtensions: ["x86_64/linux/timescaledb-1\\.7\\.4-pg12"]
				persistentVolume: {
					size:         "1Gi"
					accessModes: ["ReadWriteOnce"]
					storageClass: "local-path"
				}
			}
		}
		backupStorage: enabled: false
		clusters: environments: [
			{ name: "staging", storageSize: "1Gi" },
			{ name: "production", storageSize: "2Gi" },
		]
	}
}

// ── Enriched: name/namespace from key, chart.name + chart.alias default to key ──

_services: {
	for key, s in _serviceData {
		(key): #Service & {
			name:      key
			namespace: key
			chart: {
				name:  s.chart.name | *key
				alias: s.chart.alias | *key
			}
		} & s
	}
}
