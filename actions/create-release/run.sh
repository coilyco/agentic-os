#!/usr/bin/env bash

set -euo pipefail

source "${GITHUB_ACTION_PATH}/../_lib/release.sh"

rel_name="${NAME:-$TAG}"
json_tool=$(release_json_tool)

echo "creating Forgejo release for $TAG"
payload=$(release_json_make_create_payload "$json_tool" "$TAG" "$rel_name" "$BODY")

if ! http_code=$(release_curl_status /tmp/relresp.json \
  -X POST \
  -H "Authorization: token $FORGEJO_TOKEN" \
  -H "Content-Type: application/json" \
  -d "$payload" \
  "$FORGEJO_BASE_URL/api/v1/repos/$REPO/releases"); then
  echo "::error::forgejo release create request timed out or failed"
  cat /tmp/relresp.json
  exit 1
fi

case "$http_code" in
  201)
    release_id=$(release_json_get_field "$json_tool" /tmp/relresp.json id)
    ;;
  409)
    echo "release already exists for $TAG, looking it up"
    if ! http_code=$(release_curl_status /tmp/relresp.json \
      -H "Authorization: token $FORGEJO_TOKEN" \
      "$FORGEJO_BASE_URL/api/v1/repos/$REPO/releases/tags/$TAG"); then
      echo "::error::forgejo release lookup timed out or failed"
      cat /tmp/relresp.json
      exit 1
    fi
    if [ "$http_code" != "200" ]; then
      echo "::error::forgejo release lookup failed: HTTP $http_code"
      cat /tmp/relresp.json
      exit 1
    fi
    release_id=$(release_json_get_field "$json_tool" /tmp/relresp.json id)
    ;;
  *)
    echo "::error::forgejo release create failed: HTTP $http_code"
    cat /tmp/relresp.json
    exit 1
    ;;
esac

echo "release_id=$release_id" >> "$GITHUB_OUTPUT"
