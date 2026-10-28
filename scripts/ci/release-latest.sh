#!/usr/bin/env bash

set -euo pipefail

: "${TAG:?TAG is required}"
repo=${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is not set}

# create-release would make a release for a tag typed wrong, so refuse that first.
check() {
  if ! gh api "repos/$repo/releases/tags/$TAG" >/dev/null 2>&1; then
    echo "::error::no release exists for $TAG, so there is nothing to mark latest" >&2
    exit 1
  fi
}

verify() {
  local latest
  latest=$(gh api "repos/$repo/releases/latest" --jq .tag_name)
  if [ "$latest" != "$TAG" ]; then
    echo "::error::releases/latest reads $latest, expected $TAG" >&2
    exit 1
  fi
  echo "releases/latest reads $latest"
}

case "${1:-}" in
  check) check ;;
  verify) verify ;;
  *)
    echo "usage: $0 check|verify" >&2
    exit 2
    ;;
esac
