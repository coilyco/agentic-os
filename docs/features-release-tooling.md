# Features: cross-repo tooling and release

Pre-commit baseline, diagnostics, and Forgejo-canonical release actions.

## Cross-repo pre-commit baseline

Managed repos pin `aos-precommit-v*` via `rev:`. The distribution keeps the
`agentic_os` namespace but releases independently from dev-base and AOS.
The suite covers Actions policy, contracts, links, source-doc refs,
`actionlint`, Forgejo, `shellcheck`, and `typos`. `shfmt`, placeholders, and
issue refs stay opt-in in [pre-commit hygiene](pre-commit-hygiene.md). See the
[hook manifest](../.pre-commit-hooks.yaml).

## Seed-skill propagation

qwen-opencode's per-repo context management wants a little language context living inside each target repo (for a Python repo, a pointer to how Kai writes Python). The composed `coding-<lang>` sources declare how they propagate with a `seed:` frontmatter block: `kind: always` (the `coding-git` baseline, seeded into every repo) or `kind: language` with `language` + `extensions` (seeded into repos containing those files). Target repos reference the delivered path, e.g. `.agents/skills/coding-python/SKILL.md`, or the canonical `.agents/composed/coding-python/COMPOSED.md` source.

The composed frontmatter is the source of truth. `generate-seed-skills` renders it into `agentic_os/seed_skills_data.py`, shipped in the package so consumer repos enforce the `seed-skills` hook offline, and `check-seed-skills-drift` (dogfooded in `agentic-os` only) fails if that table goes stale. This repo ships the validator half only: the actual copying and `COMPOSED.md` to `SKILL.md` promotion in target repos is Ansible's job.

## Diagnostic + utility helpers

Single-purpose validators for cryptic failure modes. These plus [`ward context-budget`](context-budget.md) are CLI/on-demand tools, not repo-content hooks, so they ship as ward verbs (agentic-os#233):

- `ward aws-config` - catches the `[profile default]` trap (SDKs read `[default]`; a misplaced region surfaces later as a useless `NoRegion`).
- `ward ssm-path` - checks parameter paths against the `/<org>/<repo>/<tier>/<tail>` schema before IAM/KMS, where a malformed path silently misses every tier policy.
- `ward exec prod-install-ref -- guard|ward|aos` - returns the immutable
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

agentic-os dogfoods local `uses:` refs. `promote.yml` advances `release`.
`aos-precommit-release.yml` tags installed-hook changes.
`dev-base-publish.yml` publishes affected images, while `release.yml` retries
retags. Consumer pins derive from `aos-precommit-v*`. See
[release.md](release.md).
