#!/usr/bin/env bash
# Stable channel for the aterm Android APK (COI-3307). The workflow calls each
# subcommand as one step. Channel design and the Obtainium settings:
# tooling-aterm-client references/android.md.

set -euo pipefail

# The one title Obtainium filters on. Every stable release carries exactly this.
STABLE_TITLE="aterm Android stable"
STABLE_TAG_PREFIX="aterm-android-v"
# The fixed-link release. Its tag never changes, so its asset URL is bookmarkable.
FIXED_TAG="aterm-android-newest"
FIXED_TITLE="aterm Android newest (fixed link)"
APK_NAME="aterm.apk"
# A build is "new" when a commit since the last stable one touched these.
WATCHED_PATHS=(aterm-android aterm-client scripts/ci/aterm-android.sh scripts/ci/aterm-android-release.sh .github/workflows/aterm-android.yml)
NOTES_MAX=60

fail() {
  echo "::error::$*" >&2
  exit 1
}

previous_tag() {
  git tag --list "${STABLE_TAG_PREFIX}[0-9]*.[0-9]*.[0-9]*" --sort=-v:refname | head -n1
}

# changed PREV_TAG: exit 0 when a watched path changed since PREV_TAG (always for no tag).
changed() {
  local prev="${1:-}"
  [ -n "$prev" ] || return 0
  [ -n "$(git log --no-merges --format=%h -n1 "$prev..HEAD" -- "${WATCHED_PATHS[@]}")" ]
}

# plan: from EVENT_NAME and DRY_RUN, decide whether to build and whether to publish.
# Only a changed schedule day or a non-dry dispatch publishes, per android.md.
plan() {
  local event="${EVENT_NAME:?EVENT_NAME is required}" prev build publish
  prev=$(previous_tag || true)
  case "$event" in
    schedule)
      if changed "$prev"; then build=true publish=true; else build=false publish=false; fi
      ;;
    workflow_dispatch)
      build=true
      if [ "${DRY_RUN:-false}" = true ]; then publish=false; else publish=true; fi
      ;;
    *) build=true publish=false ;;
  esac
  echo "event=$event previous_tag=${prev:-none} build=$build publish=$publish"
  if [ -n "${GITHUB_OUTPUT:-}" ]; then
    printf 'build=%s\npublish=%s\nprevious_tag=%s\n' "$build" "$publish" "$prev" >>"$GITHUB_OUTPUT"
  fi
}

# notes PREV_TAG VERSION CODE SHA: the release body. One line per commit that touched
# the app since the last stable build, with the conventional-commit prefix dropped.
notes() {
  local prev="${1-}" version="${2:?usage: notes PREV_TAG VERSION CODE SHA}" code="${3:?}" sha="${4:?}" range lines total
  if [ -n "$prev" ]; then range="$prev..HEAD"; else range="HEAD"; fi
  lines=$(git log --no-merges --format='%s' "$range" -- "${WATCHED_PATHS[@]}" \
    | sed -E 's/^[a-z]+(\([^)]*\))?!?: //' | sed 's/^/* /')
  total=$(printf '%s\n' "$lines" | grep -c . || true)
  echo "aterm Android $version, version code $code, built from ${sha:0:8}."
  echo
  if [ "$total" -eq 0 ]; then
    echo "* No app changes since ${prev:-the start}. Rebuilt on request."
  else
    printf '%s\n' "$lines" | head -n "$NOTES_MAX"
    [ "$total" -le "$NOTES_MAX" ] || echo "* ... and $((total - NOTES_MAX)) older changes"
  fi
}

# publish TAG APK NOTES SHA: the stable release, then the fixed-link release.
# The fixed one goes last, so a failure never leaves it ahead of the stable channel.
publish() {
  local tag="${1:?usage: publish TAG APK NOTES SHA}" apk="${2:?}" notes_file="${3:?}" sha="${4:?}" repo="${GITHUB_REPOSITORY:?}"
  [ -f "$apk" ] || fail "no APK at $apk"
  # gh names the asset after the file, and the fixed link depends on that name.
  local stage
  stage=$(mktemp -d)
  cp "$apk" "$stage/$APK_NAME"
  apk="$stage/$APK_NAME"
  if gh release view "$tag" --repo "$repo" >/dev/null 2>&1; then
    gh release edit "$tag" --repo "$repo" --title "$STABLE_TITLE" --notes-file "$notes_file" --latest=false
  else
    gh release create "$tag" --repo "$repo" --target "$sha" --title "$STABLE_TITLE" --notes-file "$notes_file" --latest=false
  fi
  gh release upload "$tag" "$apk" --repo "$repo" --clobber
  if gh release view "$FIXED_TAG" --repo "$repo" >/dev/null 2>&1; then
    gh release edit "$FIXED_TAG" --repo "$repo" --title "$FIXED_TITLE" --notes-file "$notes_file" --prerelease --latest=false
  else
    gh release create "$FIXED_TAG" --repo "$repo" --target "$sha" --title "$FIXED_TITLE" --notes-file "$notes_file" --prerelease --latest=false
  fi
  gh release upload "$FIXED_TAG" "$apk" --repo "$repo" --clobber
  echo "stable: https://github.com/$repo/releases/tag/$tag"
  echo "fixed link: https://github.com/$repo/releases/download/$FIXED_TAG/$APK_NAME"
}

cmd="${1:-}"
[ $# -gt 0 ] && shift
case "$cmd" in
  plan | notes | publish | changed | previous_tag) "$cmd" "$@" ;;
  *) fail "usage: aterm-android-release.sh plan | notes PREV VERSION CODE SHA | publish TAG APK NOTES SHA | changed PREV | previous_tag" ;;
esac
