#!/usr/bin/env bash

set -euo pipefail

source "${GITHUB_ACTION_PATH}/../_lib/release.sh"
json_tool=$(release_json_tool)
release_host_init

# An empty template means this host's clone URL.
if [ -z "$URL_TEMPLATE" ]; then
  case "$RELEASE_HOST" in
    github) URL_TEMPLATE='https://github.com/{REPO}.git' ;;
    forgejo) URL_TEMPLATE="${FORGEJO_BASE_URL}/{REPO}.git" ;;
  esac
fi
url_value="${URL_TEMPLATE//\{REPO\}/$REPO}"
message="${MSG_TEMPLATE//\{TAG\}/$TAG}"

if ! meta_code=$(release_curl_status /tmp/formula-meta.json \
  "${RELEASE_HDRS[@]}" \
  "$RELEASE_API/repos/$REPO/contents/$FORMULA_PATH?ref=$BRANCH"); then
  echo "::error::$RELEASE_HOST contents lookup timed out or failed"
  cat /tmp/formula-meta.json
  exit 1
fi
if [ "$meta_code" != "200" ]; then
  echo "::error::$RELEASE_HOST contents lookup failed: HTTP $meta_code"
  cat /tmp/formula-meta.json
  exit 1
fi
cur_sha=$(release_json_get_field "$json_tool" /tmp/formula-meta.json sha)
tmp=$(mktemp -d)
release_json_get_field "$json_tool" /tmp/formula-meta.json content | base64 -d > "$tmp/formula.rb"

new_line="  url \"${url_value}\", tag: \"${TAG}\", revision: \"${NEW_REVISION}\""
awk -v repl="$new_line" '
  BEGIN { done = 0 }
  /^  url "/ && !done { print repl; done = 1; next }
  { print }
' "$tmp/formula.rb" > "$tmp/formula.new.rb"

new_b64=$(base64 < "$tmp/formula.new.rb" | tr -d '\n')
if [ "$json_tool" = jq ]; then
  put_payload=$(jq -n --arg msg "$message" --arg c "$new_b64" \
    --arg sha "$cur_sha" --arg branch "$BRANCH" \
    '{message:$msg, content:$c, sha:$sha, branch:$branch}')
else
  put_payload=$("$json_tool" - "$message" "$new_b64" "$cur_sha" "$BRANCH" <<'PY'
import json
import sys

msg, content, sha, branch = sys.argv[1:]
print(json.dumps({
    "message": msg,
    "content": content,
    "sha": sha,
    "branch": branch,
}))
PY
  )
fi

if ! http_code=$(release_curl_status /tmp/putresp.json \
  -X PUT \
  "${RELEASE_HDRS[@]}" \
  -H "Content-Type: application/json" \
  -d "$put_payload" \
  "$RELEASE_API/repos/$REPO/contents/$FORMULA_PATH"); then
  echo "::error::$RELEASE_HOST formula bump request timed out or failed"
  cat /tmp/putresp.json
  exit 1
fi
if [ "$http_code" != "200" ] && [ "$http_code" != "201" ]; then
  echo "::error::$RELEASE_HOST formula bump failed: HTTP $http_code"
  cat /tmp/putresp.json
  exit 1
fi
