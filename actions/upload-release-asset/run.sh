#!/usr/bin/env bash

set -euo pipefail

source "${GITHUB_ACTION_PATH}/../_lib/release.sh"
json_tool=$(release_json_tool)

if [ ! -f "$FILE" ]; then
  echo "::error::file not found: $FILE" >&2
  exit 1
fi

asset_name="${NAME:-$(basename "$FILE")}"
assets_base="$FORGEJO_BASE_URL/api/v1/repos/$REPO/releases/$RELEASE_ID/assets"
echo "checking existing assets for $asset_name"

if ! list_code=$(release_curl_status /tmp/uploadasset-list.json \
  -H "Authorization: token $FORGEJO_TOKEN" \
  "$assets_base"); then
  echo "::error::forgejo asset list request timed out or failed"
  cat /tmp/uploadasset-list.json
  exit 1
fi
if [ "$list_code" != "200" ]; then
  echo "::error::forgejo asset list failed: HTTP $list_code"
  cat /tmp/uploadasset-list.json
  exit 1
fi
existing_id=$(release_json_find_asset_id "$json_tool" /tmp/uploadasset-list.json "$asset_name")

if [ -n "$existing_id" ] && [ "$existing_id" != "null" ]; then
  echo "asset '$asset_name' already attached (id=$existing_id), deleting"
  if ! del_code=$(release_curl_status /tmp/uploadasset-del.json \
    -X DELETE \
    -H "Authorization: token $FORGEJO_TOKEN" \
    "$assets_base/$existing_id"); then
    echo "::error::forgejo asset delete request timed out or failed"
    cat /tmp/uploadasset-del.json
    exit 1
  fi
  if [ "$del_code" != "204" ]; then
    echo "::error::forgejo asset delete failed: HTTP $del_code"
    cat /tmp/uploadasset-del.json
    exit 1
  fi
fi

name_q=$(release_json_make_uri "$json_tool" "$asset_name")
if ! http_code=$(release_curl_status /tmp/uploadasset.json \
  -X POST \
  -H "Authorization: token $FORGEJO_TOKEN" \
  -F "attachment=@${FILE}" \
  "$assets_base?name=$name_q"); then
  echo "::error::forgejo asset upload request timed out or failed"
  cat /tmp/uploadasset.json
  exit 1
fi

if [ "$http_code" != "201" ]; then
  echo "::error::forgejo asset upload failed: HTTP $http_code"
  cat /tmp/uploadasset.json
  exit 1
fi

asset_url=$(release_json_get_field "$json_tool" /tmp/uploadasset.json browser_download_url)
echo "asset_url=$asset_url" >> "$GITHUB_OUTPUT"
echo "uploaded: $asset_url"
