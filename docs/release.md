# Release pipeline

`promote.yml` gates every `main` push and fast-forwards `release` with
`CI_RELEASE_TOKEN`. Three artifact workflows use native paths on that diff.
`aos-cli-release.yml` cuts the versioned CLI bundle, `aos-precommit-release.yml`
cuts the hook package, and `dev-base-publish.yml` publishes draft images under
`draft-${sha}` only when the promoted diff changes `docker/`. After the full
draft succeeds, it calls `release.yml` to publish the next root minor release.
Manual dispatch overrides the path filter and can resume the image graph.
`release.yml` is also a no-cancel retry and override queue. It never gates the
branch. `main` stays yolo-able, while `release` is last-known-good. Forgejo owns releases per
[forgejo-ops.md](../.agents/skills/tooling-aosguard/references/forgejo-ops.md).

Promotion only gates and advances the branch. Non-artifact merges produce no
package tag or draft image.
`release.yml` runs as a reusable workflow after the full draft is verified and
as the manual retry path. Its retag job promotes only that full manifest.
Dispatches can override `sha`, `tag`, and `source-tag` to resume or repair a
publication. The moving alias is always `:release`, independent of the dispatch
ref. Planning and release-metadata jobs bootstrap from the already-promoted
`:latest` full image, so an absent `:release` alias remains repairable through
the workflow. `draft-*` tags are staging refs for Forgejo package cleanup rules.
Language drafts and stable language cache refs are build-only artifacts.
`:latest` is a compatibility alias for `:release`.

The root `v*` train serves the dev-base image, the independent `aos-precommit-v*`
the hooks, and `aos-v*` the standalone CLI binaries and packages. See
[aos-cli.md](aos-cli.md).

## Version bump

`actions/tag-bump` defaults to minor and never parses commits. `aos-v*`
requires a shipped CLI input. `aos-precommit-v*` requires an installed hook
input. Root `v*` minor releases follow verified full-image drafts. Pre-commit
majors use `scripts/release.py --bump major`. Workflow dispatch remains the
explicit patch, major, retry, and native path-filter override.

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
[forgejo-ops.md](../.agents/skills/tooling-aosguard/references/forgejo-ops.md) for the mirror-side detail.

## Skip markers

`scripts/release.py`'s version commit carries `[skip ci]` so the hand-cut bump
does not re-trigger the workflow. The auto-pipeline no longer pushes a pin
commit at all. Shared composite actions live under `actions/*` in this repo.

## Cross-repo pre-commit baseline

Managed repos pin `aos-precommit-v*` via `rev:`. The distribution keeps the
`agentic_os` namespace but releases independently from dev-base and AOS.
The suite covers Actions policy, contracts, links, source-doc refs,
`actionlint`, Forgejo, `shellcheck`, and `typos`. `shfmt`, placeholders, and
issue refs stay opt-in in [pre-commit hygiene](pre-commit-hygiene.md). See the
[hook manifest](../.pre-commit-hooks.yaml).

`catalog-trifecta` requires the four consumer entrypoints to exist and
cross-link. It requires no AOS citation.

## Seed-skill propagation

qwen-opencode's per-repo context management wants a little language context living inside each target repo (for a Python repo, a pointer to how Kai writes Python). The composed `coding-<lang>` sources declare how they propagate with a `seed:` frontmatter block: `kind: always` (the `coding-core-git` baseline, seeded into every repo) or `kind: language` with `language` + `extensions` (seeded into repos containing those files). Target repos reference the delivered path, e.g. `.agents/skills/coding-python/SKILL.md`, or the canonical `.agents/composed/coding-python/COMPOSED.md` source.

The composed frontmatter is the source of truth. `generate-seed-skills` renders it into `agentic_os/seed_skills_data.py`, shipped in the package so consumer repos enforce the `seed-skills` hook offline, and `check-seed-skills-drift` (dogfooded in `agentic-os` only) fails if that table goes stale. This repo ships the validator half only: the actual copying and `COMPOSED.md` to `SKILL.md` promotion in target repos is Ansible's job.

## Diagnostic + utility helpers

Single-purpose validators for cryptic failure modes. These plus [`ward context-budget`](context-budget.md) are CLI/on-demand tools, not repo-content hooks, so they ship as ward verbs (agentic-os#233):

- `ward aws-config` - catches the `[profile default]` trap (SDKs read `[default]`; a misplaced region surfaces later as a useless `NoRegion`).
- `ward ssm-path` - checks parameter paths against the `/<org>/<repo>/<tier>/<tail>` schema before IAM/KMS, where a malformed path silently misses every tier policy.
- `just prod-install-ref guard|ward|aos` - returns the immutable
  generated product tag attached to the promoted `release` branch. It returns
  the literal `release` ref when promotion has no matching tag yet.
- GPG signing doctor that walks every check needed to diagnose `failed to sign the data` and names the likely fix per failure mode.

## Forgejo-canonical release actions

Composite Forgejo Actions for the brew release pipeline, each a forgejo-API-only replacement for a github-coupled marketplace action:

- `actions/tag-bump` - bump the latest semver tag by a fixed amount (minor by default, major hand-driven via the `bump` input), or run in compute-only mode before the public tag exists. Does not parse commit messages. Replaces `mathieudutour/github-tag-action`.
- `actions/create-release` - POST to forgejo Releases API with bounded JSON marshalling and timeouts. Idempotent on tag collision. Replaces `softprops/action-gh-release` for release creation.
- `actions/upload-release-asset` - POST a release asset with bounded lookup, delete, and upload calls.
- `actions/bump-formula` - rewrite a Homebrew Formula's `url ".."` line to pin the new tag + revision and PUT via forgejo Contents API with bounded lookup and write calls.

Forgejo imports use a fully qualified canonical URL:
`uses: https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os/actions/<name>@main`.
GitHub uses the mirror.

agentic-os dogfoods local `uses:` refs.
