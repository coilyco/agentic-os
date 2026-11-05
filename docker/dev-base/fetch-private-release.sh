#!/usr/bin/env bash
# Download named assets of one private GitHub release and check each against the
# release's SHA256SUMS (COI-3399). Contract: docs/dev-base-image.md.
#
# usage: fetch-private-release.sh TOKEN_FILE OWNER/REPO TAG DEST ASSET...
#
# The token comes from a file, a BuildKit secret mount, and goes to curl as a
# config on stdin. It is in no argv, no environment and no image layer. No
# `set -x` here on purpose, and a caller that traces must not trace this body.

set -euo pipefail

token_file=${1:?usage: fetch-private-release.sh TOKEN_FILE OWNER/REPO TAG DEST ASSET...}
repo=${2:?repository is required}
tag=${3:?release tag is required}
dest=${4:?destination directory is required}
shift 4
[ "$#" -gt 0 ] || { echo "no asset named" >&2; exit 2; }

if [ ! -s "$token_file" ]; then
  echo "${token_file} is missing or empty: pass the read token as a BuildKit secret" >&2
  exit 1
fi

# GITHUB_API_URL is the variable Actions sets, and the seam the tests point at a fake.
api="${GITHUB_API_URL:-https://api.github.com}/repos/${repo}"
mkdir -p "$dest"
release=$(mktemp)
trap 'rm -f "$release"' EXIT

# curl keeps a custom Authorization header off a redirect to another host, which
# is what stops the token reaching the storage host the asset endpoint redirects to.
api_curl() {
  printf 'header = "Authorization: Bearer %s"\n' "$(tr -d '\r\n' <"$token_file")" \
    | curl -K - --retry 5 --retry-all-errors --retry-delay 2 -fsSL \
      -H "X-GitHub-Api-Version: 2022-11-28" "$@"
}

if ! api_curl -H "Accept: application/vnd.github+json" -o "$release" "${api}/releases/tags/${tag}"; then
  echo "cannot read release ${tag} of ${repo}: the tag may not exist, the token may lack read access, or it may have expired (an App token lives one hour)" >&2
  exit 1
fi

fetch_asset() {
  local name=$1 id
  id=$(python3 -I -c '
import json, sys
for asset in json.load(open(sys.argv[1])).get("assets", []):
    if asset["name"] == sys.argv[2]:
        print(asset["id"])
        break
' "$release" "$name")
  [ -n "$id" ] || { echo "release ${tag} of ${repo} has no asset ${name}" >&2; return 1; }
  api_curl -H "Accept: application/octet-stream" -o "${dest}/${name}" "${api}/releases/assets/${id}"
  echo "fetched ${name} ($(wc -c <"${dest}/${name}" | tr -d ' ') bytes)"
}

for name in "$@"; do fetch_asset "$name"; done
fetch_asset SHA256SUMS

cd "$dest"
for name in "$@"; do
  want=$(awk -v n="$name" '$2 == n || $2 == "*" n { print $1 }' SHA256SUMS | head -n1)
  [ -n "$want" ] || { echo "SHA256SUMS has no line for ${name}" >&2; exit 1; }
  got=$(sha256sum "$name" | awk '{ print $1 }')
  [ "$want" = "$got" ] || { echo "${name} sha256 ${got} is not ${want}" >&2; exit 1; }
  echo "sha256 ok ${name}"
done
rm -f SHA256SUMS
