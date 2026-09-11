#!/usr/bin/env bash
# PreToolUse hook: refuse GitHub pull-request writes and say that Forgejo owns
# them. Behavior: the coding-core-git-workflow skill, references/github-pr-guard.md.
set -uo pipefail

mode="${AOS_GITHUB_PR_GUARD_MODE:-block}"
[ "$mode" = "off" ] && exit 0

log_file="${AOS_GITHUB_PR_GUARD_LOG:-}"

log() {
  [ -n "$log_file" ] || return 0
  { mkdir -p "$(dirname "$log_file")" 2>/dev/null \
      && printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >>"$log_file"; } 2>/dev/null || true
}

reason='Refused by the GitHub PR guard: pull requests go through Forgejo.

forgejo.coilysiren.me is canonical and GitHub is a read-only downstream mirror,
so a pull request opened on GitHub reviews and lands work on the copy instead of
on the source, and the next mirror push then fails as a non-fast-forward. Agents
on this fleet do not create, merge, edit, or review GitHub pull requests.

Open it on Forgejo instead. Push the branch to origin, then:
  aosguard ops forgejo pr create --repo <owner>/<repo> --head <branch> --base main --title "..."
The Forgejo MCP create_pull-request tool does the same thing.

If GitHub really is the right host for this one, that is a decision for the
human in front of you rather than for an agent.'

# permissionDecision rather than exit 2: the deny is a decision about this call,
# and the reason reaches the model as the refusal text rather than as stderr.
refuse() {
  log "decision=deny kind=$1 tool=$tool"
  jq -nc --arg r "$reason" '{
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      permissionDecision: "deny",
      permissionDecisionReason: $r
    }
  }'
  exit 0
}

payload=$(cat)
tool=$(printf '%s' "$payload" | jq -r '.tool_name // ""' 2>/dev/null)

# The rollout wires this on a matcher, and the matcher is not the boundary: a
# hand-widened one must not make the script judge a tool it knows nothing about.
case "$tool" in
  Bash)
    cmd=$(printf '%s' "$payload" | jq -r '.tool_input.command // ""' 2>/dev/null)
    flat=$(printf '%s' "$cmd" | tr '\n' ' ')
    lead='(^|[;&|(`$]|[[:space:]])gh[[:space:]]+'
    end='([[:space:]]|$)'
    writes='create|merge|edit|close|reopen|ready|review|comment'

    if printf '%s' "$flat" | grep -Eq "${lead}pr[[:space:]]+(${writes})${end}"; then
      refuse verb
    fi
    # `gh api` is the escape hatch around every verb above, so it is judged on
    # method and path rather than on the verb name it does not have.
    if printf '%s' "$flat" | grep -Eq "${lead}api${end}"; then
      mutate='(--method|-X)[[:space:]=]*(POST|PATCH|PUT|DELETE)'
      if printf '%s' "$flat" | grep -Eq "$mutate" && printf '%s' "$flat" | grep -Eq '/pulls'; then
        refuse rest
      fi
      if printf '%s' "$flat" | grep -Eqi 'createPullRequest|mergePullRequest|addPullRequestReview'; then
        refuse graphql
      fi
    fi
    ;;
  mcp__*)
    printf '%s' "$tool" | grep -Eqi 'github' || exit 0
    case "${tool##*__}" in
      create_pull_request | create_pull_request_with_copilot | merge_pull_request | \
        update_pull_request | update_pull_request_branch | pull_request_review_write | \
        add_comment_to_pending_review | add_reply_to_pull_request_comment)
        refuse mcp
        ;;
    esac
    ;;
esac

exit 0
