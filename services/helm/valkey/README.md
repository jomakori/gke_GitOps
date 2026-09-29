# valkey

![Version: 0.12.0](https://img.shields.io/badge/Version-0.12.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 9.1.2](https://img.shields.io/badge/AppVersion-9.1.2-informational?style=flat-square)
Standalone Valkey cache (official Valkey Helm chart) — the cluster's Redis-protocol cache service. Apps consume it; the instance lives here, never in an app release.

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://valkey-io.github.io/valkey-helm/ | valkey | 0.12.0 |

## Under the hood

A thin wrapper around the official [Valkey Helm chart](https://github.com/valkey-io/valkey-helm) — one **standalone** Valkey instance, `ClusterIP` only, in the `valkey` namespace. No operator, no CRDs, no per-app instances: this is the cluster's shared Redis-protocol cache, and apps point at it.

### Why Valkey

Valkey is the Redis fork: same RESP protocol, same commands, same persistence semantics. LiteLLM states the compatibility directly — *"Valkey, AWS ElastiCache and GCP Memorystore all speak the Redis protocol, so everything on this page applies to them too"* — so a consumer needs no Valkey-specific code path. The image ships `redis-server`, `redis-cli` and `redis-sentinel` symlinks alongside the `valkey-*` binaries (verified against `valkey/valkey:9.1.2`).

### Why not the Valkey operator

`valkey-io/valkey-operator` ships only `ValkeyCluster` / `ValkeyNode` CRDs. Even a 1-shard, 0-replica cache runs in cluster mode there, which forces cluster-aware client configuration (`REDIS_CLUSTER_NODES`, `cache_params.redis_startup_nodes`) on every consumer. LiteLLM's router state (`router_settings.redis_host`) is host/port only, so a standalone instance keeps one plain endpoint for both the cache and the router.

### Persistence, sizing and durability

- **AOF on** (`appendonly yes`, `appendfsync everysec`) on a 1Gi `local-path` PVC. Not just cached replies: LiteLLM keeps rate-limit counters and budget reservations here, so a dataset lost on restart under-enforces limits until the database reconciles.
- **`maxmemory 400mb` with `maxmemory-policy noeviction`**, tracking the 512Mi container limit at ~80%. Eviction would silently drop accounting keys in favour of cached responses; a full Redis that starts refusing writes is the honest failure.
- **CPU limit deliberately omitted** (CFS throttling); the memory limit is the real ceiling.
- **Single node, no replicas.** This is a single-node cluster — a replica would share the node and buy no failover, only a pod.

### Consumers

| Consumer | Wiring |
|----------|--------|
| `openagent` (LiteLLM gateway) | `litellm.envVars.REDIS_HOST=valkey-cache.valkey.svc.cluster.local:6379`, plus `litellm_settings.cache_params` and `router_settings.redis_*` pointing at it |

The Service FQDN comes from `fullnameOverride` in this chart's `values.yaml` — change it and the consumer's `REDIS_HOST` must move with it.

### Authentication

Disabled: the Service is `ClusterIP`-only and every consumer is in-cluster (the same posture `plane-redis` runs with). Enabling it means an ACL user, a Doppler key, an ExternalSecret and a matching `REDIS_PASSWORD` on every consumer.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| valkey.auth.enabled | bool | `false` |  |
| valkey.dataStorage.className | string | `"local-path"` |  |
| valkey.dataStorage.enabled | bool | `true` |  |
| valkey.dataStorage.requestedSize | string | `"1Gi"` |  |
| valkey.enabled | bool | `true` |  |
| valkey.fullnameOverride | string | `"valkey-cache"` |  |
| valkey.image.pullPolicy | string | `"IfNotPresent"` |  |
| valkey.image.registry | string | `"docker.io"` |  |
| valkey.image.repository | string | `"valkey/valkey"` |  |
| valkey.image.tag | string | `"9.1.2"` |  |
| valkey.nodeSelector.intent | string | `"apps"` |  |
| valkey.replica.enabled | bool | `false` |  |
| valkey.resources.limits.memory | string | `"512Mi"` |  |
| valkey.resources.requests.cpu | string | `"50m"` |  |
| valkey.resources.requests.memory | string | `"128Mi"` |  |
| valkey.service.port | int | `6379` |  |
| valkey.service.type | string | `"ClusterIP"` |  |
| valkey.valkeyConfig | string | `"appendonly yes\nappendfsync everysec\nmaxmemory 400mb\nmaxmemory-policy noeviction\n"` |  |
