#!/usr/bin/env bash

set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)

validate() {
  bash "$repo_root/scripts/ci-command.sh" bash "$repo_root/scripts/ci/repo-test-gate.sh"
  just aos-test
  # The embedded client is what a release serves, so the gate tests against a
  # staged build and fails if the binary would embed nothing.
  just aterm-client-embed
  ATERM_REQUIRE_EMBEDDED_CLIENT=1 just aterm-test
}

build() {
  just aos-release-build
  just aos-release-package
  just aos-release-check
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

# The formula and manifest name GitHub URLs, and the ser8 mirror copies the assets
# there after the release, so a bump that landed first would 404 every install.
wait_github_assets() {
  local urls url missing deadline err last_err=""
  urls=$(grep -ho "https\{0,1\}://[^\"' ]*/releases/download/[^\"' ]*" dist/aos.rb dist/aos.json | sort -u)
  if [ -z "$urls" ]; then
    echo "::error::no release URLs in dist/aos.rb or dist/aos.json" >&2
    exit 1
  fi
  deadline=$(( $(date +%s) + ${AOS_ASSET_WAIT:-1200} ))
  while :; do
    missing=""
    for url in $urls; do
      if ! err=$(curl -fsSL --max-time 60 -r 0-0 -o /dev/null "$url" 2>&1); then
        missing="$missing $url"
        last_err=$err
      fi
    done
    if [ -z "$missing" ]; then
      echo "every release asset answers"
      return 0
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
      echo "::error::the release assets are still missing, so the tap and bucket are not bumped (last curl error: ${last_err//$'\n'/ }):$missing" >&2
      exit 1
    fi
    sleep "${AOS_ASSET_POLL:-30}"
  done
}

update_homebrew() {
  if [ -z "${TAP_WRITE_TOKEN:-}" ]; then
    echo "::warning::TAP_WRITE_TOKEN is absent; skipping Homebrew update" >&2
    exit 0
  fi
  git clone --depth 1 \
    https://forgejo.coilysiren.me/coilyco/homebrew-tap.git tap
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
    "https://coilyco-ops:${TAP_WRITE_TOKEN}@forgejo.coilysiren.me/coilyco/homebrew-tap.git" \
    HEAD:main
}

update_scoop() {
  if [ -z "${SCOOP_WRITE_TOKEN:-}" ]; then
    echo "::warning::SCOOP_WRITE_TOKEN is absent; skipping Scoop update" >&2
    exit 0
  fi
  git clone --depth 1 \
    https://forgejo.coilysiren.me/coilyco/scoop-bucket.git bucket
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
    "https://coilyco-ops:${SCOOP_WRITE_TOKEN}@forgejo.coilysiren.me/coilyco/scoop-bucket.git" \
    HEAD:main
}

case "${1:-}" in
  validate) validate ;;
  build) build ;;
  attach) attach ;;
  wait-github-assets) wait_github_assets ;;
  update-homebrew) update_homebrew ;;
  update-scoop) update_scoop ;;
  *)
    echo "usage: $0 validate|build|attach|wait-github-assets|update-homebrew|update-scoop" >&2
    exit 2
    ;;
esac
