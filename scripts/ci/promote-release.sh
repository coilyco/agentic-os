#!/usr/bin/env bash

set -euo pipefail

if [ -z "${PROMOTE_TOKEN:-}" ]; then
  echo "::error::CI_RELEASE_TOKEN not set. Promotion needs a real-user PAT for the final release branch fast-forward. Mint from SSM /forgejo/ci-release-token (docs/release.md)." >&2
  exit 1
fi
proto="${SERVER%%://*}"
host="${SERVER#*://}"
git push "${proto}://oauth2:${PROMOTE_TOKEN}@${host}/${REPO}.git" HEAD:release
echo "promoted $(git rev-parse --short HEAD) to release"
