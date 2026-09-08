---
name: coding-core-git-workflow
description: Git workflow for Kai's repositories. Covers Forgejo for git, the Teable tracker, commits, pushes, PR lanes, issues, TODOs, and recovery.
---

# Git workflow

Default across `~/projects/coilyco-*/*` and `~/projects/coilysiren/*`:

<!-- TODO: a different ruleset for bridge -->

- Follow the resolved workflow for the repo and run:
- Every slug names what **you, the author** do. `-and-merge` means you merge, not that someone else will get to it.
- `pull-request-and-merge` - **the lane every repo runs.** Push a branch, open the PR, and merge it yourself once it is green. The PR is the record and the CI gate, not a wait state. Leaving one open and reporting it as awaiting someone is the failure this lane exists to prevent.
- `pull-request` - push a branch and open a Forgejo PR, then stop. You do not merge. The director merge lane takes it from there, which is exactly why this lane has no `-and-merge`. Declared by no repo now, and kept for work a director has to gate.
- `remote-branch-only` - push a branch and stop. No PR and no merge.
- `merge-remote-main` - **retired.** It was the lane that let you push `main` directly, and pushing straight to `main` ended fleet-wide. The generator no longer renders it, so a repo declaring it reads as undeclared and gets the guarded `pull-request` shape. One repo still declares it and keeps it deliberately: `coilysiren/coilysiren`, which is GitHub-canonical, carries `.agentic-os-ignore`, and wires none of the catalog hooks.
- A pushed branch always gets a PR. Only `remote-branch-only` stops at the branch, and only when the caller resolved that lane. Unassigned work defaults to `pull-request`. A branch with no PR is litter nobody reviews.
- `pr-guard` refuses any push whose destination is the default branch, with no lane exempting a repo any more. It runs at pre-push, so a repo whose other pre-push hooks are already failing cannot push at all until those are fixed. Never `--no-verify` around it.
- Run tests, linters, builds without asking. Fix failures.
- Never `--no-verify`.
- Readonly git/shell auto-allowed.
- File the tracker record first, then reference it in the commit body as `teable:<owner>/<repo>#<n>`. No trailer closes a record for you, so closing it is a separate deliberate step.
- `agentic-os-kai` only: one commit per discrete additive change.
- `git commit --amend` is fine pre-push, preferred over a "fix lint" follow-on for hook fixes. If the amend changes substance relative to the record's description, add a row in the `comments` table linked to that record so the audit trail survives. Force-push off-limits. Overrides the default Claude Code rule.

Never run destructive git commands unless the human explicitly asks. Never revert changes you didn't make.

Below are the exceptions and details.

## Recovering a commit dropped by rebase

`ward git pull --rebase` can silently drop a local commit when upstream `main` was force-pushed (release-please rewriting history). Git's default `--fork-point` heuristic reads the upstream reflog, decides the local commit is "already upstream," and drops it - no error, no conflict, just gone from `HEAD` and the working tree. The wrapper does **not** guard against this by design: it stays thin and security-focused, and the commit is only dangling, never lost. Recover it from reflog:

```
git reflog                 # find the pre-rebase HEAD (the entry just before "rebase ...")
git cherry-pick <sha>      # replay the dropped commit onto the rebased HEAD
```

Use `git reset --hard <sha>` instead of cherry-pick only when nothing else has moved since. Decided in ward#4 (no default-behavior change, reflog is the documented solve).

## Repo-specific exceptions

- **ward** - auto-push only when session's primary cwd is ward. Not if started in a sibling and cd'd in. Check env block, not live cwd.
- **infrastructure** - branch and PR, never direct push. Confirm before SSM/kubectl/cloud writes. Never print decrypted SSM values. Reach for `ward` before raw aws/kubectl.
- **message-ops** - confirm before destructive social ops (archive, delete, block). Friends-list check before any archive pass.

## Deploy knowledge

`~/projects/coilysiren/infrastructure/docs/k3s-deploy-notes.md` is source of truth for k3s topology, SSM layout, GH Actions → cluster deploys, manifest shapes, triage. Deployable repos reference it.

## Pre-commit canonical entry

Every repo has `.pre-commit-config.yaml` with offline trufflehog:

```yaml
- id: trufflehog
  name: trufflehog (secret scan, offline)
  entry: trufflehog git file://. --since-commit HEAD --no-verification --no-update --fail
  language: system
  pass_filenames: false
  stages: [pre-commit, pre-push]
```

## More detail

- [GitHub issues as work tracker](references/github-issue-tracker.md) - precedence, close-via-commit, tracker issues stay open, bot-attribution signature.
- [Guarded operator work](references/guarded-operator-work.md) - AOSguard discovery, approved bare reads, and the GitHub GraphQL wall.
- [Default TODO destination and flake discipline](references/default-todo-and-flake.md) - Teable as default tracker, never-ask-just-file, flaky-test rule.
