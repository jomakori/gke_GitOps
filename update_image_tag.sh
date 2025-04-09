#!/bin/bash
set -e

# Function to check if required environment variables are set
check_env_vars() {
  local missing_vars=()
  for var in GH_USER_EMAIL GH_USER ENV_NUM CONTAINER_TAG GIT_MESSAGE; do
    if [ -z "${!var}" ]; then
      missing_vars+=("$var")
    fi
  done

  if [ ${#missing_vars[@]} -ne 0 ]; then
    echo "Error: The following environment variables are not set: ${missing_vars[*]}"
    exit 1
  fi
}

# Check required environment variables
check_env_vars
echo "✅ Env vars confirmed"

# Update the image tag in the values.yaml file
yq -i e '.environments[env(ENV_NUM)].tag = env(CONTAINER_TAG) | (... | select(tag == "!!merge")) tag = ""' apps/helm/notes-app/values.yaml
echo "✅ Image tag updated"

# Check if there are changes to commit
if git diff --quiet; then
  echo "The container tags have already been updated to: $CONTAINER_TAG. Deployment skipped..."
  exit 0
fi

# Commit and push changes
git config --global user.email "$GH_USER_EMAIL"
git config --global user.name "$GH_USER"
git add apps/helm/notes-app/values.yaml
git commit -m "$GIT_MESSAGE"
git push

# Confirm the deployment
case "$ENV_NUM" in
  0) echo "✅ Changes deployed to staging" ;;
  1) echo "✅ Changes released to production" ;;
esac
