# Changelog

## [2.0.0] - 2026-07-12

### Added
- Umbrella chart structure with 3 deployment layers: upstream dep (litellm-helm), component subchart (openagent-component), and local CRD templates
- litellm-helm OCI dependency vendored locally at charts/litellm-helm/ (v1.92.0) for offline Helm template validation
- openagent-component subchart for headroom proxy and Discord bot (12 template files with conditional guards)
- 7 CRD templates organized in thematic directories: ensemble/, skillpacks/, db/
- Shared infrastructure templates: ExternalSecret, GHCR pull secret, VPA, Istio VirtualService + AuthorizationPolicy
- 12-model model_list in LiteLLM proxy_config across 5 providers (OpenCode, DeepSeek, Anthropic, MiniMax, z.ai, Moonshot)
- Full values.yaml with complete configuration surface area (177 lines)

### Changed
- Replaced 28 monolithic inlined templates with proper umbrella chart
- Moved headroom proxy templates into component subchart (7 files with headroom.enabled guard)
- Moved Discord bot templates into component subchart (5 files with bot.enabled guard)
- Restructured local templates into 5 thematic directories (ensemble, skillpacks, db, shared, gateway)
- Updated _helpers.tpl with component-based naming helpers
- Updated README with architecture docs, values table, and setup instructions

### Removed
- All inlined templates for LiteLLM, headroom, Discord bot, and PostgreSQL (28 files)
- Legacy kagent stack: 5 chart directories, 2 ArgoCD templates, 5+ value blocks, 2 ClusterSecretStores
- kagent-specific GitHub Actions workflow (agent_lint-test.yaml)
- All kagent references from argocd-appset and external-secrets values files

## [0.1.0] - 2024

### Added
- Initial import from kagent-discord chart source code
- Go Discord bot binary with Discord gateway integration
- Dockerfile and Makefile for containerized deployment
- main.go with Discord polling and Sisyphus web endpoint routing

[2.0.0]: https://github.com/jomakori/gke_GitOps/compare/v0.1.0...v2.0.0
[0.1.0]: https://github.com/jomakori/gke_GitOps/releases/tag/v0.1.0
