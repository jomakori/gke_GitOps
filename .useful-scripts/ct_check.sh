#!/bin/bash

# Purpose: Used for linting + testing helm charts locally

# Usage:
# $ .useful-scripts/ct_check.sh <path-to-helm-chart> # MacOS/Linux only
## Note: Helm Charts are hosted in:
## - apps/helm/<app-name>
## - services/helm/<service-name>

# Declarations
GREEN='\033[0;32m'
NC='\033[0m' # No Color
dir=$1
cluster_context=$(kubectl config current-context 2>/dev/null)

# Check if ct (Chart Testing) and yamllint are installed
if ! command -v ct &> /dev/null || ! command -v yamllint &> /dev/null
then
    missing_tools=""
    if ! command -v ct &> /dev/null; then missing_tools+="ct "; fi
    if ! command -v yamllint &> /dev/null; then missing_tools+="yamllint"; fi
    echo "$missing_tools is not installed. Please install it using 'brew install chart-testing' and/or 'brew install yamllint'."
    exit 1
fi

# Set up a trap to remove files on ct command failure
trap 'rm -rf $dir/charts; rm -f $dir/Chart.lock' EXIT ERR INT

# Confirm connection to cluster
if [ -z "$cluster_context" ]; then
    echo "ERROR: The EKS cluster credentials aren't set"
    exit 1
else
    printf "${GREEN}Testing out changes on: $cluster_context${NC}\n"
fi

# update the helm deps
echo "Updating Helm repo in cluster..."

# Prompt user for action
echo "Choose an option:"
echo "  1) Lint"
echo "  2) Lint & Test"
read -p "Enter choice [1/2]: " user_choice

if [ "$user_choice" = "1" ]; then
    printf "${GREEN}CT: Linting Helm Chart: ${dir} ${NC}\n"
    ct lint --charts $dir --validate-maintainers=false
elif [ "$user_choice" = "2" ]; then
    printf "${GREEN}CT: Lint & Test Helm Chart: ${dir} ${NC}\n"
    ct lint-and-install --charts $dir --validate-maintainers=false
else
    echo "Invalid choice. Exiting."
    exit 1
fi
