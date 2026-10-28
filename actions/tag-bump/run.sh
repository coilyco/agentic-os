#!/usr/bin/env bash

set -euo pipefail

source "${GITHUB_ACTION_PATH}/../_lib/release.sh"

git fetch --tags --quiet

prev_tag=$(git tag --list "${TAG_PREFIX}[0-9]*.[0-9]*.[0-9]*" \
  --sort=-v:refname | head -n1 || true)
if [ -z "$prev_tag" ]; then
  prev_ver="0.0.0"
else
  prev_ver="${prev_tag#"$TAG_PREFIX"}"
fi

if [ -n "$prev_tag" ]; then
  range="${prev_tag}..HEAD"
else
  range="HEAD"
fi

create_tag="$CREATE_TAG"
bump="$BUMP"
tag_name="$TAG_NAME"
case "$create_tag" in
  true | false) ;;
  *)
    echo "::error::create_tag must be true|false, got '$create_tag'"
    exit 1
    ;;
esac
case "$bump" in
  major | minor | patch | none) ;;
  *)
    echo "::error::bump must be major|minor|patch|none, got '$bump'"
    exit 1
    ;;
esac

if [ -n "$tag_name" ]; then
  new_tag="$tag_name"
  case "$new_tag" in
    "${TAG_PREFIX}"*)
      new_version="${new_tag#"$TAG_PREFIX"}"
      ;;
    *)
      echo "::error::explicit tag '$new_tag' must start with prefix '$TAG_PREFIX'"
      exit 1
      ;;
  esac
else
  if [ "$bump" = "none" ]; then
    echo "no-bump signal; skipping tag creation"
    echo "new_tag=" >> "$GITHUB_OUTPUT"
    echo "new_version=" >> "$GITHUB_OUTPUT"
    echo "previous_tag=$prev_tag" >> "$GITHUB_OUTPUT"
    {
      echo "changelog<<EOF"
      echo ""
      echo "EOF"
    } >> "$GITHUB_OUTPUT"
    exit 0
  fi

  IFS='.' read -r maj min pat <<< "$prev_ver"
  case "$bump" in
    major)
      maj=$((maj + 1))
      min=0
      pat=0
      ;;
    minor)
      min=$((min + 1))
      pat=0
      ;;
    patch) pat=$((pat + 1)) ;;
  esac
  new_version="${maj}.${min}.${pat}"
  new_tag="${TAG_PREFIX}${new_version}"
fi

if [ -z "${new_version:-}" ]; then
  new_version="${new_tag#"$TAG_PREFIX"}"
fi

if [ "$create_tag" = "true" ]; then
  release_host_init
  case "$RELEASE_HOST" in
    github)
      # A lightweight tag through the refs API. GitHub answers 422 when it exists.
      tag_url="$RELEASE_API/repos/$REPO/git/refs"
      payload=$(printf '{"ref":"refs/tags/%s","sha":"%s"}' "$new_tag" "$RELEASE_SHA")
      exists_code=422
      ;;
    forgejo)
      tag_url="$RELEASE_API/repos/$REPO/tags"
      payload=$(printf '{"tag_name":"%s","target":"%s"}' "$new_tag" "$RELEASE_SHA")
      exists_code=409
      ;;
  esac
  if ! http_code=$(release_curl_status /tmp/tagresp.json \
    -X POST \
    "${RELEASE_HDRS[@]}" \
    -H "Content-Type: application/json" \
    -d "$payload" \
    "$tag_url"); then
    echo "::error::$RELEASE_HOST tag create request timed out or failed"
    cat /tmp/tagresp.json
    exit 1
  fi
  if [ "$http_code" = 201 ]; then
    :
  elif [ "$http_code" = "$exists_code" ] &&
    { [ "$RELEASE_HOST" = forgejo ] || grep -q 'Reference already exists' /tmp/tagresp.json; }; then
    echo "tag already exists for $new_tag, continuing"
  else
    echo "::error::$RELEASE_HOST tag create failed: HTTP $http_code"
    cat /tmp/tagresp.json
    exit 1
  fi
else
  echo "dry-run tag computation only; not creating $new_tag"
fi

echo "new_tag=$new_tag" >> "$GITHUB_OUTPUT"
echo "new_version=$new_version" >> "$GITHUB_OUTPUT"
echo "previous_tag=$prev_tag" >> "$GITHUB_OUTPUT"
# A first tag would carry the whole history into an env var past the exec
# limit, so keep the newest commits and count the rest.
changelog_max=100
{
  echo "changelog<<EOF"
  git log --pretty=format:'- %s (%h)' -n "$changelog_max" "$range"
  echo ""
  total=$(git rev-list --count "$range")
  if [ "$total" -gt "$changelog_max" ]; then
    echo "- ... and $((total - changelog_max)) older commits"
  fi
  echo "EOF"
} >> "$GITHUB_OUTPUT"
