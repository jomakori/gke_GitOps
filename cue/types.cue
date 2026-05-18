package main

// ── Service Types ──

#ArgoCDApp: {
	name:     string
	namespace?: string | *"argocd"
	project?:  string | *"default"

	source: {
		repoURL:        string
		path:           string
		targetRevision: string
		helm?: {
			valueFiles?: [...string]
			values?:    string
			parameters?: [...{
				name:  string
				value: string
			}]
		}
	}

	destination: {
		server:    string | *"https://kubernetes.default.svc"
		namespace: string
	}

	syncWave?:       int | *0
	autoSync?:       bool | *true
	selfHeal?:       bool | *true
	prune?:          bool | *true
	ignoreDiffs?: [...{
		group:        string
		kind:         string
		jsonPointers: [...string]
	}]
}

#Service: {
	name:       string
	namespace:  string | *name
	enabled:    bool | *true
	syncWave:   int | *0
	description?: string

	chart: {
		repository: string
		name:       string
		version?:   string
		alias?:     string | *name
	}

	values?: {...}
	mesh?: { mtls: "STRICT" | "PERMISSIVE" | "DISABLE" | *null }
}

// ── App Types ──

#Port: {
	port:     int
	protocol: "HTTP" | "GRPC" | "TCP" | "HTTPS" | *"HTTP"
	name?:    string
}

#Resources: {
	requests?: { cpu: string | *"256m", memory: string | *"256Mi" }
	limits?:   { cpu: string | *"500m", memory: string | *"512Mi" }
}

#Storage: {
	enabled:      bool | *false
	size:         string | *"1Gi"
	storageClass: string | *"csi-hostpath-sc"
}

#CircuitBreaker: {
	maxConnections:     int | *100
	maxRequests:        int | *1000
	maxRetries:         int | *3
	maxPendingRequests: int | *50
}

#TrafficRouting: {
	type:    "canary" | "mirror"
	weight?: int
	mirror?: string
}

#GatewayConfig: {
	host:  string
	paths: [...{
		path:    string
		type:    "PathPrefix" | "Exact" | "Regex" | *"PathPrefix"
		backend: int
	}]
	tls?: { secretName: string }
}

#AppEnvironment: {
	name:      string
	namespace: string
	host:      string
	image?: { repository?: string, tag?: string }
	dopplerProject?: string
	resources?:      #Resources
	replicas?:       int | *2
	storage?:        #Storage
}

#App: {
	name:        string
	namespace:   string
	description?: string
	enabled:     bool | *true

	ports:        [...#Port]
	environments: [...#AppEnvironment]

	mesh: {
		mtls:            "STRICT" | "PERMISSIVE" | "DISABLE" | *"STRICT"
		retries:         int | *3
		timeout:         string | *"30s"
		circuitBreaker:  #CircuitBreaker | *{ maxConnections: 100, maxPendingRequests: 50, maxRequests: 1000, maxRetries: 3 }
		loadBalancer:    "ROUND_ROBIN" | "LEAST_CONN" | "RANDOM" | "PASSTHROUGH" | *"ROUND_ROBIN"
		trafficRouting?: #TrafficRouting
		authorization?:  [...#AuthorizationPolicy]
	}

	gateway?: #GatewayConfig
	resources: #Resources
	dopplerProject?: string | *name
	storage?: #Storage
}

#AuthorizationPolicy: {
	name:   string
	action: "ALLOW" | "DENY" | "CUSTOM" | *"ALLOW"
	rules?: [...{
		from?: [...{ source: { principals?: [...string], namespaces?: [...string], ipBlocks?: [...string] } }]
		to?:   [...{ operation: { methods?: [...string], paths?: [...string], ports?: [...int] } }]
	}]
}

// ── Secret Types ──

#ClusterSecretStore: {
	name:      string
	namespace: string | *"external-secrets"
	dopplerToken: {
		secretName: string | *"doppler-token-auth"
		key:        string | *"dopplerToken"
	}
}

#ExternalSecret: {
	name:             string
	namespace:        string
	refreshInterval:  string | *"10s"
	clusterSecretStore: string | *"doppler-auth"
	targetSecret:     string
	dopplerProject?:  string
	dataFrom?: [...{
		find?:    { name?: { regexp: string }, path?: { regexp: string } }
		extract?: { key: string }
	}]
	data?: [...{
		secretKey: string
		remoteRef: { key: string, property?: string }
	}]
}

// ── Enriched registries (name, namespace, host derived from key + domain) ──

_services: [string]: #Service
_apps:     [string]: #App
