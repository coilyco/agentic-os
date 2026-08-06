# Handbook: Templates, cross-linking, categories, voice, symlinks

## 7. Templates

One file per shaped category at [`../templates/`](../templates/):

* `ops-investigation.md` - investigation guide (Active)
* `ops-investigation-router.md` - router

Other categories are free-form and don't need templates.

To author a new shaped skill: copy the template, fill in. The validator catches anything you forgot.

## 8. Cross-linking rules

* Use **skill names**, not relative paths, in prose mentions. The router resolves.
* An `ops-investigation-*` skill that uses an external tool heavily
  cross-links to the relevant adapter or sibling skill when the link pulls
  weight.
* The `ops-investigation` router keeps its routing table current. Adding a new `ops-investigation-*` skill = update the router in the same commit.
* `<ops-investigation-meta>` is the meta-discipline layer. It cross-links to `ops-investigation` (the router) and to specific subject skills for examples.

## 9. Rules for adding a new category

You almost never should. Most asks fit one of the eleven families. If you genuinely have a new shape:

1. Open a draft of `categories.yaml` with the new category. Pick `kind: prefix` or `kind: exact`. Decide whether to enforce status and sections.
2. Update this handbook (§2 list, plus a per-category subsection in §5 if sections are enforced).
3. Add a template under `skill-creator/templates/` if the category has a fixed shape.
4. Run the validator to make sure it now passes against the proposed addition.
5. Then create the first skill of the new category.

The validator rejects unknown prefixes by design. It forces this discussion to happen.

## 10. Voice + writing conventions

Inherited from the user's AGENTS.md. Highlights:

* No em-dashes (U+2014). Use periods, commas, parens, or ` - `.
* No italics, no semicolons in prose.
* No tables in prose. Use flat bullet lists.
* "Load-bearing" is physical-only.
* See the user's voice guide for pronoun rules.
* Match the user's intent, not literal trigger keywords. Trigger lists are examples, not exhaustive.
* Imperative voice. Explain why over MUSTs.

The validator's em-dash check flags U+2014 in SKILL.md prose. Wrap legitimate uses (e.g. quoted prose from someone else) in backticks or double quotes.

## 11. Symlinks and the global skill surface

Top-level directories under `.agents/skills/` are canonical. Harness-specific setup owns their projection. The fleet refresh updates Claude's repo-local `.claude/skills/<name>` links and catalog aggregation. Agent-compose projects eligible catalogs into Codex's portable `~/.agents/skills/<name>` user surface. Restart your harness after refreshing so the loader picks up new entries.

Some skills (e.g. `ward-passthroughs`) live as symlinks inside `.agents/skills/` rather than real directories. The validator skips symlinks; the canonical target is validated where it lives.
