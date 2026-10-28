#!/usr/bin/env bash

set -euo pipefail

source "${GITHUB_ACTION_PATH}/../_lib/release.sh"

rel_name="${NAME:-$TAG}"
json_tool=$(release_json_tool)
release_host_init

echo "creating $RELEASE_HOST release for $TAG"
payload=$(release_json_make_create_payload "$json_tool" "$TAG" "$rel_name" "$BODY")

if ! http_code=$(release_curl_status /tmp/relresp.json \
  -X POST \
  "${RELEASE_HDRS[@]}" \
  -H "Content-Type: application/json" \
  -d "$payload" \
  "$RELEASE_API/repos/$REPO/releases"); then
  echo "::error::$RELEASE_HOST release create request timed out or failed"
  cat /tmp/relresp.json
  exit 1
fi

# Forgejo answers 409 for an existing release, GitHub 422 with code already_exists.
if [ "$RELEASE_HOST" = github ] && [ "$http_code" = 422 ] &&
  grep -q '"already_exists"' /tmp/relresp.json; then
  http_code=409
fi

case "$http_code" in
  201)
    release_id=$(release_json_get_field "$json_tool" /tmp/relresp.json id)
    ;;
  409)
    echo "release already exists for $TAG, looking it up"
    if ! http_code=$(release_curl_status /tmp/relresp.json \
      "${RELEASE_HDRS[@]}" \
      "$RELEASE_API/repos/$REPO/releases/tags/$TAG"); then
      echo "::error::$RELEASE_HOST release lookup timed out or failed"
      cat /tmp/relresp.json
      exit 1
    fi
    if [ "$http_code" != "200" ]; then
      echo "::error::$RELEASE_HOST release lookup failed: HTTP $http_code"
      cat /tmp/relresp.json
      exit 1
    fi
    release_id=$(release_json_get_field "$json_tool" /tmp/relresp.json id)
    ;;
  *)
    echo "::error::$RELEASE_HOST release create failed: HTTP $http_code"
    cat /tmp/relresp.json
    exit 1
    ;;
esac

echo "release_id=$release_id" >> "$GITHUB_OUTPUT"
