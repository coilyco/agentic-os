#!/usr/bin/env bash
# Build the declarative payload fan-in graph into the local Docker daemon.

set -euo pipefail

: "${TREEHOUSE_READ_TOKEN:?the full image fetches the private agent-compose release: mint a treehouse read token first}"

timeout --preserve-status --kill-after=5m "$BUILD_TIMEOUT" \
  env TAG="$TAG" PLATFORM="$PLATFORM" TREEHOUSE_READ_TOKEN="$TREEHOUSE_READ_TOKEN" \
  docker buildx bake --progress=plain --file docker/dev-base/docker-bake.hcl
