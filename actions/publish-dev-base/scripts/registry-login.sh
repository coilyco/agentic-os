#!/usr/bin/env bash
# Authenticate the image publisher to GHCR with the workflow token.

set -euo pipefail

retry() {
  local attempts="$1"
  shift
  local delay=2
  local attempt=1
  while true; do
    if "$@"; then
      return 0
    fi
    if [ "$attempt" -ge "$attempts" ]; then
      return 1
    fi
    echo "registry login failed, retrying in ${delay}s" >&2
    sleep "$delay"
    attempt=$((attempt + 1))
    delay=$((delay * 2))
  done
}

if [ -z "${REGISTRY_TOKEN:-}" ]; then
  echo "::error::REGISTRY_TOKEN not set. Pass the workflow GITHUB_TOKEN, and grant the job packages: write. See docs/dev-base-image.md." >&2
  exit 1
fi
retry 4 sh -c 'printf "%s" "$REGISTRY_TOKEN" | docker login ghcr.io -u "${GITHUB_ACTOR:-coilyco-ops}" --password-stdin'
