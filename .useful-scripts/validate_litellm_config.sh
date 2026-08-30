#!/bin/bash
#
# validate_litellm_config.sh — Renders the openagent Helm chart and validates
# the LiteLLM ConfigMap structure for common failure patterns.
#
# Usage:
#   .useful-scripts/validate_litellm_config.sh
#   .useful-scripts/validate_litellm_config.sh --values custom-values.yaml
#
# Validates:
#   1. No duplicate model_name entries in model_list
#   2. All litellm_params are dicts (not lists — LiteLLM crash)
#   3. No api_base pointing to port 4524 (dead port)
#   4. No api_key: "dummy" (auth mismatch)
#   5. All fallback model names exist in model_list
#   6. router_settings block present
#   7. litellm.migrationJob.enabled must be false (upstream prisma leak)
#
# Exit: 0 = pass, 1 = failures found

set -euo pipefail

RED="\033[31m"
GREEN="\033[32m"
YELLOW="\033[33m"
RESET="\033[0m"

CHART_DIR="${CHART_DIR:-services/helm/openagent}"
EXTRA_VALUES="${1:-}"
FAILURES=0
WARNINGS=0

cleanup() {
  rm -f /tmp/litellm-config-validate-*.yaml 2>/dev/null || true
  rm -f /tmp/litellm-config-validate.py 2>/dev/null || true
}
trap cleanup EXIT

# ── Validation 7: litellm.migrationJob.enabled must be false ─────────
# The upstream litellm/proxy/prisma_migration.py has a memory leak that
# OOMKills the pod after migrations complete (burndown: 256Mi/1Gi/2Gi/4Gi
# all OOM, exit 137). Disabling the Job is the only path to keep sync
# green. Re-enabling requires fixing the upstream leak first.
# This is a values-level check — no chart render needed.
echo ""
echo -e "${GREEN}── Validation 7: prisma-migration Job disabled (upstream leak)${RESET}"
MIGRATION_ENABLED=$(/opt/data/bin/yq eval '.litellm.migrationJob.enabled' "${CHART_DIR}/values.yaml" 2>/dev/null || echo "null")
if [ "$MIGRATION_ENABLED" = "false" ]; then
  echo -e "  ${GREEN}✓${RESET} litellm.migrationJob.enabled = false (correct)"
else
  echo -e "  ${RED}✗ FAIL${RESET}: litellm.migrationJob.enabled = '$MIGRATION_ENABLED' (must be false — see #148)"
  FAILURES=$((FAILURES + 1))
fi

# ── Ensure chart deps are extracted ──────────────────────────────────
# The umbrella chart depends on remote OCI charts (litellm-helm, hermes-agent)
# that are NOT vendored. `helm template` fails without them, so fetch them
# like ct_check.sh does. charts/ is gitignored.
if [ ! -d "${CHART_DIR}/charts/litellm-helm" ] || [ ! -d "${CHART_DIR}/charts/hermes-agent" ]; then
    echo "  → helm dependency update (fetch remote chart deps)"
    ( cd "${CHART_DIR}" && helm dependency update ) > /tmp/litellm-config-validate-deps.log 2>&1 \
        || { echo -e "${RED}✗ helm dependency update failed${RESET}"; cat /tmp/litellm-config-validate-deps.log | tail -15; exit 1; }
fi

# ── Render chart ─────────────────────────────────────────────────────
echo -e "${GREEN}── Rendering chart: ${CHART_DIR}${RESET}"

HELM_ARGS=("$CHART_DIR" --skip-schema-validation)
if [[ -n "$EXTRA_VALUES" ]]; then
  HELM_ARGS+=(-f "$EXTRA_VALUES")
fi

helm template test-litellm "${HELM_ARGS[@]}" > /tmp/litellm-config-validate-full.yaml 2>&1 || {
  echo -e "${RED}✗ helm template failed${RESET}"
  cat /tmp/litellm-config-validate-full.yaml | tail -20
  exit 1
}

# ── Extract ConfigMap ─────────────────────────────────────────────────
echo -e "${GREEN}── Extracting LiteLLM ConfigMap${RESET}"

python3 -c "
import yaml, sys

with open('/tmp/litellm-config-validate-full.yaml') as f:
    docs = list(yaml.safe_load_all(f))

configmap = None
for doc in docs:
    if doc and doc.get('kind') == 'ConfigMap' and '-litellm-' in doc.get('metadata', {}).get('name', '') and 'config.yaml' in doc.get('data', {}) and 'model_list' in doc.get('data', {}).get('config.yaml', ''):
        configmap = doc
        break

if not configmap:
    print('ERROR: No LiteLLM ConfigMap found in rendered output')
    sys.exit(1)

config_yaml = configmap.get('data', {}).get('config.yaml', '')
if not config_yaml:
    print('ERROR: ConfigMap has no config.yaml key')
    sys.exit(1)

with open('/tmp/litellm-config-validate-config.yaml', 'w') as f:
    f.write(config_yaml)

