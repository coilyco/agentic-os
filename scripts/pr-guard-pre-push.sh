#!/usr/bin/env bash
# Pre-push guard for the catalog suite: keep the default branch PR-only.
#
# The rule is deliberately NOT "no push without an open PR". That is
# unsatisfiable: a forge cannot open a pull request for a ref that does not
# exist on it yet, so the first push of any branch can never have one. What is
# enforceable, and what this implements, is:
#
#   1. Refuse any push whose destination is the remote's default branch. Works
#      against any forge, needs no credentials.
#   2. Once a branch already exists on the remote, require an open PR before
#      allowing further pushes to it. Forgejo API only, and only when
#      FORGEJO_TOKEN is present; it degrades to a notice otherwise so a missing
#      token never blocks a push.
#
# Strength: this is client-side and `git push --no-verify` skips it, as does
# pushing a ref pre-commit does not surface. It stops accidents, not intent.
# Server-side branch protection is the control that cannot be bypassed.
#
# Contract: pre-commit consumes git's stdin ref lines itself and re-exposes one
# ref through PRE_COMMIT_REMOTE_* / PRE_COMMIT_LOCAL_BRANCH. It skips deletions
# before invoking us, and surfaces only the first ref of a multi-ref push, so a
# `git push --all` is checked on one branch rather than all of them.

set -uo pipefail

remote_name="${PRE_COMMIT_REMOTE_NAME:-origin}"
remote_url="${PRE_COMMIT_REMOTE_URL:-}"
remote_branch="${PRE_COMMIT_REMOTE_BRANCH:-}"

# Nothing to judge outside a real push (e.g. `pre-commit run` by hand).
[ -n "$remote_url" ] && [ -n "$remote_branch" ] || exit 0

branch="${remote_branch#refs/heads/}"

die() { printf '\n[pr-guard] %s\n\n' "$*" >&2; exit 1; }

default_branch="$(
  git symbolic-ref --quiet --short "refs/remotes/$remote_name/HEAD" 2>/dev/null |
    sed "s#^$remote_name/##"
)"
[ -n "$default_branch" ] || default_branch="main"

if [ "$branch" = "$default_branch" ]; then
  die "refusing to push directly to '$default_branch'. Branch and open a PR.
       Deliberate override: git push --no-verify"
fi

case "$remote_url" in
  *forgejo*) ;;
  *) exit 0 ;;
esac

if [ -z "${FORGEJO_TOKEN:-}" ]; then
  printf '[pr-guard] FORGEJO_TOKEN unset; skipping PR check for %s\n' "$branch" >&2
  exit 0
fi

api_root="$(printf '%s' "$remote_url" | sed -E 's#(https?://[^/]+)/.*#\1#')/api/v1"
slug="$(printf '%s' "$remote_url" | sed -E 's#https?://[^/]+/##; s#\.git$##')"
auth_header="Authorization: token $FORGEJO_TOKEN"

published="$(
  curl -s -o /dev/null -w '%{http_code}' -H "$auth_header" \
    "$api_root/repos/$slug/branches/$branch" 2>/dev/null || echo 000
)"
# Unpublished branch: no PR can exist yet, so this push is the one that
# creates the ref a PR will later point at.
[ "$published" = "200" ] || exit 0

open_prs="$(
  curl -sf -H "$auth_header" "$api_root/repos/$slug/pulls?state=open&limit=100" 2>/dev/null |
    jq --arg b "$branch" '[.[] | select(.head.ref == $b)] | length' 2>/dev/null
)"
# A failed lookup yields empty, which must not read as "zero open PRs".
[ -n "$open_prs" ] || {
  printf '[pr-guard] PR lookup failed for %s; allowing push\n' "$branch" >&2
  exit 0
}

if [ "$open_prs" -eq 0 ]; then
  die "'$branch' is already on the remote but has no open PR.
       Open one, then push again:
         ${remote_url%.git}/compare/$default_branch...$branch"
fi

exit 0
