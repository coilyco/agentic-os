#!/usr/bin/env bash

set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
dist=${AOS_RELEASE_DIST:-"$repo_root/dist"}

# Promotion already ran the repo gate and the Go tests on this commit, so this
# covers only what a release adds: the embedded web client. See docs/release.md.
validate() {
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
  : "${TAG:?TAG is required}"
  # --clobber replaces an asset of the same name, so a retry converges.
  gh release upload "$TAG" "$dist"/* --clobber --repo "${GITHUB_REPOSITORY:?}"
}

# Commit one rendered file into a GitHub repo and push it. The token rides a
# credential helper, so it stays out of argv and out of the stored remote URL.
bump_repo() {
  local repo=$1 src=$2 dest=$3 noun=$4 base work
  : "${RELEASE_GIT_TOKEN:?RELEASE_GIT_TOKEN is required}"
  : "${TAG:?TAG is required}"
  base=${RELEASE_GIT_BASE:-https://github.com/coilyco}
  work=$(mktemp -d)
  GIT_TERMINAL_PROMPT=0 git clone --depth 1 "$base/$repo.git" "$work"
  mkdir -p "$(dirname "$work/$dest")"
  cp "$src" "$work/$dest"
  cd "$work"
  git add "$dest"
  if git diff --cached --quiet; then
    echo "$repo already carries $TAG"
    return 0
  fi
  git config user.name "coilyco-ops"
  git config user.email "coilyco-ops@coilysiren.me"
  git commit -m "chore(aos): bump $noun to ${TAG} [skip ci]"
  git -c credential.helper='!f() { echo username=x-access-token; echo "password=$RELEASE_GIT_TOKEN"; }; f' \
    push origin HEAD:main
}

update_homebrew() {
  bump_repo homebrew-tap "$dist/aos.rb" Formula/aos.rb formula
}

update_scoop() {
  bump_repo scoop-bucket "$dist/aos.json" bucket/aos.json manifest
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
