# Release pipeline

Two-stage Forgejo-canonical release (ward#1117 / aos#469). Stage 1:
`promote.yml` gates every `main` push, publishes the dev-base family under
`draft-${sha}`, then fast-forwards `release` with `CI_RELEASE_TOKEN`. Stage 2:
`release.yml` runs on `release` pushes only under a no-cancel `concurrency`
queue, so promoted shas release in sequence. `main` stays yolo-able. `release`
is last-known-good. Forgejo owns the release and tag per
[forgejo-github-mirror-contract.md](forgejo-github-mirror-contract.md).

`release.yml` computes semver, retags the draft image family to `vX.Y.Z`,
`:release`, and `:latest`, verifies those manifests, then cuts the public
git/release tag. The release queue owns semver, so two main pushes cannot race
to claim the same version. The expensive image build already happened on
`main`, before the release branch moved. There is no test gate on `release`:
promote.yml already ran the suite on the exact sha it fast-forwarded.

`draft-*` tags are commit-scoped staging refs for Forgejo package cleanup
rules. `:latest` is a compatibility alias for `:release`.

agentic-os is consumed as pre-commit hooks pinned by `rev:` tag, not a brew
formula or a prebuilt binary, so there is nothing to attach to the release and
no formula to bump. The only downstream artifact is the git tag itself.

## Why not release-please

release-please is PR-driven, and `coilysiren/agentic-os` has
`hasPullRequestsEnabled = false` on GitHub (the no-PR-on-GitHub stance). Rather
than port release-please to Forgejo, agentic-os reuses the forgejo-API-only
composite actions it already ships for the rest of the fleet. No PR, no
manifest config, no GitHub API calls.

## Version bump

`actions/tag-bump` runs with no bump input, so every push-to-main release is a
minor bump. Commit messages are never parsed. For a major, run
`scripts/release.py --bump major` to cut `vN.0.0` by hand (its commit carries
`[skip ci]`). The actions are referenced locally (`uses: ./actions/...`).

`actions/tag-bump` also has a compute-only mode: derive the next semver first,
create the public tag and Forgejo release only at the end.

## Manual re-run (enqueue-miss recovery)

A `workflow_dispatch` re-fires the pipeline by hand, no dummy commit - dispatch
against the `release` ref. It is the recovery lever for agentic-os#240 (missed
push enqueue). Its `bump` input defaults to `minor`. `promote.yml` also takes a
dispatch to re-attempt a promotion without a new main push.

## Consumer pin (derived from the tag)

The tag every consumer repo inherits when `apply-agentic-os-hooks` rolls out the
hook block is resolved from the **latest git tag at read time**, not a committed
constant. `default_rev()` in `scripts/apply-agentic-os-hooks.py` reads
`git tag --list 'v*'` and falls back to `FALLBACK_REV` only when no tag is
fetched.

Deriving the pin from the tag is the point of agentic-os#238: tags are refs,
not tracked files, so cutting a release needs no followup bump commit. That
removed the old `bump-pin` job and its `CI_RELEASE_TOKEN` push. Now the release
is just the tag.

pyproject `version`, `uv.lock`, and the `FALLBACK_REV` floor are reconciled on
hand-cut releases by `scripts/release.py`, so they track major bumps and may
lag the tag between majors. That lag is cosmetic: consumers pin by git rev, and
`FALLBACK_REV` is only read when no tag is present.

## Mirror to GitHub

`.forgejo/workflows/mirror-to-github.yml` fast-forwards Forgejo `main` and
`v*` tags onto the read-only GitHub mirror (`coilysiren/agentic-os`) where the
fleet's `uses:` refs resolve. It is fast-forward-only, never `--force`, and
no-ops without the PAT. See [mirror-to-github.md](mirror-to-github.md) for the
mirror-side detail.

## Skip markers

`scripts/release.py`'s version commit carries `[skip ci]` so the hand-cut bump
does not re-trigger the workflow. The auto-pipeline no longer pushes a pin
commit at all. Shared composite actions live under `actions/*` in this repo.
