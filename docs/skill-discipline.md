# skill-discipline

Pre-commit hooks and authoring docs for documentation and skill repositories.

## Hooks

The validators ship in the `aos-precommit` distribution, preserve the
`agentic_os` Python namespace, and install through the managed
`coilyco-flight-deck/agentic-os` pre-commit block.

- `check-skills` - validates `.agents/skills/` against a spec at `.agents/skills/categories.yaml`. Checks frontmatter, prefix taxonomy, status lines, required sections, size caps, stale skill-name references.
- `check-composed-skills` - validates role-scoped `.agents/composed/` sources, requires `COMPOSED.md`, and rejects discoverable entrypoints or ordinary-name collisions.
- `check-dead-links` - walks every markdown file in the repo, fails if any inline `[text](path.md)` link does not resolve or escapes the repo root.
- `check-source-doc-refs` - walks source comments, fails if a path-like documentation reference no longer resolves.
- `check-code-review-contract` - requires a root `CODE-REVIEW.md` that names repo-local invariants, historical issues, and refresh triggers instead of generic review advice.
- `check-documentation-layout` - keeps Markdown at repo root, flat `docs/*.md`, or skill folders only.
- `check-code-comments` - keeps standalone code comments to two contiguous lines max, 90 chars each, with a top-of-file header block exempt from the line limit. YAML is stricter: comments only in that top block, above the first content line, so a key-sorter cannot drift one off its target.

See [`skill-discipline-example-pre-commit-config.yaml`](skill-discipline-example-pre-commit-config.yaml) for the managed `.pre-commit-config.yaml` block.

## Docs

- [`skill-discipline-handbook.md`](skill-discipline-handbook.md) - the discipline these hooks enforce, with the why behind each rule.
- [`skill-discipline-authoring.md`](skill-discipline-authoring.md) - how to draft, validate, and ship a new skill.
- [`skill-discipline-example-categories.yaml`](skill-discipline-example-categories.yaml) - heavily commented spec to start from.
- [`skill-discipline-template-SKILL.md.template`](skill-discipline-template-SKILL.md.template) - minimal starter for a new skill.

## Skill Discipline Handbook - Conventions

Sections 6 to 9 of the [handbook](skill-discipline-handbook.md): how to
write a skill once its shape is settled.

## 6. Voice rules (project standard, honor-system)

These are the writing conventions the rest of this handbook follows. The validator does not enforce them. Adopt them in your project's handbook if they fit; drop or replace if your project has different voice.

* **No italics.** Use bold for structural anchors at the start of bullets, or for terms of art on first mention. Italics for emphasis tends to read as performative.
* **No semicolons in prose.** Split into two sentences. Code is fine.
* **No prose tables.** Use flat bullets: `* anchor - category - detail`. Tables are correct when the structure is genuinely tabular (a machine-readable spec, a matrix of cases). They are wrong when you reach for one because the prose is getting hard to read - fix the prose instead.
* **Imperative voice in procedure.** "Run X. Check Y." beats "you should run X and then you might want to check Y."
* **Explain the why, not just the what.** Every rule should carry the reason it exists. Future readers need the why to judge edge cases. See section 9.

## 7. Size caps

Three size caps in `categories.yaml`, all with built-in defaults that apply when unset. Set any value to `0` to disable that specific check.

* `max_skill_md_lines` (default `500`) and `max_skill_md_bytes` (default `10000`) cap the SKILL.md file itself. Past either, agent harnesses degrade: the loader either refuses the file or drops it from context. Push detail into `<skill>/references/<topic>.md` files when a SKILL.md fills up. Reference files are not capped.
* `max_description_bytes` (default `500`) caps the frontmatter `description` field. Every skill's description is loaded into every agent session for keyword matching, so descriptions are pure always-on context cost. 500 fits a canonical-name + one sentence of trigger phrasings; past that you're paying for padding.
  * **Router/meta exception**: skills whose category declares `role: router` or `role: meta` get **2x** the cap (default 1000). Routers genuinely need wider keyword surface to fan out to all the skills they cross-link. The validator applies the multiplier automatically.

### Description budget targets

The validator enforces a hard byte cap, but the practical target should be lower. Treat `description` as routing metadata, not documentation. Codex and Claude both see this text before deciding whether to open the skill body.

* **Pointer skills** - under 160 chars. Use this when the body mostly points at a canonical doc elsewhere.
* **Normal task skills** - 120-200 chars. Most coding, writing, gaming, and tool-usage skills should fit here.
* **Complex task skills** - 220-300 chars. Use this for skills with several adjacent trigger phrasings or one important boundary.
* **Router/meta skills** - 250-400 chars normally. A router earns extra surface only when it prevents many child skills from carrying broad aliases.
* **Rare exceptions** - 400-500 chars for public-safety, MCP routing, or cross-repo failure handling where false negatives are expensive.

Put the main task shape, a few trigger phrases, and critical disambiguators in `description`. Put procedure, examples, policy rationale, command sequences, implementation details, and historical context in the body or `references/`.

If a skill needs more than 8-12 aliases in the description, rename the skill, add a router parent, or split the domain.

### Frozen-archive exemption

`archive_path_components` (default `[]`) lists path components that mark a frozen archive. Any `.md` whose path contains one of these is skipped from the size caps - but only the size caps. Stale-ref, forbidden-body-string, and frontmatter checks still apply.

The motivation: investigation writeups, per-incident case libraries, and ticket-stamped diagnoses are loaded by name when an agent revisits the incident, not by the loader on trigger. Forcing splits on a 28KB rollout analysis destroys narrative for zero loader benefit. The convention this enables: park frozen content under a directory whose name is in the list (`results/`, `archive/`, etc.) and the cap stops applying.

```yaml
archive_path_components:
  - results
```

## 8. Cross-links

Two valid forms for in-prose references to other skills:

* **Bare backticks** `` `skill-name` `` for passing mentions in prose. Not navigable.
* **Markdown link** `` [`skill-name`](../skill-name/SKILL.md) `` for navigable references.

Either form: if the name does not resolve to a real skill in the repo, `check_dead_links.py` flags it. A cross-repo reference (a skill or file living in a sibling repo) cannot be a navigable link - it would escape the repo root, which is now a hard violation - so use the bare-backtick form with a parenthetical, e.g. `` `coilyco-aws-auth` (in agentic-os-kai) ``.

External URLs, mailto links, and bare anchors (`#section`) are out of scope for the dead-link check. A `../` link is no longer skipped: an internal one is existence-checked, and one that escapes the repo root fails.

## 9. Encode the why, not just the what

Every agent session starts cold. There is no human in the loop to ask "why was this rule written?" Undocumented reasoning gets re-derived badly, or the rule gets deleted by an agent who cannot see why it mattered.

When you write a rule, lead with the rule, then write a **Why:** line (the incident, constraint, or prior failure mode that produced it), then a **How to apply:** line (when the rule fires). Date-stamp the flag where useful so future readers can judge whether the why is still load-bearing.

Framing reference: [The end of "just ask Sarah"](https://simme.dev/posts/the-end-of-just-ask-sarah/).
