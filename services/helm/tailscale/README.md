# Tailscale Kubernetes Operator

This operator joins this cluster to
the tailnet for private access.

## What it does

- `apiServerProxyConfig.mode: "true"` — in-process API server proxy (auth mode).
  The operator creates a tailnet device + a `kubeconfig` ConfigMap in this
  namespace with a ready-to-use kubeconfig (server `https://<name>.<tailnet>.ts.net`).
- OAuth client auth — the operator mounts Secret `operator-oauth`
  (files `client_id` + `client_secret`), pre-created by the Doppler ExternalSecret.

## Prerequisites (user-side)

1. Create a Tailscale **OAuth client** (login.tailscale.com → Settings → OAuth clients):
   - Scopes: `devices` read+write
   - Make it owner of tag `tag:k8s-operator`
2. Doppler config `svc_tailscale` (project `devops`) with:
   - `TAILSCALE_OAUTH_CLIENT_ID`
   - `TAILSCALE_OAUTH_CLIENT_SECRET`
3. Tailnet ACL: allow CI identities (the auth-key-tagged runner node) to reach the
   apiserver proxy tag. With a default-allow tailnet, nothing to do.

## CI usage

The generated kubeconfig (ConfigMap `kubeconfig`, ns `tailscale`) is stored as
Doppler `ci` → `KUBECONFIG` and written to `~/.kube/config` in the GitHub Actions
workflows after `tailscale/github-action` connects the runner to the tailnet.
