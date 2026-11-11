#!/usr/bin/env bash
# Required gate for the Android release build (COI-3634). A PR that touches it must
# see its `apk` job pass, any other passes at once. See references/android.md.
set -euo pipefail

repo="${GITHUB_REPOSITORY:?}" pr="${PR_NUMBER:?}" sha="${PR_HEAD_SHA:?}"
wait_s="${ATERM_GATE_WAIT:-3600}" absent_s="${ATERM_GATE_ABSENT:-300}" poll_s="${ATERM_GATE_POLL:-30}"
# The same paths aterm-android.yml runs on for a pull request.
paths='^(aterm-android/|scripts/ci/aterm-android[^/]*\.sh$|\.github/workflows/aterm-android\.yml$)'

files=$(gh api "repos/$repo/pulls/$pr/files" --paginate --jq '.[].filename')
if ! printf '%s\n' "$files" | grep -q -E "$paths"; then
  echo "no Android build path in this PR, so there is no apk job to wait for"
  exit 0
fi

state() {
  gh api "repos/$repo/commits/$sha/check-runs?per_page=100" --jq \
    '[.check_runs[] | select(.name == "apk")] | sort_by(.id) | last
     | if . == null then "absent" elif .status != "completed" then "pending" else .conclusion end'
}

waited=0
while :; do
  now=$(state)
  case "$now" in
    success | skipped) echo "the apk job finished: $now"; exit 0 ;;
    pending) ;;
    absent) [ "$waited" -lt "$absent_s" ] || { echo "::error::this PR touches the Android build and no apk job ran for $sha"; exit 1; } ;;
    *) echo "::error::the apk job finished: $now. A red release build does not merge."; exit 1 ;;
  esac
  [ "$waited" -lt "$wait_s" ] || { echo "::error::the apk job did not finish in ${wait_s}s"; exit 1; }
  sleep "$poll_s"
  waited=$((waited + (poll_s > 0 ? poll_s : 1)))
done
