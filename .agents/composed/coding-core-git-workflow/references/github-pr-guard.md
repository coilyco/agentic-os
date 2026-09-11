# The GitHub PR guard

`agentic-os/scripts/github-pr-guard.sh` is a Claude Code `PreToolUse` hook that refuses GitHub pull-request writes and hands back the reason instead of a bare denial. It exists because the refusal an agent meets has to carry the alternative: a silent block reads as a broken tool and the next move is a retry, where a named one routes the work to Forgejo on the first try.

## What it refuses

* `gh pr create|merge|edit|close|reopen|ready|review|comment`, anywhere in a command, including after a `cd`, an `export`, or a pipe.
* `gh api` with `--method`/`-X` `POST|PATCH|PUT|DELETE` against a `/pulls` path, which is the escape hatch around every verb above.
* `gh api graphql` carrying `createPullRequest`, `mergePullRequest`, or `addPullRequestReview`.
* The GitHub MCP pull-request writers on any server whose name carries `github`: `create_pull_request`, `create_pull_request_with_copilot`, `merge_pull_request`, `update_pull_request`, `update_pull_request_branch`, `pull_request_review_write`, `add_comment_to_pending_review`, `add_reply_to_pull_request_comment`.

Reads are untouched. `gh pr view`, `gh pr list`, `gh pr diff`, `gh pr checks`, `gh repo list`, a `gh api` GET, and every GitHub MCP read tool pass through, because the guard is about where work lands rather than about GitHub being unreachable.

`gh issue create` is **not** refused here. The fleet rule is that Kai's own work goes to the Teable tracker, and the GitHub queue belongs to external contributors, but that is a separate boundary with its own open question (`teable:coilyco-flight-deck/agentic-os#7380`) rather than something this guard decides on the side.

## Why Forgejo owns the pull request

Forgejo is canonical and GitHub is a read-only downstream mirror, so a pull request opened on GitHub reviews and lands work on the copy rather than on the source. The mirror job then pushes Forgejo `main` to GitHub `main` without `--force`, and a GitHub-only merge commit makes that push a non-fast-forward, which fails red and needs a human admin to repair. The contract is in the `tooling-aosguard` skill, `references/forgejo-ops.md`.

## The two layers, and which one speaks

* **The hook** is the enforcement that speaks. `PreToolUse` hooks run before the settings permission rules, and its `permissionDecisionReason` is what the model reads.
* **`permissions.deny`** in `~/.claude/settings.json` carries the same `gh pr` spellings, appended by `scripts/apply-base-claude-settings.py`. It is the fail-closed backstop for a host where the hook is not wired yet, and it refuses without saying why, which is exactly why the hook exists on top of it.

The MCP path has no deny-rule backstop, because an MCP deny rule names one exact `mcp__<server>__<tool>` and the connector's server name is per-host. On a host with the hook unwired, the MCP writers are open.

## Standing it down

`AOS_GITHUB_PR_GUARD_MODE=off` passes everything through, for the case where a human has decided GitHub really is the right host. `AOS_GITHUB_PR_GUARD_LOG=<path>` records each refusal. Rollout is the `claude-hooks` ansible role in `coilyco-bridge/infrastructure`, per the authoring-vs-rollout split.
