#!/usr/bin/env bash
# Shared helpers for the release composite actions. Each action runs on a
# Forgejo runner or a GitHub one, and release_host_init picks the API. See
# docs/release.md#release-actions-on-forgejo-and-github.

# Sets RELEASE_HOST (github|forgejo), RELEASE_API, RELEASE_UPLOADS,
# RELEASE_AUTH (the token) and RELEASE_HDRS (curl header args). Reads the
# action inputs as env: RELEASE_HOST_INPUT, RELEASE_API_URL,
# RELEASE_UPLOADS_URL, RELEASE_TOKEN_INPUT, FORGEJO_TOKEN, FORGEJO_BASE_URL.
release_host_init() {
  RELEASE_HOST=${RELEASE_HOST_INPUT:-auto}
  if [ "$RELEASE_HOST" = auto ]; then
    # A Forgejo runner reports its own server URL here, never github.com.
    if [ "${GITHUB_SERVER_URL:-}" = "https://github.com" ]; then
      RELEASE_HOST=github
    else
      RELEASE_HOST=forgejo
    fi
  fi

  # The neutral `token` wins. forgejo_token defaults to the job token, so a
  # caller that sets neither still authenticates as the job.
  RELEASE_AUTH=${RELEASE_TOKEN_INPUT:-${FORGEJO_TOKEN:-}}
  if [ -z "$RELEASE_AUTH" ]; then
    echo "::error::no API token: set the token input" >&2
    exit 1
  fi

  case "$RELEASE_HOST" in
    github)
      RELEASE_API=${RELEASE_API_URL:-https://api.github.com}
      RELEASE_UPLOADS=${RELEASE_UPLOADS_URL:-${RELEASE_API_URL:-https://uploads.github.com}}
      RELEASE_HDRS=(
        -H "Authorization: Bearer $RELEASE_AUTH"
        -H "Accept: application/vnd.github+json"
        -H "X-GitHub-Api-Version: 2022-11-28"
      )
      ;;
    forgejo)
      RELEASE_API=${RELEASE_API_URL:-${FORGEJO_BASE_URL:?forgejo_base_url is empty}/api/v1}
      RELEASE_UPLOADS=$RELEASE_API
      RELEASE_HDRS=(-H "Authorization: token $RELEASE_AUTH")
      ;;
    *)
      echo "::error::host must be auto|github|forgejo, got '$RELEASE_HOST'" >&2
      exit 1
      ;;
  esac
}

release_json_tool() {
  if command -v python3 >/dev/null 2>&1; then
    printf '%s\n' python3
    return
  fi

  if command -v python >/dev/null 2>&1; then
    printf '%s\n' python
    return
  fi

  if ! command -v jq >/dev/null 2>&1; then
    if command -v apt-get >/dev/null 2>&1; then
      echo "installing jq with a bounded apt-get call" >&2
      timeout --preserve-status --kill-after=15s 2m sh -c 'apt-get update -qq && apt-get install -y -qq jq'
    elif command -v apk >/dev/null 2>&1; then
      echo "installing jq with a bounded apk call" >&2
      timeout --preserve-status --kill-after=15s 2m apk add --no-cache jq
    else
      echo "::error::need python, jq, apt-get, or apk for JSON handling" >&2
      exit 1
    fi
  fi

  printf '%s\n' jq
}

release_json_make_create_payload() {
  tool=$1
  tag=$2
  name=$3
  body=$4

  case "$tool" in
    python3|python)
      "$tool" - "$tag" "$name" "$body" <<'PY'
import json
import sys

tag, name, body = sys.argv[1:]
print(json.dumps({
    "tag_name": tag,
    "name": name,
    "body": body,
    "draft": False,
    "prerelease": False,
}))
PY
      ;;
    jq)
      jq -n --arg tag "$tag" --arg name "$name" --arg body "$body" \
        '{tag_name:$tag, name:$name, body:$body, draft:false, prerelease:false}'
      ;;
    *)
      echo "::error::unsupported JSON tool: $tool" >&2
      exit 1
      ;;
  esac
}

release_json_get_field() {
  tool=$1
  file=$2
  field=$3

  case "$tool" in
    python3|python)
      "$tool" - "$file" "$field" <<'PY'
import json
import sys

path, field = sys.argv[1:]
with open(path, encoding="utf-8") as fh:
    data = json.load(fh)
print(data[field])
PY
      ;;
    jq)
      jq -r ".$field" "$file"
      ;;
    *)
      echo "::error::unsupported JSON tool: $tool" >&2
      exit 1
      ;;
  esac
}

release_json_find_asset_id() {
  tool=$1
  file=$2
  asset_name=$3

  case "$tool" in
    python3|python)
      "$tool" - "$file" "$asset_name" <<'PY'
import json
import sys

path, asset_name = sys.argv[1:]
with open(path, encoding="utf-8") as fh:
    assets = json.load(fh)
for asset in assets:
    if asset.get("name") == asset_name:
        print(asset.get("id", ""))
        raise SystemExit(0)
PY
      ;;
    jq)
      jq -r --arg n "$asset_name" '.[] | select(.name==$n) | .id' "$file" | head -n1
      ;;
    *)
      echo "::error::unsupported JSON tool: $tool" >&2
      exit 1
      ;;
  esac
}

release_json_count() {
  tool=$1
  file=$2

  case "$tool" in
    python3|python)
      "$tool" - "$file" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as fh:
    print(len(json.load(fh)))
PY
      ;;
    jq)
      jq -r 'length' "$file"
      ;;
    *)
      echo "::error::unsupported JSON tool: $tool" >&2
      exit 1
      ;;
  esac
}

release_json_make_uri() {
  tool=$1
  text=$2

  case "$tool" in
    python3|python)
      "$tool" - "$text" <<'PY'
from urllib.parse import quote
import sys

print(quote(sys.argv[1], safe=""))
PY
      ;;
    jq)
      jq -rn --arg n "$text" '$n|@uri'
      ;;
    *)
      echo "::error::unsupported JSON tool: $tool" >&2
      exit 1
      ;;
  esac
}

release_curl_status() {
  out_file=$1
  shift

  timeout --preserve-status --kill-after=15s 2m \
    curl -sS --connect-timeout 10 --max-time 120 -o "$out_file" -w '%{http_code}' "$@"
}
