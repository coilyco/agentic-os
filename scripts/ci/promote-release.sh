#!/usr/bin/env bash

set -euo pipefail

if [ -z "${PROMOTE_TOKEN:-}" ]; then
  echo "::error::PROMOTE_TOKEN not set. Promotion needs a token that can push the release branch and start workflows, which a job token cannot (docs/release.md)." >&2
  exit 1
fi
proto="${SERVER%%://*}"
host="${SERVER#*://}"
git push "${proto}://x-access-token:${PROMOTE_TOKEN}@${host}/${REPO}.git" HEAD:release
echo "promoted $(git rev-parse --short HEAD) to release"
