# skill-discipline

Pre-commit hooks and authoring docs for documentation and skill repositories.

## Hooks

The validators live in the `agentic_os` Python package and ship through the managed `coilyco-flight-deck/agentic-os` pre-commit block.

- `check-skills` - validates `.agents/skills/` against a spec at `.agents/skills/categories.yaml`. Checks frontmatter, prefix taxonomy, status lines, required sections, size caps, stale skill-name references.
- `check-composed-skills` - validates role-scoped `.agents/composed/` sources, requires `COMPOSED.md`, and rejects discoverable entrypoints or ordinary-name collisions.
- `check-dead-links` - walks every markdown file in the repo, fails if any inline `[text](path.md)` link does not resolve or escapes the repo root.
- `check-source-doc-refs` - walks source comments, fails if a path-like documentation reference no longer resolves.
- `check-code-review-contract` - requires a root `CODE-REVIEW.md` that names repo-local invariants, historical issues, and refresh triggers instead of generic review advice.
- `check-documentation-layout` - keeps Markdown at repo root, flat `docs/*.md`, or skill folders only.
- `check-code-comments` - keeps standalone code comments to two contiguous lines max, 90 chars each. YAML is stricter: one comment line, first line only, so a key-sorter cannot drift it.

See [`skill-discipline-example-pre-commit-config.yaml`](skill-discipline-example-pre-commit-config.yaml) for the managed `.pre-commit-config.yaml` block.

## Docs

- [`skill-discipline-handbook.md`](skill-discipline-handbook.md) - the discipline these hooks enforce, with the why behind each rule.
- [`skill-discipline-authoring.md`](skill-discipline-authoring.md) - how to draft, validate, and ship a new skill.
- [`skill-discipline-example-categories.yaml`](skill-discipline-example-categories.yaml) - heavily commented spec to start from.
- [`skill-discipline-template-SKILL.md.template`](skill-discipline-template-SKILL.md.template) - minimal starter for a new skill.
