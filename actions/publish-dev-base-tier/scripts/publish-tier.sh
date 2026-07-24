#!/usr/bin/env bash

set -euo pipefail

summary_file="${GITHUB_STEP_SUMMARY:-}"
read -r -a aliases <<< "${ALIAS:-} ${EXTRA_ALIASES:-}"
alias_args=()
for alias in "${aliases[@]}"; do
  [ -n "$alias" ] && alias_args+=(--alias "$alias")
done
base_args=(
  uv run python scripts/dev-base-build.py
  --registry "$IMAGE_BASE"
  --tag "$TAG"
  "${alias_args[@]}"
)
if [ "$MODE" = "build" ]; then
  publish_cmd=(
    timeout --preserve-status --kill-after=5m "$BUILD_TIMEOUT"
    "${base_args[@]}"
    build
    --push
    --tier "$TIER"
    --platforms "$PLATFORMS"
  )
elif [ "$MODE" = "promote" ]; then
  if [ -z "${SOURCE_TAG:-}" ]; then
    echo "::error::source-tag is required in promote mode" >&2
    exit 1
  fi
  publish_cmd=(
    timeout --preserve-status --kill-after=5m "$BUILD_TIMEOUT"
    "${base_args[@]}"
    promote
    --source-tag "$SOURCE_TAG"
    --tier "$TIER"
  )
else
  echo "::error::unknown mode: ${MODE}" >&2
  exit 1
fi

command_line="$(printf '%q ' "${publish_cmd[@]}")"
command_line="${command_line% }"
log_file="$(mktemp)"

run_logged() {
  set +e
  "$@" 2>&1 | tee "$log_file"
  status=${PIPESTATUS[0]}
  set -e
  return "$status"
}

if run_logged "${publish_cmd[@]}"; then
  exit 0
else
  rc=$?
fi
if [ "$TIER" = "core" ] && [ -n "$summary_file" ]; then
  {
    echo "### core publish failure"
    echo ""
    echo "- mode: ${MODE}"
    echo "- tier: ${TIER}"
    echo "- exit code: ${rc}"
    echo "- failing command:"
    echo '```bash'
    printf '%s\n' "$command_line"
    echo '```'
    echo ""
    echo "Recent output:"
    echo '```text'
    tail -n 60 "$log_file"
    echo '```'
  } >> "$summary_file"
fi
exit "$rc"
