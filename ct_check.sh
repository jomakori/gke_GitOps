#!/bin/bash

# Purpose: Used for linting + testing helm charts in CI
# Note: You can test charts locally, using the script inside: .useful-scripts/ct_check.sh

# Declarations
changed_files="$1"
IFS=' ' read -r -a files_array <<< "$changed_files"
helm_dirs=()
GREEN="\033[32m"
BLUE="\033[34m"
RED="\033[31m"
RESET="\033[0m"

# Determine helm directories from changed files
for file in "${files_array[@]}"; do
  if [[ "$file" == *"/templates/"* || "$file" == *"/helm/"* ]]; then
    ## Extract the directory path up to the level above 'templates' or 'helm'
    helm_dir=$(dirname "$file" | sed 's|\(/templates\|/helm\).*||')
    ## Add to the helm_dirs array if not already present
    if [[ ! " ${helm_dirs[@]} " =~ " ${helm_dir} " ]]; then
      helm_dirs+=("$helm_dir")
    fi
  fi
done

# Run chart testing on each Helm directory
echo "::notice::Running chart testing on: ${helm_dirs[@]}"

for dir in "${helm_dirs[@]}"; do
  ## Test apps/services
  if [[ "$dir" == *"helm"* ]]; then
    printf "${GREEN}Running ct lint-and-install on: ${RESET}" "$dir"
    ct lint-and-install --charts "$dir" --validate-maintainers=false
  
  ## Lint argocd templates
  else
    printf "${BLUE}Running ct lint on: ${RESET}" "$dir"
    ct lint --charts "$dir" --validate-maintainers=false
  fi

  ## Fail-catch
  if [ $? -ne 0 ]; then
    printf "${RED}ERROR: ct command failed on: ${RESET}" "$dir"
    exit 1
  fi
done

echo "✅ Lint and Tested ${#helm_dirs[@]} changed Helm charts."
