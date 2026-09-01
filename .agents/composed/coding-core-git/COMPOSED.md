---
name: coding-core-git
description: Git + GitHub umbrella. Hard rule - never use gh api graphql without double-confirming, REST is default. Owns the full PR lifecycle (branch, commit, open, monitor CI, auto-fix, merge) and routes to ward-passthrough and git-workflow siblings.
seed:
  kind: always
---

# coding-core-git

Umbrella skill for any work that touches git or GitHub. Owns the broad keyword surface, carries the hard rules that apply to every GitHub touch, holds the full pull-request lifecycle inline, and routes to the focused siblings for ward passthrough and the `coilysiren/*` commit rules.

## Triggers

git, github, gh, gh cli, gh api, octokit, graphql, repo, repo, pull request, PR, issue, fork, branch, commit, push, workflow, action, release, tag, label.

## Hard rule: never use the GitHub GraphQL API without confirming first

Never reach for the GitHub GraphQL API (`gh api graphql`, `octokit.graphql`, raw POSTs to `/graphql`, etc.) without double-confirming with the human first. State that the proposed approach uses GraphQL, name a REST alternative if one exists, and wait for an explicit go-ahead before running or writing the call.

Applies to one-off commands, scripts, workflows, skills, and any code the human will run.

**Why:** GraphQL rate limits have burned three days of Kai's time. The recovery is slow and annoying, and the burn is silent until limits hit.

**How to apply:** default to REST (`gh api /repos/...`, search endpoints, list endpoints) and accept the extra round-trips. REST rate limits are far more forgiving and the failure mode is per-call, not per-account.

## Pull request lifecycle

Each step shows the `gh` way first, then the `git` + `curl` fallback for machines without `gh`. Detect which to use once up front (`gh auth status` succeeds -> `gh`, else extract `owner/repo` from the remote and use a token for REST calls).

- [Branch creation and commits](references/branch-and-commit.md) - naming conventions and Conventional Commits messages.
- [Conventional Commits](references/conventional-commits.md) - the encouraged commit-message house style (no longer hook-enforced).
- [Pushing and creating a PR](references/push-and-create-pr.md) - push the branch, open the PR with gh or curl.
- [Monitoring CI status](references/monitoring-ci.md) - one-shot checks, watch mode, and a curl polling loop.
- [Auto-fixing CI failures](references/auto-fixing-ci.md) - diagnose, fix, push, recheck. Cap at 3 attempts before asking.
- [CI failure patterns](references/ci-failure-patterns.md) and [build-infra failures](references/ci-build-infra-failures.md) - the failure-pattern catalog.
- [CI decision tree](references/ci-decision-tree.md) and [CI troubleshooting](references/ci-troubleshooting.md) - the auto-fix decision tree.
- [Merging](references/merging.md) - squash merge, branch cleanup, and auto-merge.
- [Workflow example and command reference](references/workflow-example-and-commands.md) - end-to-end walkthrough and a gh-vs-curl command table.

### PR body templates

- [Feature PR body](templates/pr-body-feature.md) - scaffold for a feature PR description.
- [Bugfix PR body](templates/pr-body-bugfix.md) - scaffold for a bug-fix PR description.

## Routing

- **Ward passthrough for `gh`** (audit-log binding, scope routing) - `ward-ops-gh-meta` (in ward).
- **Git workflow for `coilysiren/*` repos** (commit-to-main default, every-commit-closes-an-issue rule, readonly exceptions) - `coding-core-git-workflow`.

## See also

- [AGENTS.md "GitHub Issues - Echo on Touch"](../../../AGENTS.md) - quoting rules for `owner/repo#N` refs and issue URLs.
- agentic-os-kai#561 - this skill's origin and taxonomy history.
