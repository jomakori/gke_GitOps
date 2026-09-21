# gitopsctl

Stdlib-only Go CLI for the openagent umbrella — the gateway boot sequence + MCP manifest pre-warm/verification. Moved from `services/helm/openagent/extras/` with the binary renamed `hermes-tools` → `gitopsctl`; that dir is now removed (the prebuilt tools image ships the binary — nothing is compiled in-cluster).

## What it is

One binary, one command family, plus the sibling repo-side scripts staged to fold in later:

| Command | Stage | Status |
|---------|-------|--------|
| `gitopsctl boot` | in-cluster runtime (gateway entrypoint) | implemented — shipped prebuilt via the tools image (no in-cluster Go build) |
| `gitopsctl install DEST` | in-cluster runtime (initContainer tool seeding) | implemented — copies the scratch-image binary into a shared `emptyDir` |
| `gitopsctl mcp prewarm` | in-cluster runtime (boot cache warm) | implemented |
| `gitopsctl mcp verify --mode preflight\|drift\|validate` | in-cluster runtime (PostSync preflight + drift CronJob) | implemented |
| `chart lint/render/unittest/schema-patch` | repo-side | staged — shell (`.useful-scripts/ct_check.sh`, `helm_render_and_kubeconform.sh`, `helm_unittest_openagent.sh`, `patch_hermes_schema.sh`) |
| `validate litellm-config\|mcp-manifest\|selectors` | repo-side | staged — shell (`.useful-scripts/validate_litellm_config.sh`, `validate_mcp_manifest.sh`, `check_selectors.sh`) |
| `skill render-local` | repo-side | staged — shell (`.useful-scripts/render_local_skill.sh`) |

`boot` reconstructs the old `boot.sh` python flow 1:1 (mise install, system deps, MCP pre-warm, chown) and hands over to `/init hermes gateway run`. `mcp verify` mirrors the MCP client's stdio + Streamable HTTP handshake so a preflight PASS means a runtime PASS; `--mode validate` is the static policy lint (pins, timeouts, lifecycle, parked duplicates, tool surface, each npx server's own package cache, a uvx server's toolchain env, auth-probe declarations) with no network; `--mode drift` additionally calls each server's declared `auth_probe`, because a handshake certifies the protocol and never the credential.

## Setup

Go 1.24, stdlib-only — no external modules.

```bash
# devbox (go@latest is in devbox.json) or mise:
devbox run go build ./cmd/gitopsctl

# optional: drop the binary on PATH as .bin/gitopsctl
mkdir -p .bin && go build -o .bin/gitopsctl ./cmd/gitopsctl
```

The repo's pre-commit hooks already exercise the binary indirectly — `validate_mcp_manifest.sh` (wired to the `mcp-manifest-validate` hook) renders the manifest and runs `mcp verify --mode validate` on every `values.yaml`/`hooks.yaml` change.

## Usage

Local repo commands:

```bash
# static policy lint — no network
go run ./cmd/gitopsctl mcp verify \
  --manifest <(helm template openagent services/helm/openagent --skip-schema-validation \
    | yq 'select(.kind=="ConfigMap" and .metadata.name=="openagent-mcp-manifest").data."mcp-manifest"') \
  --mode validate

# real handshake of every enabled server (stdio + Streamable HTTP)
go run ./cmd/gitopsctl mcp verify --manifest /tmp/mcp-manifest.json --mode preflight

# drift check (quiet on match, non-zero on drift)
go run ./cmd/gitopsctl mcp verify --manifest /tmp/mcp-manifest.json --mode drift
```

Cluster side, the preflight Job and drift CronJob run the verifier from the prebuilt tools image (`tools.image` in `services/helm/openagent/values.yaml`):

| How the binary reaches the workloads |
|--------------------------------------|
| `install-gitopsctl` initContainer copies it out of the prebuilt tools image `ghcr.io/jomakori/gitopsctl:<sha>` (`tools.image.repository` + pinned `tools.image.tag`) into an emptyDir at `/opt/tools/gitopsctl`; the hermes image + PVC stay for the MCP toolchain under `/opt/data` |

## Development

```bash
go test ./...   # manifest model, static policy, stdio handshake (testdata/fake-mcp.sh)
go vet ./...
```

The shell scripts listed as "staged" above are the future absorption targets. When one moves into Go, add a golden-file parity test — capture the script's current stdout/stderr for a representative fixture, assert the Go command reproduces it byte-for-byte, then delete the script. No new dependencies: the module stays stdlib-only.
