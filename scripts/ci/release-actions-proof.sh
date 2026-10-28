#!/usr/bin/env bash
# Steps of .github/workflows/release-actions-proof.yml that read GitHub back.
# Usage: release-actions-proof.sh payload|verify|cleanup. Needs GH_TOKEN.
set -euo pipefail

repo=${GITHUB_REPOSITORY:?}
tag=${TAG:-}
payload=${PROOF_FILE:-${RUNNER_TEMP:-/tmp}/release-actions-proof.txt}

case "${1:-}" in
  payload)
    printf 'release-actions-proof %s\n' "${GITHUB_SHA:?}" > "$payload"
    echo "file=$payload" >> "${GITHUB_OUTPUT:?}"
    ;;
  verify)
    # Read the result back from GitHub, not from the actions' own outputs.
    : "${RELEASE_ID:?}" "${SECOND_RELEASE_ID:?}"
    ref_sha=$(gh api "repos/$repo/git/ref/tags/$tag" --jq .object.sha)
    [ "$ref_sha" = "$GITHUB_SHA" ] || { echo "::error::tag $tag is at $ref_sha, want $GITHUB_SHA"; exit 1; }
    live_id=$(gh api "repos/$repo/releases/tags/$tag" --jq .id)
    [ "$live_id" = "$RELEASE_ID" ] || { echo "::error::release for $tag is $live_id, want $RELEASE_ID"; exit 1; }
    [ "$SECOND_RELEASE_ID" = "$RELEASE_ID" ] || { echo "::error::second create-release made $SECOND_RELEASE_ID, want reuse of $RELEASE_ID"; exit 1; }
    name=$(basename "$payload")
    count=$(gh api "repos/$repo/releases/$RELEASE_ID/assets" --jq "[.[] | select(.name==\"$name\")] | length")
    [ "$count" = 1 ] || { echo "::error::$count assets named $name, want exactly 1 after two uploads"; exit 1; }
    size=$(gh api "repos/$repo/releases/$RELEASE_ID/assets" --jq ".[] | select(.name==\"$name\") | .size")
    [ "$size" = "$(wc -c < "$payload" | tr -d ' ')" ] || { echo "::error::asset is $size bytes, file is different"; exit 1; }
    echo "verified: tag $tag at $ref_sha, release $RELEASE_ID reused, asset $name replaced once ($size bytes)"
    ;;
  cleanup)
    # Leave no proof tag behind, so the next run computes the same version.
    [ -n "$tag" ] || { echo "no tag was created, nothing to delete"; exit 0; }
    if id=$(gh api "repos/$repo/releases/tags/$tag" --jq .id 2>/dev/null); then
      gh api -X DELETE "repos/$repo/releases/$id"
    fi
    gh api -X DELETE "repos/$repo/git/refs/tags/$tag" || true
    echo "deleted release and tag $tag"
    ;;
  *)
    echo "usage: $0 payload|verify|cleanup" >&2
    exit 2
    ;;
esac
