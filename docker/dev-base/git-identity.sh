#!/usr/bin/env bash
# Backfill a user-scoped git identity from the image-owned identity.
# See docs/dev-base-agent-identity.md.
#
# Split out of the retired agent-name.sh, which carried this alongside naming
# for no reason beyond both running at SessionStart. Container-only: the values
# it reads are the image-level AOS_GIT_* ENV.
set -euo pipefail

# Respect the image-owned system config. Only backfill when the value is absent.
if ! git config --get user.name >/dev/null 2>&1; then
  git config --global user.name "${AOS_GIT_NAME:?AOS_GIT_NAME is required}" 2>/dev/null || true
fi
if ! git config --get user.email >/dev/null 2>&1; then
  git config --global user.email "${AOS_GIT_EMAIL:?AOS_GIT_EMAIL is required}" 2>/dev/null || true
fi
