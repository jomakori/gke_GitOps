#!/bin/bash

# Purpose: Used for linting + testing helm charts in CI
# Note: You can test charts locally, using the script inside: .useful-scripts/ct_check.sh

# Get the list of changed files from the environment variable
changed_files="$1"

# Convert the string of changed files into an array
IFS=' ' read -r -a files_array <<< "$changed_files"

# Initialize an array to hold Helm directories
helm_dirs=()

# Iterate over each file to find Helm directories
for file in "${files_array[@]}"; do
  # Check if the file is within a Helm directory containing 'templates'
  if [[ "$file" == *"/templates/"* ]]; then
    # Extract the directory path up to the Helm chart level
    helm_dir=$(dirname "$file" | sed 's|/templates.*||')
    # Add to the helm_dirs array if not already present
    if [[ ! " ${helm_dirs[@]} " =~ " ${helm_dir} " ]]; then
      helm_dirs+=("$helm_dir")
    fi
  fi
done

# Run ct lint-and-install on each Helm directory
for dir in "${helm_dirs[@]}"; do
  ## Test apps/services
  if [[ "$dir" == *"helm"* ]]; then
    echo "Running ct lint-and-install on $dir"
    ct lint-and-install --charts "$dir" --validate-maintainers=false
  ## Lint argocd templates
  else
    echo "Running ct lint on $dir"
    ct lint --charts "$dir" --validate-maintainers=false
  fi

  ## fail-catch
  if [ $? -ne 0 ]; then
    echo "ERROR: ct command failed on: $dir"
    exit 1
  fi
done

echo "✅ Lint and Tested ${#helm_dirs[@]} changed Helm charts."
