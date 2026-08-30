#!/usr/bin/env bash
#
# check_selectors.sh — org-wide guard for the
# "Deployment selector doesn't match pod template labels" class of bug
# that ArgoCD can't fix (selector is immutable; only delete+recreate works).
#
# Usage: check_selectors.sh <rendered-manifest.yaml>
#
# For every Deployment, StatefulSet, DaemonSet, ReplicaSet, Job, CronJob in
# the file, verify that EVERY key in spec.selector.matchLabels is present in
# spec.template.metadata.labels. Mismatches cause ArgoCD sync to fail with
# "spec.selector: Invalid value: field is immutable".
#
# Exit code 0 = all selectors valid. Non-zero = at least one mismatch.

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
  case " $workload_kinds " in
    *" $kind "*) ;;
    *) continue ;;
  esac

  name=$("$yq_bin" eval '.metadata.name // ""' "$tmpdir/doc.yaml")
  namespace=$("$yq_bin" eval '.metadata.namespace // "default"' "$tmpdir/doc.yaml")

  # Check selector exists
  selector=$("$yq_bin" eval '.spec.selector.matchLabels // {}' "$tmpdir/doc.yaml" 2>/dev/null)
  if [ "$selector" = "{}" ] || [ -z "$selector" ] || [ "$selector" = "null" ]; then
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
      echo "::error::$kind $namespace/$name selector.matchLabels key '$k' is NOT in spec.template.metadata.labels — ArgoCD will fail to apply (immutable selector)"
      errors=$((errors+1))
    fi
  done < <("$yq_bin" eval '.spec.selector.matchLabels | keys | .[]' "$tmpdir/doc.yaml" 2>/dev/null)
done

if [ "$errors" -gt 0 ]; then
  echo "::error::$errors selector/label mismatch(es) found"
  exit 1
fi

echo "All selectors match pod template labels."
