#!/usr/bin/env bash

set -euo pipefail

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
  payload=$(printf '{"tag_name":"%s","target":"%s"}' "$new_tag" "$RELEASE_SHA")
  http_code=$(curl -sS -o /tmp/tagresp.json -w '%{http_code}' \
    -X POST \
    -H "Authorization: token $FORGEJO_TOKEN" \
    -H "Content-Type: application/json" \
    -d "$payload" \
    "$FORGEJO_BASE_URL/api/v1/repos/$REPO/tags")
  case "$http_code" in
    201)
      ;;
    409)
      echo "tag already exists for $new_tag, continuing"
      ;;
    *)
      echo "::error::forgejo tag create failed: HTTP $http_code"
      cat /tmp/tagresp.json
      exit 1
      ;;
  esac
else
  echo "dry-run tag computation only; not creating $new_tag"
fi

echo "new_tag=$new_tag" >> "$GITHUB_OUTPUT"
echo "new_version=$new_version" >> "$GITHUB_OUTPUT"
echo "previous_tag=$prev_tag" >> "$GITHUB_OUTPUT"
{
  echo "changelog<<EOF"
  if [ -n "$prev_tag" ]; then
    git log --pretty=format:'- %s (%h)' "$range"
  else
    git log --pretty=format:'- %s (%h)' HEAD
  fi
  echo ""
  echo "EOF"
} >> "$GITHUB_OUTPUT"
