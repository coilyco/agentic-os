#!/usr/bin/env bash

set -euo pipefail

source "${GITHUB_ACTION_PATH}/../_lib/release.sh"

rel_name="${NAME:-$TAG}"
json_tool=$(release_json_tool)
release_host_init

# Only GitHub has a latest flag. A repo has one latest across every train that
# releases into it, so a train that must not take it passes false.
latest=""
case "${MAKE_LATEST:-}" in
  "") ;;
  true | false)
    if [ "$RELEASE_HOST" = github ]; then
      latest=$MAKE_LATEST
    else
      echo "make_latest has no effect on $RELEASE_HOST, ignoring"
    fi
    ;;
  *)
    echo "::error::make_latest must be true|false, got '$MAKE_LATEST'" >&2
    exit 1
    ;;
esac

echo "creating $RELEASE_HOST release for $TAG"
payload=$(release_json_make_create_payload "$json_tool" "$TAG" "$rel_name" "$BODY" "$latest")

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
    if [ -n "$latest" ]; then
      # The create call did nothing, so re-apply the flag. This is the repair path.
      if ! http_code=$(release_curl_status /tmp/relpatch.json \
        -X PATCH \
        "${RELEASE_HDRS[@]}" \
        -H "Content-Type: application/json" \
        -d "{\"make_latest\":\"$latest\"}" \
        "$RELEASE_API/repos/$REPO/releases/$release_id"); then
        echo "::error::github release update timed out or failed"
        cat /tmp/relpatch.json
        exit 1
      fi
      if [ "$http_code" != "200" ]; then
        echo "::error::github release update failed: HTTP $http_code"
        cat /tmp/relpatch.json
        exit 1
      fi
    fi
    ;;
  *)
    echo "::error::$RELEASE_HOST release create failed: HTTP $http_code"
    cat /tmp/relresp.json
    exit 1
    ;;
esac

echo "release_id=$release_id" >> "$GITHUB_OUTPUT"
