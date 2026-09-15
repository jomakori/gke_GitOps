#!/usr/bin/env bash
#
# check_selectors.sh — org-wide guard for two selector classes that ArgoCD
# cannot repair for you:
#
#   1. "selector doesn't match pod template labels" — the Deployment is invalid
#      and every sync fails until the selector is corrected.
#   2. "selector carries a version-bearing key" (helm.sh/chart,
#      app.kubernetes.io/managed-by, app.kubernetes.io/version) — the chart
#      renders clean and the keys still match, so NO render-time gate sees it,
#      but a Deployment's spec.selector is IMMUTABLE: the next chart version
#      bump makes ArgoCD fail with
#        spec.selector: Invalid value: {...}: field is immutable
#      and the only repair is deleting and recreating the workload. A Service
#      selector with the same key is not immutable, but it churns endpoints on
#      every version bump for no benefit.
#
# Usage: check_selectors.sh <rendered-manifest.yaml>
#
# For every Deployment, StatefulSet, DaemonSet, ReplicaSet, Job, CronJob in the
# file, verify that EVERY key in the selector is present in
# spec.template.metadata.labels. For those kinds AND every Service, verify the
# selector carries no version-bearing key.
#
# Exit code 0 = all selectors valid. Non-zero = at least one problem.

set -euo pipefail

if [ $# -ne 1 ] || [ ! -f "$1" ]; then
  echo "Usage: $0 <rendered-manifest.yaml>" >&2
  exit 2
fi

file="$1"

# yq is required. Try common install locations.
yq_bin=""
for candidate in /opt/data/bin/yq /usr/local/bin/yq /usr/bin/yq; do
  if [ -x "$candidate" ]; then yq_bin="$candidate"; break; fi
done
if [ -z "$yq_bin" ]; then
  echo "::error::yq not found in PATH or /opt/data/bin — install mikefarah/yq" >&2
  exit 3
fi

workload_kinds="Deployment StatefulSet DaemonSet ReplicaSet Job CronJob"
selector_kinds="$workload_kinds Service"
errors=0

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

# Split multi-doc YAML into single docs (yq v4: split-docs reads stdin,
# writes numbered doc files to current dir or to -o target).
# Use a workaround: yq eval-all with `document_index` to count, then
# extract each doc individually.
total_docs=$("$yq_bin" eval-all 'document_index' "$file" | wc -l | tr -d ' ')

for ((idx=0; idx<total_docs; idx++)); do
  # Extract single doc
  "$yq_bin" eval-all "select(document_index == $idx)" "$file" > "$tmpdir/doc.yaml"
  [ -s "$tmpdir/doc.yaml" ] || continue

  kind=$("$yq_bin" eval '.kind // ""' "$tmpdir/doc.yaml")
  case " $selector_kinds " in
    *" $kind "*) ;;
    *) continue ;;
  esac

  name=$("$yq_bin" eval '.metadata.name // ""' "$tmpdir/doc.yaml")
  namespace=$("$yq_bin" eval '.metadata.namespace // "default"' "$tmpdir/doc.yaml")

  # A Service carries its selector at spec.selector; a workload at
  # spec.selector.matchLabels.
  if [ "$kind" = "Service" ]; then
    selector_path=".spec.selector"
    selector_label="spec.selector"
  else
    selector_path=".spec.selector.matchLabels"
    selector_label="spec.selector.matchLabels"
  fi

  selector=$("$yq_bin" eval "$selector_path // {}" "$tmpdir/doc.yaml" 2>/dev/null)
  if [ "$selector" = "{}" ] || [ -z "$selector" ] || [ "$selector" = "null" ]; then
    continue
  fi

  # Version-bearing labels must never be in a selector. This is an ERROR, not a
  # warning: every chart in this repo is now clean (claude-proxy #202,
  # hermes-webui #205), so a regression has to fail the build rather than pass
  # with a note — the whole defect is that nothing else sees it. Fixing a chart
  # that regresses here is itself a delete-and-recreate, so the time to catch it
  # is before it merges.
  while IFS= read -r k; do
    [ -z "$k" ] && continue
    case "$k" in
      helm.sh/chart|app.kubernetes.io/managed-by|app.kubernetes.io/version)
        echo "::error::$kind $namespace/$name $selector_label carries version-bearing key '$k' — a chart version bump makes this an immutable-selector failure (workloads) or needless endpoint churn (Services); keep it on metadata/pod labels only"
        errors=$((errors+1))
        ;;
    esac
  done < <("$yq_bin" eval "$selector_path | keys | .[]" "$tmpdir/doc.yaml" 2>/dev/null)

  # The subset check applies only to workloads: a Service has no pod template.
  if [ "$kind" = "Service" ]; then
    continue
  fi

  # Pod template labels (ReplicaSet has no .spec.template — its selector IS the pod label set)
  if [ "$kind" = "ReplicaSet" ]; then
    pod_labels=$selector
  else
    pod_labels=$("$yq_bin" eval '.spec.template.metadata.labels // {}' "$tmpdir/doc.yaml" 2>/dev/null)
  fi

  if [ -z "$pod_labels" ] || [ "$pod_labels" = "null" ]; then
    echo "::error::$kind $namespace/$name has spec.selector.matchLabels but no spec.template.metadata.labels"
    errors=$((errors+1))
    continue
  fi

  # Iterate selector keys
  while IFS= read -r k; do
    [ -z "$k" ] && continue
    has=$("$yq_bin" eval ".spec.template.metadata.labels | has(\"$k\")" "$tmpdir/doc.yaml" 2>/dev/null)
    if [ "$has" != "true" ]; then
      echo "::error::$kind $namespace/$name spec.selector.matchLabels key '$k' is NOT in spec.template.metadata.labels — ArgoCD will fail to apply (immutable selector)"
      errors=$((errors+1))
    fi
  done < <("$yq_bin" eval '.spec.selector.matchLabels | keys | .[]' "$tmpdir/doc.yaml" 2>/dev/null)
done

if [ "$errors" -gt 0 ]; then
  echo "::error::$errors selector problem(s) found"
  exit 1
fi

echo "All selectors match pod template labels and carry no version-bearing keys."
