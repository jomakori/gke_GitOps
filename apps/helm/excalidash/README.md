# ExcaliDash

Values override layer for the upstream [alekc/excalidash](https://artifacthub.io/packages/helm/alekc/excalidash)
chart (v1.0.0, repo `https://charts.alekc.dev`).

- Domain: `excalidash.maklab.net` (private — CF Access Google OAuth at the edge, app-level `AUTH_MODE=local` login)
- Secrets: Doppler config `svc_excalidash` → `doppler-svc-excalidash` ClusterSecretStore → k8s Secret `excalidash-secrets` (`JWT_SECRET`, `CSRF_SECRET`)
- Storage: 5Gi `local-path` PVC at `/app/prisma` (SQLite, single backend replica)
- Routing: VirtualService `excalidash` in the istio chart (`enablePrivate: true` → `require-cf-access-excalidash` DENY policy)
- Backend env: `FRONTEND_URL`, `TRUST_PROXY=1`, `ENFORCE_HTTPS_REDIRECT=false` (edge TLS), `UPDATE_CHECK_OUTBOUND=false`
