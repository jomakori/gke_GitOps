#!/bin/bash

# Purpose: Used for linting + testing helm charts in CI
# Note: You can test charts locally, using the script inside: .useful-scripts/ct_check.sh

# Declarations
changed_files="$1"
IFS=' ' read -r -a files_array <<< "$changed_files"
helm_dirs=()
GREEN="\033[32m"
BLUE="\033[34m"
RESET="\033[0m"

# Determine helm directories from changed files
for file in "${files_array[@]}"; do
  if [[ "$file" == *"/argocd-appset/"* || "$file" == *"/helm/"* ]]; then
    ## Grab helm path from file path
    helm_dir=$(dirname "$file" | sed 's#^\(.*\)\(argocd-appset\|helm/[^/]*\).*#\1\2#')
    ## Add to the helm_dirs array if not already present
    if [[ ! " ${helm_dirs[@]} " =~ " ${helm_dir} " ]]; then
      helm_dirs+=("$helm_dir")
    fi
  fi
done

# Run chart testing on each Helm directory
echo "::notice::Running chart testing on: ${helm_dirs[@]}"
for dir in "${helm_dirs[@]}"; do
  ct lint --charts "$dir" --validate-maintainers=false
  ## Fail-catch
  if [ $? -ne 0 ]; then
    echo "::ERROR::Chart testing failed on: ${dir}."
    exit 1
  fi
done

# Confirm success
echo "✅ Lint and Tested ${#helm_dirs[@]} changed Helm charts."
