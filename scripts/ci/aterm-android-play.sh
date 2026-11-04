#!/usr/bin/env bash
# Upload the signed AAB to a Play track via the Developer API. Switched off and never
# run against Play yet. How to switch it on: references/android.md.

set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
api="${PLAY_API:-https://androidpublisher.googleapis.com}"
token_url="${PLAY_TOKEN_URL:-https://oauth2.googleapis.com/token}"
track="${ATERM_PLAY_TRACK:-internal}"
# A never-published app only accepts draft releases, so the first run stays draft.
status="${ATERM_PLAY_RELEASE_STATUS:-draft}"

fail() {
  echo "::error::$*" >&2
  exit 1
}

b64url() {
  openssl base64 -A | tr '+/' '-_' | tr -d '='
}

# The service account key rides the environment and never touches disk or argv.
access_token() {
  local email header claims sig now
  : "${PLAY_SERVICE_ACCOUNT_JSON:?PLAY_SERVICE_ACCOUNT_JSON is required}"
  email=$(jq -r .client_email <<<"$PLAY_SERVICE_ACCOUNT_JSON")
  now=$(date +%s)
  header=$(printf '{"alg":"RS256","typ":"JWT"}' | b64url)
  claims=$(jq -nc --arg iss "$email" --arg aud "$token_url" --argjson iat "$now" \
    '{iss:$iss,scope:"https://www.googleapis.com/auth/androidpublisher",aud:$aud,iat:$iat,exp:($iat+3600)}' | b64url)
  sig=$(printf '%s.%s' "$header" "$claims" \
    | openssl dgst -sha256 -sign <(jq -r .private_key <<<"$PLAY_SERVICE_ACCOUNT_JSON") | b64url)
  curl -fsS -X POST "$token_url" \
    -d grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer -d "assertion=$header.$claims.$sig" | jq -r .access_token
}

# upload AAB VERSION_CODE NOTES: one edit adds the bundle to the track, then commits.
upload() {
  local aab="${1:?usage: upload AAB VERSION_CODE NOTES}" code="${2:?}" notes="${3:?}" pkg token edit base body
  [ -f "$aab" ] || fail "no bundle at $aab"
  [ -n "${ATERM_ANDROID_UPLOAD_KEYSTORE_B64:-}" ] || fail "the bundle is throwaway-signed, Play needs the real upload key"
  pkg=$(bash "$repo_root/scripts/ci/aterm-android.sh" app-id)
  token=$(access_token)
  base="$api/androidpublisher/v3/applications/$pkg"
  auth=(-H "Authorization: Bearer $token")
  edit=$(curl -fsS -X POST "${auth[@]}" -H "Content-Length: 0" "$base/edits" | jq -r .id)
  [ -n "$edit" ] && [ "$edit" != null ] || fail "Play returned no edit id"
  curl -fsS -X POST "${auth[@]}" -H "Content-Type: application/octet-stream" --data-binary "@$aab" \
    "$api/upload/androidpublisher/v3/applications/$pkg/edits/$edit/bundles?uploadType=media" >/dev/null
  # Play caps release notes at 500 characters.
  body=$(jq -nc --arg track "$track" --arg status "$status" --arg code "$code" --arg notes "$(head -c 500 "$notes")" \
    '{track:$track,releases:[{status:$status,versionCodes:[$code],releaseNotes:[{language:"en-US",text:$notes}]}]}')
  curl -fsS -X PUT "${auth[@]}" -H "Content-Type: application/json" -d "$body" "$base/edits/$edit/tracks/$track" >/dev/null
  curl -fsS -X POST "${auth[@]}" -H "Content-Length: 0" "$base/edits/$edit:commit" >/dev/null
  echo "uploaded version code $code to the $track track as $status"
}

cmd="${1:-}"
[ $# -gt 0 ] && shift
case "$cmd" in
  upload) upload "$@" ;;
  *) fail "usage: aterm-android-play.sh upload AAB VERSION_CODE NOTES" ;;
esac
