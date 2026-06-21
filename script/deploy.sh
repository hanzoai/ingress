#!/usr/bin/env bash
set -e

if [ -n "${VERSION}" ]; then
  echo "Deploying..."
else
  echo "Skipping deploy"
  exit 0
fi

git config --global user.email "${HANZO_DEPLOYER_EMAIL}"
git config --global user.name "Hanzo Deployer"

# Image publishing is handled by .github/workflows/build.yaml, which builds
# multi-arch images and pushes them to ghcr.io/hanzoai/ingress. There is no
# separate library-image repo to update.

echo "Deployed"