print(f'ConfigMap: {configmap[\"metadata\"][\"name\"]}')
print(f'Config size: {len(config_yaml)} bytes')
" 2>&1 || {
  echo -e "${RED}✗ Failed to extract ConfigMap${RESET}"
  exit 1
}

# ── Run validations ───────────────────────────────────────────────────
echo ""
echo -e "${GREEN}── Running validations${RESET}"

python3 << 'PYEOF'
import yaml, sys, os
from collections import Counter

with open('/tmp/litellm-config-validate-config.yaml') as f:
    config = yaml.safe_load(f)

failures = 0
warnings = 0

def fail(msg):
    global failures
    print(f"  \033[31m✗ FAIL\033[0m: {msg}")
    failures += 1

def warn(msg):
    global warnings
    print(f"  \033[33m⚠ WARN\033[0m: {msg}")
    warnings += 1

def ok(msg):
    print(f"  \033[32m✓\033[0m {msg}")

# ── Validation 1: No duplicate model_names ────────────────────────────
model_list = config.get('model_list', [])
model_names = [m.get('model_name') for m in model_list]
name_counts = Counter(model_names)
dupes = {n: c for n, c in name_counts.items() if c > 1}

if dupes:
    for name, count in dupes.items():
        fail(f"Duplicate model_name '{name}' appears {count} times — causes LiteLLM list/dict crash")
else:
    ok(f"No duplicate model_names in {len(model_list)} entries")

# ── Validation 2: litellm_params must be dict ─────────────────────────
for m in model_list:
    params = m.get('litellm_params')
    if isinstance(params, list):
        fail(f"model '{m.get('model_name')}': litellm_params is a LIST (should be dict) — causes 'list has no items()' crash")
    elif not isinstance(params, dict):
        fail(f"model '{m.get('model_name')}': litellm_params is {type(params).__name__} (should be dict)")

if not any(isinstance(m.get('litellm_params'), list) for m in model_list):
    ok(f"All litellm_params are dicts")

# ── Validation 3: No port 4524 in api_base ────────────────────────────
for m in model_list:
    api_base = m.get('litellm_params', {}).get('api_base', '')
    if '4524' in str(api_base):
        fail(f"model '{m.get('model_name')}': api_base references port 4524 (dead port, nothing listens)")

# ── Validation 4: No dummy api_key ────────────────────────────────────
for m in model_list:
    api_key = m.get('litellm_params', {}).get('api_key', '')
    if api_key == 'dummy':
        fail(f"model '{m.get('model_name')}': api_key is 'dummy' (auth mismatch with claude-proxy)")

# ── Validation 5: Fallback models exist in model_list ─────────────────
fallbacks = config.get('fallbacks', [])
all_model_names_set = set(model_names)

for fb in fallbacks:
    for src, targets in fb.items():
        if src not in all_model_names_set:
            fail(f"Fallback source '{src}' not found in model_list")
        for tgt in targets:
            if tgt not in all_model_names_set:
                fail(f"Fallback target '{tgt}' (from '{src}') not found in model_list")

if fallbacks:
    ok(f"All fallback references valid ({len(fallbacks)} chains)")

# ── Validation 6: router_settings present ─────────────────────────────
router = config.get('router_settings')
if router:
    cooldown = router.get('cooldown_time', 'NOT SET')
    retries = router.get('num_retries', 'NOT SET')
    fails = router.get('allowed_fails', 'NOT SET')
    timeout = router.get('request_timeout', 'NOT SET')
    ok(f"router_settings: cooldown={cooldown}s, retries={retries}, allowed_fails={fails}, timeout={timeout}s")
else:
    warn("No router_settings — no cooldown/retry config (rate-limit thrashing risk)")

# ── Bonus: Check for rpm on claude-proxy models ───────────────────────
claude_models = [m for m in model_list if 'claude-proxy' in str(m.get('litellm_params', {}).get('api_base', ''))]
for m in claude_models:
    rpm = m.get('litellm_params', {}).get('rpm')
    if rpm:
        ok(f"model '{m.get('model_name')}': rpm={rpm} (rate-limited)")
    else:
        warn(f"model '{m.get('model_name')}': no rpm set (no rate limiting on Claude Pro)")

# ── Summary ───────────────────────────────────────────────────────────
print()
if failures:
    print(f"\033[31m{failures} FAILURE(S)\033[0m")
else:
    print(f"\033[32mAll validations passed\033[0m")
if warnings:
    print(f"\033[33m{warnings} WARNING(S)\033[0m")

sys.exit(1 if failures else 0)
PYEOF

EXIT_CODE=$?

echo ""
if [[ $EXIT_CODE -eq 0 ]]; then
  echo -e "${GREEN}✓ LiteLLM ConfigMap validation passed${RESET}"
else
  echo -e "${RED}✗ LiteLLM ConfigMap validation failed — fix before deploying${RESET}"
fi

exit $EXIT_CODE
