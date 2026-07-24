# Handbook: Frontmatter and description budgets

## 3. Entrypoint frontmatter (universal)

Every `SKILL.md` and `COMPOSED.md` begins with YAML frontmatter:

```markdown
---
name: <directory-name>
description: Use when <routing condition>. Include the canonical task shape plus a compact alias set. Keep discovery text short; put procedure in the body.
---
```

Rules:

* `name` MUST equal the directory name. Validator enforces this.
* `description` MUST be non-empty. Validator enforces this.
* `description` is routing metadata, not documentation. Claude and Codex both see it before reading the skill body. The field answers "when should the agent open this skill?"
* `description` may include a compact `Triggers - foo, bar, baz` tail when trigger aliases help. The tail is optional, and it should earn its bytes. Avoid exhaustive keyword bags.
* `SKILL.md` is the ordinary discoverable entrypoint. `COMPOSED.md` is the
  source entrypoint that agent-compose renames after role selection.
* Cross-links to other skills use either:
  * bare backticks `` `skill-name` `` for in-prose passing mentions, or
  * markdown link `` [`skill-name`](../skill-name/SKILL.md) `` for navigable references.
  Either form is fine; both are validated. The dead-link checker resolves the markdown target.

### Description budgets

Descriptions are the highest-cost text in the skill system because every candidate skill pays the cost before an agent decides what to open. Claude's larger context can tolerate chatty descriptions, but Codex routing benefits from shorter, sharper metadata. Optimize the eager surface first; leave the skill body rich.

Hard validator limits:

* **Normal skills** - 500 by default. Override to 200 for a Codex-optimized catalog.
* **Router/meta skills** - 2x the cap when the matched category declares `role: router` or `role: meta`. At the default cap, that means 1000 bytes.
* **Entrypoint bodies** - cap in [`check_documentation_layout.py`](../../../../agentic_os/pre_commit/check_documentation_layout.py), same as all Markdown. Neither entrypoint is exempt. Move detail into a sibling `<topic>.md`, `scripts/`, or `assets/`.

Target bands, what belongs in `description` vs the body, alias discipline, description templates, and the 2026-05-21 audit baseline live in [`handbook-description-budgets.md`](handbook-description-budgets.md).
