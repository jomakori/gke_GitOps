# Kagent Intent Classification Tests

Go test suite for Sisyphus inline intent classification. Two modes: **unit tests** (fast, no LLM) and **integration tests** (live agent endpoint via private domain).

## Access & Auth

Three clients talk to kagent:

| Client | Route | Auth | Why |
|---|---|---|---|
| **Discord bot** | `http://kagent-controller.kagent.svc.cluster.local:8083/a2a` | Istio mTLS (in-cluster) | Same namespace, mesh-encrypted |
| **Local dev** | `https://kagent.maklab.net/a2a` | CF Access JWT (Google OAuth + OTP) | Human access via browser/CLI |
| **CI (GitHub Actions)** | `https://kagent.maklab.net/a2a` | CF Access service token | Machine-to-machine, no browser |

### Discord Bot (In-Cluster)

Bot runs in `kagent` namespace, connects directly to controller service via internal DNS. Istio ambient mesh handles auth transparently via **STRICT mTLS** — no CF Access tokens or custom auth logic in bot:

```yaml
# services/helm/kagent-discord/values.yaml
config:
  kagentA2aUrl: "http://kagent-controller.kagent.svc.cluster.local:8083/a2a"
```

All pod-to-pod traffic in mesh encrypted + authenticated by Istio. Bot trusts kagent because both in same SPIFFE identity domain.

### Local Development (Private Domain)

Kagent private domain configured via Istio umbrella chart:

- **Domain**: `https://kagent.<clusterDomain>` (e.g., `https://kagent.maklab.net`)
- **Access**: Requires valid Cloudflare Access JWT (Google OAuth + OTP)
- **Service**: `kagent-ui.kagent.svc.cluster.local:8080` (provided by upstream kagent chart)

Registered in `services/argocd-appset/values.yaml`:

```yaml
kagent:
  gateways:
    enable_private: true
    subdomain: kagent
    destination:
      serviceName: kagent-ui
      servicePort: 8080
```

**To use locally** (with Doppler CLI):
```bash
doppler login
doppler configure set project devops
doppler configure set config ci
export KAGENT_PRIVATE_URL=$(doppler secrets get KAGENT_PRIVATE_URL --plain)
export CF_ACCESS_CLIENT_ID=$(doppler secrets get CF_ACCESS_CLIENT_ID --plain)
export CF_ACCESS_CLIENT_SECRET=$(doppler secrets get CF_ACCESS_CLIENT_SECRET --plain)
make full_coverage
```

### CI / GitHub Actions (CF Access Service Token)

CI uses **Cloudflare Access service token** for machine-to-machine auth. No browser, no port-forward, no cluster access:

1. Doppler `devops/ci` config holds service token
2. CI fetches via `dopplerhq/secrets-fetch-action`
3. Go test client sends `CF-Access-Client-Id` + `CF-Access-Client-Secret` headers with every request

See `.github/workflows/kagent_tests.yaml` for full workflow.

#### Doppler Secrets for CI

| Doppler Config | Secret Key | Value | How to get |
|---|---|---|---|
| `devops/ci` | `KAGENT_PRIVATE_URL` | `https://kagent.maklab.net` | Your private domain |
| `devops/ci` | `CF_ACCESS_CLIENT_ID` | `xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx.access` | Cloudflare Zero Trust → Access → Service Tokens |
| `devops/ci` | `CF_ACCESS_CLIENT_SECRET` | `<uuid>` | Generated alongside Client ID |

**Create service token:**
1. Cloudflare dashboard → Zero Trust → Access → Service Tokens
2. Create token → name it `kagent-ci`
3. Copy Client ID + Client Secret to Doppler `devops/ci`
4. Add token to Access policy for `kagent.maklab.net`

## Running Tests

### Makefile Targets

```bash
cd services/helm/kagent/tests

# Unit tests only (fast, no LLM dependency)
make tests

# Unit + integration tests against live agent endpoint
# Requires KAGENT_PRIVATE_URL env var (from Doppler or manual export)
make full_coverage

# Ensure go.mod/go.sum are clean
make tidy

# Purge test cache
make clean
```

### Unit Tests (fast, no dependencies)

```bash
make tests
```

Runs in ~1 second. Validates:
- YAML test file parses correctly
- All 6 tiers have coverage
- Test case structure is valid
- Classification validation logic works

### Integration Tests (requires private domain access)

```bash
export KAGENT_PRIVATE_URL="https://kagent.maklab.net"
make full_coverage
```

Sets `KAGENT_A2A_URL=$(KAGENT_PRIVATE_URL)/a2a` and runs integration tests:
- Sends each test input to Sisyphus via A2A
- Parses classification JSON from response
- Compares against expected tier/domain/agent/gates
- Requires 80% pass rate (some ambiguous tests may fail by design)

## CI/CD

### GitHub Actions (Pull Request)

Workflow `.github/workflows/kagent_tests.yaml` runs kagent tests **only when kagent files change**:

**Trigger paths**:
- `services/helm/kagent/**`
- `services/helm/kagent-discord/**`
- `services/helm/kagent-headroom/**`
- `services/helm/kagent-substrate/**`

**What runs**:
1. Sets up Go 1.22
2. Runs `make tidy && make tests` (unit tests only for PRs)

**Integration tests** run on manual trigger (`workflow_dispatch`) or when PR has `integration-test` label.

### Nightly Integration Tests

```yaml
name: Kagent Integration Tests
on:
  schedule:
    - cron: '0 6 * * *'  # 6 AM daily
jobs:
  integration:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - uses: dopplerhq/secrets-fetch-action@main
        with:
          doppler-token: ${{ secrets.DOPPLER_TOKEN }}
          doppler-project: ${{ secrets.DOPPLER_CI_PROJECT }}
          doppler-config: ci
          inject-env-vars: true
      - run: cd services/helm/kagent/tests && make full_coverage
```

## Test File Format

Tests live in `intent-classification.yaml` as markdown-embedded YAML blocks:

```yaml
test_id: trivial-001
description: Typo fix in single file
input: "Fix the typo 'recieve' -> 'receive' in README.md line 42"
expected:
  tier: trivial
  domain: general
  primary_agent: sisyphus-junior
  loop_depth: direct
  verification: V1
  gates:
    ask_user: false
    require_confirmation: false
notes: Single file, single line, obvious fix. No planning needed.
```

## Adding Tests

1. Add new YAML block to `intent-classification.yaml`
2. Ensure `test_id` is unique
3. Pick correct tier based on complexity
4. Run `make tests` to verify parsing

## Why Go?

- Same language as Discord bot and kagent substrate
- Native YAML parsing with `gopkg.in/yaml.v3`
- Standard `testing` package — no external test runner
- Easy CI integration (`go test` is universal)
- Can be extended to test other agent behaviors (not just classification)
