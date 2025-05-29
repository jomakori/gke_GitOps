#!/bin/bash

# ------------------------------------------------------------------
# Script: k8s-delete-by-keyword.sh
# Description: Prompts for a keyword, lists all Kubernetes resources
#              (all types, all namespaces, including cluster-scoped) whose
#              names contain the keyword, and asks for confirmation before deletion.
# Usage: ./.useful-scripts/k8s-delete-by-keyword.sh
# ------------------------------------------------------------------

set -e

# Prompt for the keyword to search for
echo "Global Kubernetes Resource Deletion Script"
echo "This script will search for all Kubernetes resources containing a specified keyword and delete them."
read -p "Enter the keyword: " keyword

if [ -z "$keyword" ]; then
  echo "No keyword entered. Exiting."
  exit 1
fi

echo "🔍 Searching for all Kubernetes resources containing '$keyword'..."

# Gather namespaced resources
namespaced_matches=$(kubectl api-resources --verbs=list --namespaced -o name | \
  xargs -n 1 kubectl get --all-namespaces -o name 2>/dev/null | \
  grep "$keyword" || true)

# Gather cluster-scoped resources
cluster_matches=$(kubectl api-resources --verbs=list --namespaced=false -o name | \
  xargs -n 1 kubectl get -o name 2>/dev/null | \
  grep "$keyword" || true)

# List resources that match the keyword
all_matches=$(printf "%s\n%s" "$namespaced_matches" "$cluster_matches" | sort | uniq | sed '/^$/d')

if [ -z "$all_matches" ]; then
  echo "⛔ No resources found containing '$keyword'."
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
  echo "$all_matches" | xargs -r kubectl delete
  echo "✅ All matching resources have been deleted."
fi
