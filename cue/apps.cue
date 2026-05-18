package main

// ── Raw app data (name, namespace, gateway.host, env.host derived from key + domain) ──

_appData: [string]: {
	description?: string
	enabled?:     bool
	ports:        [...#Port]
	environments: [...{
		name:      string
		namespace: string
		host?:     string
		image?: { repository?: string, tag?: string }
		dopplerProject?: string
		resources?:      #Resources
		replicas?:       int
		storage?:        #Storage
	}]
	mesh: {
		mtls?:            string
		retries?:         int
		timeout?:         string
		circuitBreaker?:  #CircuitBreaker
		loadBalancer?:    string
		trafficRouting?:  #TrafficRouting
		authorization?:   [...#AuthorizationPolicy]
	}
	gateway?: {
		host?: string
		paths: [...{
			path:    string
			type?:   "PathPrefix" | "Exact" | "Regex" | *"PathPrefix"
			backend: int
		}]
		tls?: { secretName: string }
	}
	resources: #Resources
}

_appData: "demo-app": {
	description: "Demo Go application with health endpoint"
	ports: [{port: 3000, protocol: "HTTP", name: "http"}]
	environments: [{
		name:      "active"
		namespace: "demo-app"
		image: repository: "123456.dkr.ecr.us-east-2.amazonaws.com/demoapp"
		image: tag:        "latest"
	}]
	mesh: {
		timeout:      "15s"
		loadBalancer: "LEAST_CONN"
		circuitBreaker: {
			maxConnections: 100
			maxRequests:    500
		}
	}
	gateway: {
		paths: [{path: "/", type: "PathPrefix", backend: 3000}]
	}
	resources: requests: {cpu: "256m", memory: "256Mi"}
}

_appData: "notes-app": {
	description: "Note-taking app with backend API + frontend UI"
	ports: [
		{port: 8080, protocol: "HTTP", name: "backend"},
		{port: 8181, protocol: "HTTP", name: "frontend"},
	]
	environments: [
		{
			name:      "staging"
			namespace: "staging-notes-app"
			image: repository: "ghcr.io/jomakori/note-app-backend"
			image: tag:        "pr-7"
			resources: requests: {cpu: "100m", memory: "256Mi"}
			replicas: 1
		},
		{
			name:      "production"
			namespace: "prod-notes-app"
			image: repository: "ghcr.io/jomakori/note-app-backend"
			image: tag:        "1.0.1"
			resources: requests: {cpu: "150m", memory: "750Mi"}
			replicas: 2
		},
	]
	mesh: {
		retries:      5
		loadBalancer: "ROUND_ROBIN"
	}
	gateway: {
		paths: [{path: "/", type: "PathPrefix", backend: 8080}]
	}
	resources: requests: {cpu: "256m", memory: "512Mi"}
}

// ── Enriched: name/namespace from key, gateway.host + env.host from domain ──

_apps: {
	for key, s in _appData {
		(key): #App & {
			name:      key
			namespace: key
			mesh:      s.mesh | *{}
			resources: s.resources
			environments: [
				for e in s.environments {
					e & {
						host: e.host | *"\(e.name).\(key).\(domain)"
					}
				}
			]
			if s.gateway != _|_ {
				gateway: {
					host: s.gateway.host | *"\(key).\(domain)"
				}
			}
		} & s
	}
}
