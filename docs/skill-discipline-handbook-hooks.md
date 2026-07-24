# Skill Discipline Handbook - The Hooks

## 10. The hooks

`.pre-commit-hooks.yaml` exposes the repo-authored discipline hooks. Wire each one explicitly in your repo's `.pre-commit-config.yaml`.

### check-skills (pre-commit, pre-push)

Runs `check-skills`. Checks frontmatter, prefix/exact match, status (where enforced), H1 pattern, required sections, forbidden body strings, stale skill-name backtick references, size caps. Symlinks under `.agents/skills/` are skipped, since their canonical target is validated where it lives.

### check-composed-skills (pre-commit, pre-push)

Runs `check-composed-skills`. Validates `.agents/composed/` against the ordinary
taxonomy, requires `COMPOSED.md`, and rejects discoverable entrypoints,
symlinks, invalid entries, and ordinary-name collisions.

### dead-cross-links (pre-commit, pre-push)

Runs `check-dead-links`. Walks every Markdown file in the repo (root `README.md`/`AGENTS.md`, `docs/`, co-located module READMEs, the skill tree), extracts inline `[text](target)` links, fails on any local-relative target that does not resolve to a real file. A link that resolves outside the repo root is a hard violation, not a skip - an internal `../` link is validated for existence like any other, while one that escapes the repo fails. External URLs, anchors, and placeholders (`...`, `TBD`, `TODO`) are skipped. Directory skipping mirrors `documentation-layout` (`.git`, `node_modules`, build/cache dirs); per-repo `excludes` live under `[tool.agentic-os.dead-cross-links]`.

### source-doc-refs (pre-commit)

Runs `check-source-doc-refs`. Validates documentation paths in tracked source
comments, including both skill entrypoint forms, root pointers, and bare doc
basenames. Per-repo excludes live under
`[tool.agentic-os.source-doc-refs]`.

### catalog-trifecta (pre-commit, pre-push)

Runs `check-catalog-trifecta`. Enforces the repo entrypoint set: `README.md`, `AGENTS.md`, `docs/FEATURES.md`, and one catalog YAML (`.ward/ward.yaml` or `.ward/ward.yaml`). Each Markdown file needs `## See also`, links to the other entrypoints, and the convention citation.

`AGENTS.md` also carries a required repo-local heading set so agents can scan operating rules without guessing each repo's prose shape:

* `## Scope`
* `## Project shape`
* `## Repo boundaries`
* `## Commands`
* `## Validation`
* `## Safety`
* `## Cross-repo contracts`
* `## Release`
* `## Agent rules`
* `## See also`

### code-review-contract (pre-commit)

Runs `check-code-review-contract`. Enforces a root `CODE-REVIEW.md` that documents repo-local invariants, historical issues, and refresh triggers. Generic-purpose review advice stays out so the contract remains about defending the repo's own failure modes.

### documentation-layout (pre-commit)

Runs `check-documentation-layout`. Enforces Markdown placement across the repo:

* root Markdown is limited to the universal allow-list (`README.md`, `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `CODE-REVIEW.md`, `GOVERNANCE.md`, `SECURITY.md`, `SUPPORT.md`, `LICENSE.md`);
* ordinary documentation lives in flat `docs/*.md`;
* skill documentation lives under `.agents/skills/`, `.agents/composed/`, `.claude/skills/`, or `skills/`;
* `docs/` has no subdirectories. Use filename prefixes when grouping is needed;
* every Markdown file stays under the cap in [`check_documentation_layout.py`](../agentic_os/pre_commit/check_documentation_layout.py). Entrypoints are not exempt.

### code-comments (pre-commit)

Runs `check-code-comments`. Enforces code-comment discipline for common source files: standalone comments are up to two contiguous lines, max 90 chars each. Longer durable explanation belongs in `docs/*.md`; code gets a short pointer only. YAML is stricter: a key-sorter would drift any lower comment off its target, so YAML allows just one comment line and only as the first line of the file.
