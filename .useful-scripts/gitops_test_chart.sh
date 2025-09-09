#!/bin/bash

# Purpose: Used for linting, testing, installing, and uninstalling helm charts locally

# Usage:
# $ ./.useful-scripts/gitops_test_chart.sh <path-to-helm-chart> # MacOS/Linux only
## Note: Helm Charts are hosted in:
## - apps/helm/<app-name>
## - services/helm/<service-name>

# Declarations
GREEN='\033[0;32m'
NC='\033[0m' # No Color
dir="$PWD"
cluster_context=$(kubectl config current-context 2>/dev/null)

# Check if ct (Chart Testing) and yamllint are installed
check_dependencies() {
    if ! command -v ct &>/dev/null || ! command -v yamllint &>/dev/null; then
        missing_tools=""
        if ! command -v ct &>/dev/null; then missing_tools+="ct "; fi
        if ! command -v yamllint &>/dev/null; then missing_tools+="yamllint"; fi
        echo "$missing_tools is not installed. Please install it using 'brew install chart-testing' and/or 'brew install yamllint'."
        exit 1
    fi
}

# Lint function (option 1)
lint_chart() {
    printf "${GREEN}CT: Linting Helm Chart: ${dir} ${NC}\n"
    ct lint --charts "$dir" --validate-maintainers=false
}

# CT Test function (option 2)
test_chart_ct() {
    printf "${GREEN}CT: Test Helm Chart: ${dir} ${NC}\n"
    read -p "Set test values file (press Enter to skip): " CUSTOM_VALUES
        read -p "Set Timeout (default 5m, press Enter to skip): " CUSTOM_TIMEOUT
        TIMEOUT="5m"
    if [ -n "$CUSTOM_TIMEOUT" ]; then
        TIMEOUT="$CUSTOM_TIMEOUT"
    fi
    EXTRA_ARGS="--timeout=$TIMEOUT"
    if [ -n "$CUSTOM_VALUES" ]; then
        EXTRA_ARGS="--values=$CUSTOM_VALUES $EXTRA_ARGS"
    fi
    ct install --chart-dirs . --charts . --helm-extra-set-args="$EXTRA_ARGS"
}

# Helm Test Function (option 3)
test_install_chart() {
    read -p "Enter a release name for testing: " RELEASE_NAME
    if [ -z "$RELEASE_NAME" ]; then
        echo "Release name cannot be empty. Exiting."
        exit 1
    fi

    read -p "Custom values file (press Enter to skip): " CUSTOM_VALUES

    printf "${GREEN}Building helm dependencies...${NC}\n"
    helm dependency build "$dir"

    printf "${GREEN}Installing helm chart with release name: $RELEASE_NAME${NC}\n"
    INSTALL_CMD="helm install \"$RELEASE_NAME\" \"$dir\" -n \"$RELEASE_NAME\" --create-namespace --wait --wait-for-jobs"
    if [ -n "$CUSTOM_VALUES" ]; then
        INSTALL_CMD="$INSTALL_CMD -f \"$CUSTOM_VALUES\""
    fi
    eval $INSTALL_CMD

    if [ $? -eq 0 ]; then
        echo "Chart installed successfully!"
        kubectl get all -n $RELEASE_NAME
    else
        echo "Failed to install chart."
    fi

    read -p "Would you like to uninstall the chart now? (y/n): " UNINSTALL_CHOICE
    if [ "$UNINSTALL_CHOICE" = "y" ] || [ "$UNINSTALL_CHOICE" = "Y" ]; then
        uninstall_chart $RELEASE_NAME
    else
        echo "Chart remains installed. You can uninstall it later by re-running the script"
    fi
}

# Uninstall chart function (accepts optional release name)
uninstall_chart() {
    local RELEASE_NAME="$1"
    if [ -z "$RELEASE_NAME" ]; then
        read -p "Enter the release name to uninstall: " RELEASE_NAME
        if [ -z "$RELEASE_NAME" ]; then
            echo "Release name cannot be empty. Exiting."
            exit 1
        fi
    fi
    helm uninstall $RELEASE_NAME -n $RELEASE_NAME
    echo "Chart uninstalled successfully!"
}

# Main script logic
check_dependencies

# Set up a trap to remove files on ct command failure
trap 'rm -rf $dir/charts; rm -f $dir/Chart.lock' EXIT ERR INT

# Confirm connection to cluster
if [ -z "$cluster_context" ]; then
    echo "ERROR: The EKS cluster credentials aren't set"
    exit 1
else
    printf "Testing out changes on: ${GREEN}$cluster_context${NC} cluster\n"
fi

# Prompt user for action
printf "Selected Helm Chart: ${GREEN}$dir${NC}\n"
echo "Choose an option:"
echo "  1) Lint"
echo "  2) Test - via ct"
echo "  3) Test - via Helm"
echo "  4) Uninstall chart"
read -p "Enter choice [1-4]: " user_choice

case "$user_choice" in
    1)
        lint_chart
        ;;
    2)
        test_chart_ct
        ;;
    3)
        test_install_chart
        ;;
    4)
        uninstall_chart
        ;;
    *)
        echo "Invalid choice. Exiting."
        exit 1
        ;;
esac
