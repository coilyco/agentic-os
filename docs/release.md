# Release pipeline

`promote.yml` gates every `main` push and fast-forwards `release` with
`CI_RELEASE_TOKEN`. Two affected-surface workflows react to that diff.
`aos-precommit-release.yml` cuts an `aos-precommit-v*` release only for
installed hook changes. `dev-base-publish.yml` publishes draft images under
`draft-${sha}` only when its tier classifier finds work. Manual dispatch
overrides either decision and can resume the image graph. `release.yml` is a
no-cancel manual retry queue and never gates the branch. `main` stays
yolo-able, while `release` is last-known-good. Forgejo owns releases per
[forgejo-github-mirror-contract.md](forgejo-github-mirror-contract.md).

Promotion only gates and advances the branch. Non-artifact merges produce no
package tag or draft image.
`release.yml` is the manual retry path and no longer runs on push. Its language
and full retag jobs wait for the draft source tags. Dispatches can override
`sha`, `tag`, and `source-tag` to resume the publish or retag.
`draft-*` tags are commit-scoped staging refs for Forgejo package cleanup
rules. `:latest` is a compatibility alias for `:release`.

The root `v*` train serves the dev-base image. Hook consumers pin the
independent `aos-precommit-v*` train. The standalone CLI publishes binaries and
packages on its `aos-v*` train. See [aos-cli-release.md](aos-cli-release.md).

## Version bump

`actions/tag-bump` defaults to minor and never parses commits. `aos-v*`
requires a shipped CLI input. `aos-precommit-v*` requires an installed hook
input. Pre-commit majors use `scripts/release.py --bump major`. Other trains
use workflow dispatch, which also overrides each classifier.

`actions/tag-bump` also has a compute-only mode: derive the next semver first,
create the public tag and Forgejo release only at the end.

## Manual re-run (enqueue-miss recovery)

A `workflow_dispatch` retries publication without a dummy commit. Dispatch
against `release`. Image controls resume one closure at a time.

Dispatch `aos-precommit-release.yml` against `release` to retry or override a
package release. An explicit tag reuses that version idempotently. An empty tag
uses the selected bump.

## Consumer pin (derived from the tag)

`apply-agentic-os-hooks` resolves the latest `aos-precommit-v*` tag at read
time. Its `default_rev()` falls back to `FALLBACK_REV` only without a fetched
package tag.

Tags are refs, so automatic releases need no follow-up pin commit.

The `aos-precommit` pyproject `version`, `uv.lock`, and the `FALLBACK_REV`
floor are reconciled on hand-cut package releases by `scripts/release.py`, so
they track major bumps and may lag automatic tags between majors. That lag is
cosmetic. Consumers pin by git rev, and `FALLBACK_REV` is only read when no
package tag is present.

## Mirror to GitHub

`.forgejo/workflows/mirror-to-github.yml` fast-forwards Forgejo `main`, root
`v*`, and `aos-precommit-v*` tags onto the read-only GitHub mirror
(`coilysiren/agentic-os`) where downstream refs resolve. It is
fast-forward-only, never `--force`, and no-ops without the PAT. See
[mirror-to-github.md](mirror-to-github.md) for the mirror-side detail.

## Skip markers

`scripts/release.py`'s version commit carries `[skip ci]` so the hand-cut bump
does not re-trigger the workflow. The auto-pipeline no longer pushes a pin
commit at all. Shared composite actions live under `actions/*` in this repo.
