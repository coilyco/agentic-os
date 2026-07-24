# Skills Repository Handbook

**Purpose.** Single source of truth for everything uniform and structured about
a provider repo's ordinary and role-composed skill sources. Read the linked
sections cold and you can recreate the taxonomy, entrypoint shapes, validators,
templates, pre-commit wiring, cross-link rules, and category process.

This handbook is paired with [`categories.yaml`](../../categories.yaml) (at `.agents/skills/categories.yaml`), the machine-readable spec consumed by the `coilyco-flight-deck/agentic-os` skill-discipline validator. When the two disagree, the YAML is authoritative for the validator and these files should be updated to match.

The handbook is split into topical references so each stays under the documentation-layout caps. Read the one you need:

* [Layout](handbook-layout.md) - §1 ordinary and composed source trees. Routes to [categories](handbook-categories.md) for §2.
* [Frontmatter](handbook-frontmatter.md) - §3 universal entrypoint frontmatter and hard validator limits. Routes to [description budgets](handbook-description-budgets.md) for target bands, alias discipline, templates, and the audit baseline.
* [Status lines and required sections](handbook-sections.md) - §4 the status line and its kind/emoji pairings, §5 required H2 sections per category plus the repo AGENTS.md heading set.
* [Validators and pre-commit wiring](handbook-validators.md) - §6 documentation-wide validators, `skill-conventions`, `dead-cross-links`, `em-dash-check`, pre-commit wiring.
* [Templates, cross-linking, categories, voice, symlinks](handbook-conventions.md) - §7 templates, §8 cross-linking rules, §9 adding a new category, §10 voice conventions, §11 symlinks and the global skill surface.
