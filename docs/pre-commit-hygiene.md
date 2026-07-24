# Pre-commit hygiene

This repo ships the baseline cleanliness hooks plus a few staged opt-ins for
text hygiene that are too disruptive to flip on everywhere at once.

## Active hooks

- `trailing-whitespace`
- `end-of-file-fixer`
- `check-added-large-files` with a 2048 KB ceiling
- `check-merge-conflict`
- `check-case-conflict`
- `check-illegal-windows-names`
- `mixed-line-ending`
- `check-json`
- `check-toml`
- `actionlint` on `.forgejo/workflows/*.yml` and `.yaml`. `.github/actionlint.yaml` teaches it the Forgejo runner label `docker`
- `actions-run-one-line` rejects block, folded, escaped-newline, and physically split `run:` commands in GitHub and Forgejo workflows plus composite actions. A tracked script or `ward exec` verb owns the implementation while YAML invokes it from one line
- `forgejo-runner-validate` for Forgejo-native workflow and local-action semantics
- `shellcheck` on shell scripts
- `typos` with repo-specific words in [`.typos.toml`](../.typos.toml)

## Manual opt-ins

- `shfmt` - manual stage only. Shellcheck is the default gate because it is
  lower drama for the current shell style.
- `unresolved-placeholder-guard` - manual stage only. Use it once the repo has
  enough allowlists for examples and quoted snippets.
- `issue-reference-guard` - manual stage only. It skips fenced code, inline
  code, quoted command examples, and test fixtures, and it leaves external
  upstream issue links alone. Use it once the repo has a staged rollout plan
  and local allowlists for historical references.

## Opting in

Add the hook at `stages: [manual]` and supply repo-local config.

```toml
[tool.agentic-os.unresolved-placeholder-guard]
enabled = true
excludes = [
  "docs/feature-examples/**",
]
allow_globs = [
  "docs/quoted-examples.md",
]

[tool.agentic-os.issue-reference-guard]
enabled = true
excludes = [
  "docs/skill-discipline-handbook-hooks.md",
]
```

The guard is intended for durable prose breadcrumbs like `See #337 for the draft`,
not literal syntax examples or upstream issue links.

The manual-only hooks are intentionally excluded from the fleet coverage audit
until they are rolled out as active checks.
