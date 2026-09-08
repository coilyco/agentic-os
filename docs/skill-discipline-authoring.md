# Authoring a skill, and the hooks that check it

## Authoring walkthrough and the hooks

How to draft a new skill, validate it, and ship it. Pairs with [`skill-discipline-handbook.md`](skill-discipline-handbook.md), which carries the structural rules this walkthrough assumes. The validate, commit, and iterate steps live in [`skill-discipline-authoring.md`](skill-discipline-authoring.md).

## Capture intent

Start by understanding what the skill should do. Often the current conversation already contains the workflow: an investigation that worked, a recurring debug pattern, a checklist that keeps coming back. Extract from history before writing: which tools were used, the sequence of steps, the corrections, the input and output formats.

Five questions to pin down before writing anything:

1. **What should this skill enable an agent to do?** One sentence.
2. **When should it trigger?** Concrete user phrasings, contexts, scheduled routines.
3. **What is the expected output?** A knowledge note, a tracker record, an in-session report, a terminal command, a file on disk.
4. **Which category does it fit?** Pick the family up front. If none fits, you are committing to update the spec before authoring.
5. **Does it touch live systems?** Writes, deletions, network calls with side effects. If yes, design the skill to default to read-only or dry-run, and require an explicit opt-in for destructive operations.

Don't draft until those five are settled.

## Interview and research

Ask the author about edge cases. What inputs are valid? What outputs are correct? What are the success criteria? What are the dependencies (other skills, MCP servers, external services)?

If similar skills exist in the repo, read them. There is almost always prior art worth cross-linking, and copying a successful skill's shape is faster than designing from scratch.

If the repo provides templates under (typically) a `templates/` directory, start from the closest one. Validator-enforced structure is documented in [`skill-discipline-handbook.md`](skill-discipline-handbook.md) sections 4 and 5.

## Draft the SKILL.md

Create the directory: `.agents/skills/<skill-name>/`. Add a `SKILL.md` with frontmatter and body.

Frontmatter rules (validator-enforced):

* `name` MUST equal the directory name.
* `description` MUST be non-empty. This is the primary triggering field. Include both what the skill does and concrete trigger phrasings. End with a packed `Triggers - foo, bar, baz.` line. Bias toward over-triggering since agent harnesses tend to under-invoke.

Status line: only if the category enforces it. Format is `Status: <emoji> <Kind> | Last <updated|tested>: YYYY-MM-DD`, directly under the H1. The emoji must pair with the kind exactly as declared in `categories.yaml`.

Required H2 sections: only if the category enforces them, dispatched by status kind. The validator names the missing section in its error output; that is usually enough to know what to add.

Body length: see [`check_documentation_layout.py`](../agentic_os/pre_commit/check_documentation_layout.py) for the cap. SKILL.md is not exempt. If you outgrow either limit, split into sibling `<skill>/<topic>.md` files linked from the SKILL.md, not inlined.

Follow your project's voice rules (the ones in [`skill-discipline-handbook.md`](skill-discipline-handbook.md) section 6 are a starting point if you have none yet). Bullets are usually clearer than dense prose. Imperative voice ("Run X. Check Y.") beats hedged voice ("you might consider running X"). Explain the why, not just the what.

Continues [`skill-discipline-authoring.md`](skill-discipline-authoring.md). Once the SKILL.md is drafted, validate it locally, commit it through the hooks, and iterate after it ships.
## Validate locally

Before staging:

```sh
pre-commit run skill-conventions --all-files
pre-commit run dead-cross-links --all-files
```

Both should exit 0. If they don't, fix the reported issues. The error messages name the line and the rule.

## Commit

Stage the new directory and commit. `skill-conventions` and `dead-cross-links`
re-run automatically. If either fails, fix the underlying issue and commit
again. **Do not use `--no-verify`.** The hooks are the discipline.

## Iterate

A skill that ships and gets used always reveals gaps. Common patterns after the first invocation:

* The trigger was too narrow and the agent did not pick the skill up. Pack more aliases into `description`.
* The body assumed context the agent lacked. Add it, or split it into `references/`.
* A new edge case appeared. Add it with the why and the fix.

The validator and the dead-link checker catch structural drift. Voice and usefulness drift you have to catch yourself, usually by re-reading a skill cold after a few weeks.

## The hooks

`.pre-commit-hooks.yaml` exposes the repo-authored discipline hooks. Wire each
one explicitly in your repo's `.pre-commit-config.yaml`.

**check-skills** checks frontmatter, prefix or exact match, status where
enforced, the H1 pattern, required sections, forbidden body strings, stale
skill-name backtick references, and size caps. Symlinks under
`.agents/skills/` are skipped, since their target is validated where it lives.

**check-composed-skills** validates `.agents/composed/` against the ordinary
taxonomy, requires `COMPOSED.md`, and rejects discoverable entrypoints,
symlinks, invalid entries, and ordinary-name collisions.

**dead-cross-links** walks every Markdown file, extracts inline `[text](target)`
links, and fails on any local target that does not resolve. A link escaping the
repo root is a hard violation rather than a skip, while an internal `../` link
is existence-checked like any other. External URLs, anchors, and placeholders
are skipped, and directory skipping mirrors `documentation-layout`.

**source-doc-refs** validates documentation paths in tracked source comments,
including both skill entrypoint forms, root pointers, and bare basenames.

**catalog-trifecta** enforces the entrypoint set of `README.md`, `AGENTS.md`,
`docs/FEATURES.md`, and `.ward/ward.yaml`, each Markdown file carrying
`## See also` and linking the others. `AGENTS.md` also carries a required
repo-local heading set, so agents can scan operating rules without guessing
each repo's prose shape.

**code-review-contract** enforces a root `CODE-REVIEW.md` documenting
repo-local invariants, historical issues, and refresh triggers, keeping generic
review advice out so the contract stays about this repo's failure modes.

**documentation-layout** limits root Markdown to a universal allow-list, keeps
ordinary docs in flat `docs/*.md` with no subdirectories, keeps skill docs under
their skill roots, and holds every file under the band caps. Entrypoints are not
exempt.

**code-comments** keeps standalone comments to two contiguous lines of at most
90 chars, with a top-of-file header block exempt. Longer explanation belongs in
`docs/*.md` with a short pointer in the code. YAML is stricter, allowing
comments only in that header block, because a key-sorter would drift any lower
comment off its target. Two dials change both: `header_cap` caps the header,
and `yaml_comments_below_content` lifts it where no `yaml-strict` sorter runs.

Per-repo excludes and dials live under `[tool.agentic-os.<hook>]` in
`pyproject.toml`.
