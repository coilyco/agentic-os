#!/usr/bin/env bash

set -euo pipefail

source "${GITHUB_ACTION_PATH}/../_lib/release.sh"
json_tool=$(release_json_tool)

if [ ! -f "$FILE" ]; then
  echo "::error::file not found: $FILE" >&2
  exit 1
fi

asset_name="${NAME:-$(basename "$FILE")}"
release_host_init
assets_base="$RELEASE_API/repos/$REPO/releases/$RELEASE_ID/assets"
echo "checking existing assets for $asset_name"

# Forgejo lists every asset in one reply. GitHub pages at 100.
existing_id=""
page=1
while :; do
  list_url=$assets_base
  [ "$RELEASE_HOST" = github ] && list_url="$assets_base?per_page=100&page=$page"
  if ! list_code=$(release_curl_status /tmp/uploadasset-list.json \
    "${RELEASE_HDRS[@]}" \
    "$list_url"); then
    echo "::error::$RELEASE_HOST asset list request timed out or failed"
    cat /tmp/uploadasset-list.json
    exit 1
  fi
  if [ "$list_code" != "200" ]; then
    echo "::error::$RELEASE_HOST asset list failed: HTTP $list_code"
    cat /tmp/uploadasset-list.json
    exit 1
  fi
  existing_id=$(release_json_find_asset_id "$json_tool" /tmp/uploadasset-list.json "$asset_name")
  [ -n "$existing_id" ] && break
  [ "$RELEASE_HOST" = github ] || break
  [ "$(release_json_count "$json_tool" /tmp/uploadasset-list.json)" -ge 100 ] || break
  page=$((page + 1))
done

if [ -n "$existing_id" ] && [ "$existing_id" != "null" ]; then
  echo "asset '$asset_name' already attached (id=$existing_id), deleting"
  # GitHub deletes an asset by its own id, not under the release.
  del_url="$assets_base/$existing_id"
  [ "$RELEASE_HOST" = github ] && del_url="$RELEASE_API/repos/$REPO/releases/assets/$existing_id"
  if ! del_code=$(release_curl_status /tmp/uploadasset-del.json \
    -X DELETE \
    "${RELEASE_HDRS[@]}" \
    "$del_url"); then
    echo "::error::$RELEASE_HOST asset delete request timed out or failed"
    cat /tmp/uploadasset-del.json
    exit 1
  fi
  if [ "$del_code" != "204" ]; then
    echo "::error::$RELEASE_HOST asset delete failed: HTTP $del_code"
    cat /tmp/uploadasset-del.json
    exit 1
  fi
fi

name_q=$(release_json_make_uri "$json_tool" "$asset_name")
# GitHub takes the raw file body on the uploads host. Forgejo takes multipart.
if [ "$RELEASE_HOST" = github ]; then
  upload_args=(
    -H "Content-Type: application/octet-stream"
    --data-binary "@${FILE}"
    "$RELEASE_UPLOADS/repos/$REPO/releases/$RELEASE_ID/assets?name=$name_q"
  )
else
  upload_args=(-F "attachment=@${FILE}" "$assets_base?name=$name_q")
fi
if ! http_code=$(release_curl_status /tmp/uploadasset.json \
  -X POST \
  "${RELEASE_HDRS[@]}" \
  "${upload_args[@]}"); then
  echo "::error::$RELEASE_HOST asset upload request timed out or failed"
  cat /tmp/uploadasset.json
  exit 1
fi

if [ "$http_code" != "201" ]; then
  echo "::error::$RELEASE_HOST asset upload failed: HTTP $http_code"
  cat /tmp/uploadasset.json
  exit 1
fi

asset_url=$(release_json_get_field "$json_tool" /tmp/uploadasset.json browser_download_url)
echo "asset_url=$asset_url" >> "$GITHUB_OUTPUT"
echo "uploaded: $asset_url"
