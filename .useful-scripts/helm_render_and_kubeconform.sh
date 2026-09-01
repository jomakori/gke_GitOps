#!/usr/bin/env bash
# helm_render_and_kubeconform.sh — pre-commit hook entry point.
#
# Renders the touched helm chart with `helm template` and runs the
# rendered manifest through `kubeconform -strict` (ignoring CRDs
# without built-in schemas). The same shape CI runs in
# .github/workflows/helm_lint-test.yaml.
#
# Invoked by pre-commit with the changed template paths as args.
# `pre-commit` runs from the repo root (gke_GitOps).
#
# Why this exists: plain YAML linters (yamllint, the editor's
# "yaml-language-server" mode) choke on Helm Go-template syntax
# (`{{ .Release.Namespace }}`, `{{- if ... -}}`, etc.) and emit
# false-positive "unhashable key" or "missing colon" errors.
# Rendering first means the linter sees the actual YAML the
# cluster will receive, not the Go template that produced it.
set -euo pipefail

# pre-commit runs from the repo root.
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "${REPO_ROOT}"

# Find the changed templates' parent chart(s). A file path from
# pre-commit looks like `services/helm/openagent/templates/foo.yaml`;
# the chart root is the parent of `templates/`.
CHARTS=()
for f in "$@"; do
    case "${f}" in
        services/helm/*/templates/*)
            chart_dir="$(echo "${f}" | sed -E 's#^(services/helm/[^/]+)/templates/.*$#\1#')"
            if [[ ! " ${CHARTS[*]:-} " =~ " ${chart_dir} " ]]; then
                CHARTS+=("${chart_dir}")
            fi
            ;;
    esac
done

if [[ ${#CHARTS[@]} -eq 0 ]]; then
    echo "helm_render_and_kubeconform: no chart changed; skipping."
    exit 0
fi

# kubeconform binary: same as CI (v0.6.7).
KUBECONFORM="${KUBECONFORM:-kubeconform}"
if ! command -v "${KUBECONFORM}" >/dev/null 2>&1; then
    echo "helm_render_and_kubeconform: '${KUBECONFORM}' not on PATH."
    echo "Install with: mise use kubeconform@latest  (or: brew install kubeconform)"
    # Don't block the commit when the linter is missing — CI is the
    # authoritative gate. The hook is opt-in shift-left.
    echo "Skipping (CI will catch this in helm_lint-test.yaml)."
    exit 0
fi

# Default to the cluster version CI validates against (matches
# .github/workflows/helm_lint-test.yaml kube_version input default).
KUBE_VERSION="${KUBE_VERSION:-1.35.1}"

failed=0
for chart_dir in "${CHARTS[@]}"; do
    echo "helm_render_and_kubeconform: rendering ${chart_dir}"
    if ! helm dependency update "${chart_dir}" >/dev/null 2>&1; then
        echo "  warning: helm dependency update failed (chart may have no dependencies)"
    fi
    if ! helm template "${chart_dir}" \
            --skip-schema-validation --validate=false \
            > /tmp/openkite_kubeconform_$$.yaml 2> /tmp/openkite_kubeconform_$$.err; then
        echo "  ERROR: helm template failed for ${chart_dir}:"
        sed 's/^/    /' /tmp/openkite_kubeconform_$$.err
        rm -f /tmp/openkite_kubeconform_$$.yaml /tmp/openkite_kubeconform_$$.err
        failed=1
        continue
    fi
    echo "  rendered: /tmp/openkite_kubeconform_$$.yaml"
    # Same flags as CI: -strict (fail on unknown keys), -ignore-missing-schemas
    # (skip CRDs whose schemas aren't in kubeconform's built-in K8s set).
    if ! "${KUBECONFORM}" -strict -ignore-missing-schemas -summary \
            -kubernetes-version "${KUBE_VERSION}" \
            /tmp/openkite_kubeconform_$$.yaml; then
        echo "  ERROR: kubeconform failed for ${chart_dir}"
        failed=1
    fi
    rm -f /tmp/openkite_kubeconform_$$.yaml /tmp/openkite_kubeconform_$$.err
done

if [[ ${failed} -ne 0 ]]; then
    echo "helm_render_and_kubeconform: one or more charts failed validation."
    exit 1
fi
echo "helm_render_and_kubeconform: all charts OK."
exit 0
