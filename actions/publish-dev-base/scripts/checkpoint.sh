#!/usr/bin/env bash
# Skip publication when the full target manifest already exists.

set -euo pipefail

read -r -a aliases <<< "${ALIAS:-} ${EXTRA_ALIASES:-}"
alias_args=()
for alias in "${aliases[@]}"; do
  [ -n "$alias" ] && alias_args+=(--alias "$alias")
done
source_args=()
if [ -n "${SOURCE_TAG:-}" ]; then
  source_args+=(--source-tag "$SOURCE_TAG")
fi
if uv run python scripts/dev-base-build.py \
  --registry "$IMAGE_BASE" \
  --tag "$TAG" \
  "${alias_args[@]}" \
  check \
  --mode "$MODE" \
  "${source_args[@]}"
then
  echo "skip_publish=true" >> "$GITHUB_OUTPUT"
else
  rc=$?
  if [ "$rc" -ne 1 ]; then
    exit "$rc"
  fi
  echo "skip_publish=false" >> "$GITHUB_OUTPUT"
fi
