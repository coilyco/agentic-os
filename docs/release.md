# Release pipeline

Three-stage Forgejo-canonical release (ward#1117 / aos#469). Stage 1:
`promote.yml` gates every `main` push and fast-forwards `release` with
`CI_RELEASE_TOKEN`. Stage 2: `dev-base-publish.yml` publishes the draft full
dev-base image under `draft-${sha}` on the promoted SHA, and its manual
dispatch path can resume that image. Stage 3: `release.yml`
is manual retry only under a no-cancel queue, so retries stay sequenced and
never gate the branch. `main` stays yolo-able. `release` is last-known-good.
Forgejo owns the release and tag per [forgejo-github-mirror-contract.md](forgejo-github-mirror-contract.md).

`promote.yml` only computes the repo gate and advances the branch. The draft
publish workflow keeps image availability separate from branch promotion.
`release.yml` is the manual retry path and no longer runs on push. Its retag
job waits for the draft source tag. Dispatches can override `sha`, `tag`, and
`source-tag` to resume the publish or retag.
`draft-*` tags are commit-scoped staging refs for Forgejo package cleanup
rules. `:latest` is a compatibility alias for `:release`.

The root `v*` train serves hook pins and the dev-base image. The standalone CLI
publishes binaries and packages on its independent `aos-v*` train. See
[aos-cli-release.md](aos-cli-release.md).

## Why not release-please
release-please is PR-driven, and `coilysiren/agentic-os` disables GitHub PRs. Rather than port it to Forgejo, agentic-os reuses its forgejo-API-only release actions.

## Version bump

`actions/tag-bump` runs with no bump input, so every push-to-main release is minor. Commit messages are never parsed. For a major, run `scripts/release.py --bump major` to cut `vN.0.0` by hand (`[skip ci]`). The actions are referenced locally (`uses: ./actions/...`).

`actions/tag-bump` also has a compute-only mode: derive the next semver first,
create the public tag and Forgejo release only at the end.

## Manual re-run (enqueue-miss recovery)

A `workflow_dispatch` re-fires the publication retry stage by hand, no dummy
commit - dispatch against the `release` ref when the draft images already
exist and the release publication needs a retry. It is the recovery lever for
agentic-os#240 (missed enqueue). Its `bump` input defaults to `minor`, and the
image controls can resume one release closure at a time.

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
