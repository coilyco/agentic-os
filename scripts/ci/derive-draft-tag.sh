#!/usr/bin/env bash

set -euo pipefail

if [ -n "${DRAFT_TAG_OVERRIDE:-}" ]; then
  echo "draft_tag=$DRAFT_TAG_OVERRIDE" >> "$GITHUB_OUTPUT"
else
  echo "draft_tag=draft-${SHA}" >> "$GITHUB_OUTPUT"
fi
