# ExcaliDash

Values override layer for the upstream [alekc/excalidash](https://artifacthub.io/packages/helm/alekc/excalidash)
chart (v1.0.0, repo `https://charts.alekc.dev`).

- Domain: `draw.maklab.net` (private — CF Access Google OAuth at the edge, app-level `AUTH_MODE=local` login)
- Secrets: Doppler config `svc_excalidash` → `doppler-svc-excalidash` ClusterSecretStore → k8s Secret `excalidash-excalidash-secrets` (`JWT_SECRET`, `CSRF_SECRET`; chart prefixes the envFrom secretRef with the release name, so the ExternalSecret target matches the chart-rendered name)
- Storage: 5Gi `local-path` PVC at `/app/prisma` (SQLite, single backend replica)
- Routing: VirtualService `excalidash` in the istio chart (`enablePrivate: true` → `require-cf-access-excalidash` DENY policy)
- Backend env: `FRONTEND_URL`, `TRUST_PROXY=1`, `ENFORCE_HTTPS_REDIRECT=false` (edge TLS), `UPDATE_CHECK_OUTBOUND=false`


## Secrets convention (ES evolution 2026-08-09)

- **Always dynamic**: the ExternalSecret uses `dataFrom` find-all (`regexp: ".*"`) —
  every key in Doppler `svc_excalidash` lands in the k8s Secret automatically.
  No per-key `data:` list to maintain; new Doppler secrets just work.
- **Directly-fetchable name**: the chart secret key is `secrets` (not
  `excalidash-secrets`) so the rendered object is `<fullname>-<key>` =
  `excalidash-secrets` — predictable, no double prefix. The ES target matches it.
- Exception to the dynamic rule: consumers that require fixed key/file names
  (e.g. Tailscale operator-oauth `client_id`/`client_secret`) keep explicit
  `data:` mapping.
