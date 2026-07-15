# Features: cross-repo tooling and release

Pre-commit baseline, diagnostics, and Forgejo-canonical release actions.

## Cross-repo pre-commit baseline

Ships canonical hook IDs each `coilysiren/*` repo pins via `rev:`. The active set covers hygiene hooks, contract hooks, `actionlint`, `shellcheck`, and `typos`. `dead-cross-links` validates Markdown links; `source-doc-refs` validates source-comment doc paths. `shfmt`, placeholders, and issue refs stay opt-in in [pre-commit hygiene](pre-commit-hygiene.md). Manual guards live in [`.pre-commit-hooks.yaml`](../.pre-commit-hooks.yaml).

## Seed-skill propagation

qwen-opencode's per-repo context management wants a little language context living inside each target repo (for a Python repo, a pointer to how Kai writes Python). The `coding-<lang>` skills declare how they propagate with a `seed:` frontmatter block: `kind: always` (the `coding-git` baseline, seeded into every repo) or `kind: language` with `language` + `extensions` (seeded into repos containing those files). Target repos reference a seeded skill by its canonical path, e.g. `.agents/skills/coding-python/SKILL.md`.

The frontmatter is the source of truth. `generate-seed-skills` renders it into `agentic_os/seed_skills_data.py`, shipped in the package so consumer repos enforce the `seed-skills` hook offline, and `check-seed-skills-drift` (dogfooded in `agentic-os` only) fails if that table goes stale. This repo ships the validator half only: the actual copying of skills into target repos is Ansible's job.

## Diagnostic + utility helpers

Single-purpose validators for cryptic failure modes. These plus [`ward context-budget`](context-budget.md) are CLI/on-demand tools, not repo-content hooks, so they ship as ward verbs (agentic-os#233):

- `ward aws-config` - catches the `[profile default]` trap (SDKs read `[default]`; a misplaced region surfaces later as a useless `NoRegion`).
- `ward ssm-path` - checks parameter paths against the `/<org>/<repo>/<tier>/<tail>` schema before IAM/KMS, where a malformed path silently misses every tier policy.
- Verbatim-echo wrapper that fences command output and clips to mobile-readable size, for the `$$ <cmd>` chat convention.
- GPG signing doctor that walks every check needed to diagnose `failed to sign the data` and names the likely fix per failure mode.

## Forgejo-canonical release actions

Composite Forgejo Actions for the brew release pipeline, each a forgejo-API-only replacement for a github-coupled marketplace action:

- `actions/tag-bump` - bump the latest semver tag by a fixed amount (minor by default, major hand-driven via the `bump` input), or run in compute-only mode before the public tag exists. Does not parse commit messages. Replaces `mathieudutour/github-tag-action`.
- `actions/create-release` - POST to forgejo Releases API with bounded JSON marshalling and timeouts. Idempotent on tag collision. Replaces `softprops/action-gh-release` for release creation.
- `actions/upload-release-asset` - POST a release asset with bounded lookup, delete, and upload calls.
- `actions/bump-formula` - rewrite a Homebrew Formula's `url ".."` line to pin the new tag + revision and PUT via forgejo Contents API with bounded lookup and write calls.

Consumed via `uses: coilyco-flight-deck/agentic-os/actions/<name>@main` from a `.forgejo/workflows/*.yml`. Issued `${{ github.token }}` covers writes.

agentic-os dogfoods these actions in `.forgejo/workflows/`, using local `uses: ./actions/...` refs so the repo never waits on its mirror. Release is split so `promote.yml` gates every main push and fast-forwards `release`, `dev-base-publish.yml` publishes the draft image family on the promoted SHA with tier reruns, and `release.yml` stays the manual retry path. PR retriggers need a real tracked diff. Consumer pin is tag-derived (agentic-os#238). Major bumps stay hand-cut via `scripts/release.py`. `workflow_dispatch` re-fires the retry stage on enqueue miss. Walkthrough: [docs/release.md](release.md).
