#!/usr/bin/env bash

set -euo pipefail

validate() {
  ward exec test
  ward exec aos-test
  ward exec pre-commit-all
}

build() {
  ward exec aos-release-build
  ward exec aos-release-package
  ward exec aos-release-check
}

attach() {
  source actions/_lib/release.sh
  local json_tool assets_base asset name list_code old delete_code name_q code
  json_tool=$(release_json_tool)
  assets_base="${FORGEJO_API}/releases/${RELEASE_ID}/assets"
  for asset in dist/*; do
    name=$(basename "$asset")
    if ! list_code=$(release_curl_status /tmp/aos-assets.json \
      -H "Authorization: token ${FORGEJO_TOKEN}" \
      "$assets_base"); then
      echo "::error::release asset lookup failed" >&2
      exit 1
    fi
    if [ "$list_code" != "200" ]; then
      echo "::error::release asset lookup returned HTTP $list_code" >&2
      exit 1
    fi
    old=$(release_json_find_asset_id "$json_tool" /tmp/aos-assets.json "$name")
    if [ -n "$old" ] && [ "$old" != "null" ]; then
      if ! delete_code=$(release_curl_status /tmp/aos-asset-delete.json \
        -X DELETE \
        -H "Authorization: token ${FORGEJO_TOKEN}" \
        "$assets_base/$old"); then
        echo "::error::release asset delete failed for $name" >&2
        exit 1
      fi
      if [ "$delete_code" != "204" ]; then
        echo "::error::release asset delete returned HTTP $delete_code for $name" >&2
        exit 1
      fi
    fi
    name_q=$(release_json_make_uri "$json_tool" "$name")
    if ! code=$(release_curl_status /tmp/aos-asset-upload.json \
      -X POST \
      -H "Authorization: token ${FORGEJO_TOKEN}" \
      -F "attachment=@${asset}" \
      "$assets_base?name=$name_q"); then
      echo "::error::release asset upload failed for $name" >&2
      exit 1
    fi
    if [ "$code" != "201" ]; then
      echo "::error::release asset upload returned HTTP $code for $name" >&2
      exit 1
    fi
    echo "uploaded $name"
  done
}

update_homebrew() {
  if [ -z "${TAP_WRITE_TOKEN:-}" ]; then
    echo "::warning::TAP_WRITE_TOKEN is absent; skipping Homebrew update" >&2
    exit 0
  fi
  git clone --depth 1 \
    https://forgejo.coilysiren.me/coilyco-flight-deck/homebrew-tap.git tap
  cp dist/aos.rb tap/Formula/aos.rb
  cd tap
  git add Formula/aos.rb
  if git diff --cached --quiet; then
    exit 0
  fi
  git config user.name "coilyco-ops"
  git config user.email "coilyco-ops@coilysiren.me"
  git commit -m "chore(aos): bump formula to ${TAG} [skip ci]"
  git push \
    "https://coilyco-ops:${TAP_WRITE_TOKEN}@forgejo.coilysiren.me/coilyco-flight-deck/homebrew-tap.git" \
    HEAD:main
}

update_scoop() {
  if [ -z "${SCOOP_WRITE_TOKEN:-}" ]; then
    echo "::warning::SCOOP_WRITE_TOKEN is absent; skipping Scoop update" >&2
    exit 0
  fi
  git clone --depth 1 \
    https://forgejo.coilysiren.me/coilyco-flight-deck/scoop-bucket.git bucket
  cp dist/aos.json bucket/bucket/aos.json
  cd bucket
  git add bucket/aos.json
  if git diff --cached --quiet; then
    exit 0
  fi
  git config user.name "coilyco-ops"
  git config user.email "coilyco-ops@coilysiren.me"
  git commit -m "chore(aos): bump manifest to ${TAG} [skip ci]"
  git push \
    "https://coilyco-ops:${SCOOP_WRITE_TOKEN}@forgejo.coilysiren.me/coilyco-flight-deck/scoop-bucket.git" \
    HEAD:main
}

case "${1:-}" in
  validate) validate ;;
  build) build ;;
  attach) attach ;;
  update-homebrew) update_homebrew ;;
  update-scoop) update_scoop ;;
  *)
    echo "usage: $0 validate|build|attach|update-homebrew|update-scoop" >&2
    exit 2
    ;;
esac
