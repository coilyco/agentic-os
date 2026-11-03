#!/usr/bin/env bash
# Pre-push guard for the catalog suite: keep the default branch PR-only.
#
# The rule is deliberately NOT "no push without an open PR". That is
# unsatisfiable: a forge cannot open a pull request for a ref that does not
# exist on it yet, so the first push of any branch can never have one. What is
# enforceable, and what this implements, is:
#
#   1. Refuse any push whose destination is the remote's default branch. No lane
#      exempts a repo from this any more. Works against any forge, needs no
#      credentials.
#   2. Once a branch already exists on the remote, require an open PR before
#      allowing further pushes to it. GitHub remotes only, through `gh`; it
#      degrades to a notice when `gh` is missing or unauthenticated so that
#      never blocks a push.
#   3. Name the merged PR when one exists, because "no open PR" and "its PR
#      already merged" want opposite next moves: open one, versus branch again
#      from main. Asking the forge is the only way to see a squash or rebase
#      merge, whose branch tip is not an ancestor of the commit it merged as
#      (agentic-os#1034).
#
# Strength: this is client-side and `git push --no-verify` skips it, as does
# pushing a ref pre-commit does not surface. It stops accidents, not intent.
# Server-side branch protection is the control that cannot be bypassed.
#
# Lane: this guard used to stand down on `merge-remote-main`, because that lane
# was defined as the push to `main` and refusing it left `--no-verify`, which the
# fleet rules forbid, as the only way through (agentic-os#1321). That lane is
# retired and the stand-down went with it, so rule 1 is now unconditional and a
# repo cannot declare its way past it. The read survives only to name the lane in
# the refusal, which is what tells an agent whether it is looking at a repo that
# has not been migrated yet. The lane is declared once as `ward.workflow` in
# AGENTS.md frontmatter and `agentic_os.generators.generate_git_workflow` owns
# reading it, so this asks that owner through `--print-lane` rather than growing
# a second frontmatter parser. pre-commit builds no environment for a
# `language: script` hook, so the resolver is looked for in three places, in
# order: `AOS_LANE_READER`, PATH, then the python env pre-commit built for this
# same hook repo's python hooks, which sits beside this scripts/ directory.
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

# Print the lane resolver to call, or nothing. See the header for the order.
lane_reader() {
  if [ -n "${AOS_LANE_READER:-}" ]; then
    printf '%s' "$AOS_LANE_READER"
    return
  fi
  if command -v generate-git-workflow >/dev/null 2>&1; then
    printf 'generate-git-workflow'
    return
  fi
  # Expansion, not dirname: the refusal below stays reachable with an empty PATH.
  local candidate
  for candidate in "${BASH_SOURCE[0]%/*}"/../py_env-*/bin/generate-git-workflow; do
    [ -x "$candidate" ] && { printf '%s' "$candidate"; return; }
  done
}

if [ "$branch" = "$default_branch" ]; then
  reader="$(lane_reader)"
  lane=""
  [ -n "$reader" ] && lane="$("$reader" --print-lane 2>/dev/null)"

  # Separates "this repo declares no lane" from "the lane could not be read".
  # Both refuse now, but they want different follow-up.
  [ -n "$reader" ] ||
    printf '[pr-guard] no lane reader found; treating this repo as undeclared\n' >&2

  die "refusing to push directly to '$default_branch'. Branch and open a PR.
       No lane exempts a repo from this: 'merge-remote-main' was the one that
       did and it is retired. This repo declares ${lane:-no lane}.
       Deliberate override: git push --no-verify"
fi

case "$remote_url" in
  *github.com*) ;;
  *) exit 0 ;;
esac

if ! command -v gh >/dev/null 2>&1 || ! gh auth status >/dev/null 2>&1; then
  printf '[pr-guard] gh missing or unauthenticated; skipping PR check for %s\n' "$branch" >&2
  exit 0
fi

slug="$(printf '%s' "$remote_url" | sed -E 's#^(https?://[^/]+/|git@[^:]+:|ssh://git@[^/]+/)##; s#\.git$##')"

# Unpublished branch: no PR can exist yet, so this push is the one that
# creates the ref a PR will later point at.
gh api "repos/$slug/branches/$branch" >/dev/null 2>&1 || exit 0

# Prints jq's answer over the PRs whose head is this branch, in one state.
branch_prs() {
  gh pr list --repo "$slug" --head "$branch" --state "$1" --limit 100 \
    --json number --jq "$2" 2>/dev/null
}

# Open-only: one page of every state would drop an older open PR and refuse a
# legitimate push. The open set is small enough that one page holds it.
open_count="$(branch_prs open 'length')"
# A failed lookup yields empty, which must not read as "zero open PRs".
[ -n "$open_count" ] || {
  printf '[pr-guard] PR lookup failed for %s; allowing push\n' "$branch" >&2
  exit 0
}
[ "$open_count" -eq 0 ] || exit 0

# The push is refused either way now. This read only picks which refusal is
# true, so a miss costs the better message rather than the verdict.
merged_pr="$(branch_prs merged 'map(.number) | max // empty')"

if [ -n "$merged_pr" ]; then
  die "'$branch' already merged as PR #$merged_pr. This push would strand the
       work on a dead branch that nothing points at.
       Branch again from '$default_branch' and open a new PR:
         git switch $default_branch && git pull && git switch -c <new-branch>
       Deliberate override: git push --no-verify"
fi

die "'$branch' is already on the remote but has no open PR.
     Open one, then push again:
       https://github.com/$slug/compare/$default_branch...$branch"

exit 0
