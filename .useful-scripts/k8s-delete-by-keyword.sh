#!/bin/bash

# ------------------------------------------------------------------
# Script: k8s-delete-by-keyword.sh
# Description: Prompts for a keyword, lists all Kubernetes resources
#              (all types except events, all namespaces, including cluster-scoped) whose
#              names contain the keyword, and asks for confirmation before deletion.
#              Handles resources with finalizers by removing them first.
# Usage: ./.useful-scripts/k8s-delete-by-keyword.sh
# ------------------------------------------------------------------

set -e

# Prompt for the keyword to search for
echo "Global Kubernetes Resource Deletion Script"
echo "This script will search for all Kubernetes resources (excluding events) containing"
echo "a specified keyword and delete them, handling resources with finalizers."
read -p "Enter the keyword: " keyword

if [ -z "$keyword" ]; then
  echo "No keyword entered. Exiting."
  exit 1
fi

echo "🔍 Searching for all Kubernetes resources (excluding events) containing '$keyword'..."

# Gather namespaced resources (excluding events)
namespaced_matches=$(kubectl api-resources --verbs=list --namespaced -o name | \
  grep -v '^events$' | \
  xargs -n 1 kubectl get --all-namespaces -o name 2>/dev/null | \
  grep "$keyword" || true)

# Gather cluster-scoped resources (excluding events)
cluster_matches=$(kubectl api-resources --verbs=list --namespaced=false -o name | \
  grep -v '^events$' | \
  xargs -n 1 kubectl get -o name 2>/dev/null | \
  grep "$keyword" || true)

# List resources that match the keyword (excluding events)
all_matches=$(printf "%s\n%s" "$namespaced_matches" "$cluster_matches" | \
  grep -v '^event\.events\.k8s\.io/' | \
  sort | uniq | sed '/^$/d')

if [ -z "$all_matches" ]; then
  echo "⛔ No resources found containing '$keyword' (excluding events)."
  exit 0
fi

echo
echo "The following resources will be deleted:"
echo "----------------------------------------"
echo "$all_matches"
echo "----------------------------------------"
echo

# Confirm and process
read -p "⚠️ WARNING: Are you sure you want to delete ALL of these resources? Type 'yes' to confirm: " confirm

if [ "$confirm" != "yes" ]; then
  echo "🔔 Aborted. No resources were deleted."
else
  echo "🔔 Proceeding with deletion..."
  echo "$all_matches" | while read -r resource; do
    echo "Processing $resource..."
    # First try normal deletion
    if ! kubectl delete "$resource" --wait=false 2>/dev/null; then
      # If normal deletion fails, try removing finalizers
      echo "⚠️ Normal deletion failed, attempting to remove finalizers..."
      # Handle both standard and ArgoCD application formats
      if [[ $resource == */*/* ]]; then
        # Standard namespaced resource (format: namespace/type/name)
        IFS='/' read -r ns type name <<< "$resource"
        kubectl patch -n "$ns" "$type" "$name" --type=merge -p '{"metadata":{"finalizers":null}}'
      elif [[ $resource == *argoproj.io* ]]; then
        # ArgoCD application (format: type/name, but needs argocd namespace)
        IFS='/' read -r type name <<< "$resource"
        kubectl patch -n argocd "$type" "$name" --type=merge -p '{"metadata":{"finalizers":null}}'
      else
        # Cluster-scoped resource (format: type/name)
        IFS='/' read -r type name <<< "$resource"
        kubectl patch "$type" "$name" --type=merge -p '{"metadata":{"finalizers":null}}'
      fi
      # Retry deletion after removing finalizers
      kubectl delete "$resource"
    fi
  done
  echo "✅ Deletion completed for all matching resources."
fi
