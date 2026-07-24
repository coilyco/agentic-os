#!/usr/bin/env bash
# Reuse the persistent multi-architecture Buildx builder.

set -euo pipefail

echo "buildx before bootstrap"
docker buildx ls
docker run --privileged --rm tonistiigi/binfmt --install all
if ! docker buildx inspect aosbuilder >/dev/null 2>&1; then
  docker buildx create --name aosbuilder \
    --driver docker-container --driver-opt network=host || true
fi
if ! docker buildx inspect aosbuilder --bootstrap; then
  echo "aosbuilder exists but fails to bootstrap. Recreating it (warm cache is lost this once)"
  docker buildx rm -f aosbuilder 2>/dev/null || true
  docker buildx create --name aosbuilder \
    --driver docker-container --driver-opt network=host
  docker buildx inspect aosbuilder --bootstrap
fi
docker buildx use aosbuilder
echo "buildx after bootstrap"
docker buildx ls
