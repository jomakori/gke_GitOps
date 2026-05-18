package main

// ── Cluster-level resources ──

cluster: {
	// Istio installation + mesh config
	istio: {
		operator: {
			apiVersion: "install.istio.io/v1alpha1"
			kind:       "IstioOperator"
			metadata: {
				name:      "cluster-mesh"
				namespace: "istio-system"
			}
			spec: {
			let _clusterName = clusterName
			profile: "default"
			components: {
				base:       enabled: true
				pilot:      enabled: true
				ingressGateways: [{
					name:    "istio-ingressgateway"
					enabled: true
				}]
				egressGateways: [{
					name:    "istio-egressgateway"
					enabled: false
				}]
				cni: enabled: false
			}
			meshConfig: {
				enableTracing: true
				defaultConfig: {
					terminationDrainDuration: "30s"
					proxyMetadata: {
						ISTIO_META_DNS_CAPTURE: "true"
					}
				}
				// JSON access logs for security auditability
				accessLogFile:   "/dev/stdout"
				accessLogFormat: "JSON"
			}
			values: {
				global: {
					meshID: _clusterName
					multiCluster: clusterName: _clusterName
					network:      "k8s-network"
				}
			}
		}
		}

		// Mesh-wide STRICT mTLS
		meshPeerAuth: {
			apiVersion: "security.istio.io/v1beta1"
			kind:       "PeerAuthentication"
			metadata: {
				name:      "mesh-wide"
				namespace: "istio-system"
			}
			spec: mtls: mode: "STRICT"
		}

		// Default deny-all authorization for the mesh
		meshDenyAll: {
			apiVersion: "security.istio.io/v1beta1"
			kind:       "AuthorizationPolicy"
			metadata: {
				name:      "mesh-deny-all"
				namespace: "istio-system"
			}
			spec: {
				action: "DENY"
				rules: [{}]  // denies all traffic by default
			}
		}
	}

	// Shared ingress gateway (Gateway API)
	ingressGateway: {
		apiVersion: "gateway.networking.k8s.io/v1"
		kind:       "Gateway"
		metadata: {
			name:      "shared-ingress"
			namespace: "istio-system"
		}
		spec: {
			gatewayClassName: "istio"
			listeners: [{
				name:     "http"
				port:     80
				protocol: "HTTP"
				allowedRoutes: namespaces: from: "All"
			}, {
				name:     "https"
				port:     443
				protocol: "HTTPS"
				tls: {
					mode: "Terminate"
					credentialName: "shared-ingress-cert"
				}
				allowedRoutes: namespaces: from: "All"
			}]
		}
	}

	// Doppler ClusterSecretStore
	clusterSecretStore: {
		apiVersion: "external-secrets.io/v1beta1"
		kind:       "ClusterSecretStore"
		metadata: {
			name: "doppler-auth"
		}
		spec: {
			provider: doppler: {
				auth: secretRef: dopplerToken: {
					name: "doppler-token-auth"
					key:  "dopplerToken"
				}
			}
		}
	}
}
