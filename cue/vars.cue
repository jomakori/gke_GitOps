package main

// Cluster-scoped values (single source of truth)
// Terraform can overwrite this file for per-cluster overrides.
repoURL:        "https://github.com/jomakori/gke_GitOps"
targetRevision: "main"
region:         "us-east-2"
clusterName:    "jmak-lab"
clusterEndpoint: "https://kubernetes.default.svc"
domain:         "maklab.net"

argoNamespace: "argocd"
argoProject:   "default"
storageClass:  "csi-hostpath-sc"
