package main

import "encoding/yaml"

// ── Generate ArgoCD Applications for services ──

genServiceApps: {
	for key, s in _services if s.enabled {
		let rurl = repoURL
		let trev = targetRevision
		"\(key)": #ArgoCDApp & {
			name: "\(key)"
			source: {
				repoURL:        rurl
				path:           "services/helm/\(key)"
				targetRevision: trev
				if s.values != _|_ {
					helm: values: yaml.Marshal(s.values)
				}
			}
			destination: namespace: "\(s.namespace)"
			syncWave:    s.syncWave
		}
	}
}

// ── Generate ArgoCD Applications for apps ──

genAppApps: {
	for key, s in _apps if s.enabled {
		let rurl = repoURL
		let trev = targetRevision
		"\(key)": #ArgoCDApp & {
			name: "\(key)"
			source: {
				repoURL:        rurl
				path:           "apps/helm/\(key)"
				targetRevision: trev
			}
			destination: namespace: "\(s.namespace)"
			syncWave:    10
		}
	}
}

// ── Generate ExternalSecrets per app environment ──

genExternalSecrets: {
	for name, s in _apps if s.enabled {
		for e in s.environments {
			"\(e.namespace)-vars": {
				apiVersion: "external-secrets.io/v1beta1"
				kind:       "ExternalSecret"
				metadata: {
					name:      "\(e.namespace)-vars"
					namespace: "\(e.namespace)"
				}
				spec: {
					refreshInterval: "30s"
					secretStoreRef: {
						kind: "ClusterSecretStore"
						name: "doppler-auth"
					}
					target: {
						name:           "\(e.namespace)-vars"
						deletionPolicy: "Merge"
					}
					dataFrom: [{
						find: name: regexp: ".*"
					}]
				}
			}
		}
	}
}

// ── Generate PeerAuthentication per app namespace ──

genPeerAuths: {
	for name, s in _apps if s.enabled {
		"\(s.namespace)-mtls": {
			apiVersion: "security.istio.io/v1beta1"
			kind:       "PeerAuthentication"
			metadata: {
				name:      "default"
				namespace: "\(s.namespace)"
			}
			spec: mtls: mode: "\(s.mesh.mtls)"
		}
	}
}

// ── Generate DestinationRule per app ──

genDestRules: {
	for name, s in _apps if s.enabled {
		"\(s.namespace)-dr": {
			apiVersion: "networking.istio.io/v1beta1"
			kind:       "DestinationRule"
			metadata: {
				name:      "\(s.name)"
				namespace: "\(s.namespace)"
			}
			spec: {
				host: "\(s.name).\(s.namespace).svc.cluster.local"
				trafficPolicy: {
					loadBalancer: simple: "\(s.mesh.loadBalancer)"
					connectionPool: {
						tcp: {
							maxConnections: s.mesh.circuitBreaker.maxConnections
							connectTimeout: "5s"
						}
						http: {
							http1MaxPendingRequests: s.mesh.circuitBreaker.maxPendingRequests
							http2MaxRequests:        s.mesh.circuitBreaker.maxRequests
							maxRetries:              s.mesh.circuitBreaker.maxRetries
						}
					}
				}
			}
		}
	}
}

// ── Generate AuthorizationPolicy per app ──

genAuthPolicies: {
	for name, s in _apps if s.enabled {
		"\(s.namespace)-allow-mesh": {
			apiVersion: "security.istio.io/v1beta1"
			kind:       "AuthorizationPolicy"
			metadata: {
				name:      "allow-mesh"
				namespace: "\(s.namespace)"
			}
			spec: {
				action: "ALLOW"
				rules: [{
					from: [{
						source: namespaces: ["istio-system"]
					}]
				}]
			}
		}
	}
}

// ── Generate HTTPRoute per app with gateway ──

genHTTPRoutes: {
	for name, s in _apps if s.enabled && s.gateway != _|_ {
		"\(s.namespace)-ingress": {
			apiVersion: "gateway.networking.k8s.io/v1"
			kind:       "HTTPRoute"
			metadata: {
				name:      "\(s.name)"
				namespace: "\(s.namespace)"
			}
			spec: {
				parentRefs: [{
					name:      "shared-ingress"
					namespace: "istio-system"
				}]
				hostnames: ["\(s.gateway.host)"]
				rules: [{
					matches: [{
						path: {
							type:  "\(s.gateway.paths[0].type)"
							value: "\(s.gateway.paths[0].path)"
						}
					}]
					backendRefs: [{
						name: "\(s.name)"
						port: s.gateway.paths[0].backend
					}]
				}]
			}
		}
	}
}

// ── Output file map consumed by gen-split.py ──

files: {
	// Cluster
	"istio/operator.yaml":                    yaml.Marshal(cluster.istio.operator)
	"istio/mesh-peer-authentication.yaml":    yaml.Marshal(cluster.istio.meshPeerAuth)
	"istio/mesh-deny-all.yaml":               yaml.Marshal(cluster.istio.meshDenyAll)
	"istio/ingress-gateway.yaml":             yaml.Marshal(cluster.ingressGateway)
	"eso/cluster-secret-store.yaml":          yaml.Marshal(cluster.clusterSecretStore)

	// Service Applications
	for k, v in genServiceApps {
		"services/argocd-appset/\(k).yaml":   yaml.Marshal(v)
	}

	// App Applications
	for k, v in genAppApps {
		"apps/argocd-appset/\(k).yaml":       yaml.Marshal(v)
	}

	// ExternalSecrets
	for k, v in genExternalSecrets {
		"eso/external-secrets/\(k).yaml":     yaml.Marshal(v)
	}

	// PeerAuthentications
	for k, v in genPeerAuths {
		"istio/peer-authentications/\(k).yaml": yaml.Marshal(v)
	}

	// DestinationRules
	for k, v in genDestRules {
		"istio/destination-rules/\(k).yaml":  yaml.Marshal(v)
	}

	// AuthorizationPolicies
	for k, v in genAuthPolicies {
		"istio/authorization-policies/\(k).yaml": yaml.Marshal(v)
	}

	// HTTPRoutes
	for k, v in genHTTPRoutes {
		"istio/http-routes/\(k).yaml":        yaml.Marshal(v)
	}
}
