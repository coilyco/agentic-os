# Authoring walkthrough

How to draft a new skill, validate it, and ship it. Pairs with [`skill-discipline-handbook.md`](skill-discipline-handbook.md), which carries the structural rules this walkthrough assumes. The validate, commit, and iterate steps live in [`skill-discipline-authoring.md`](skill-discipline-authoring.md).

## Capture intent

Start by understanding what the skill should do. Often the current conversation already contains the workflow: an investigation that worked, a recurring debug pattern, a checklist that keeps coming back. Extract from history before writing: which tools were used, the sequence of steps, the corrections, the input and output formats.

Five questions to pin down before writing anything:

1. **What should this skill enable an agent to do?** One sentence.
2. **When should it trigger?** Concrete user phrasings, contexts, scheduled routines.
3. **What is the expected output?** A knowledge note, a Forgejo issue, an in-session report, a terminal command, a file on disk.
4. **Which category does it fit?** Pick the prefix or exact-name family up front. If none fits, you are committing to update the spec first, before authoring.
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

## Voice

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

Stage the new directory and commit. The pre-commit hooks run automatically:

* `skill-conventions` re-runs the validator.
* `dead-cross-links` re-runs the cross-link checker.

If any hook fails, fix the underlying issue and commit again. **Do not use `--no-verify`.** The hooks are the discipline; bypassing them defeats the point.

## Iterate

A skill that ships and gets used always reveals gaps. Common patterns after the first invocation:

* The trigger description was too narrow: the agent did not pick the skill up when it should have. Pack more aliases into `description`.
* The body assumed context the agent did not have. Add the missing assumption to a body section, or split it into a `references/` file.
* A new edge case appeared. Add it to the relevant section with the why and the fix.

The validator and the dead-link checker catch structural drift. Voice and usefulness drift you have to catch yourself, usually by re-reading a skill cold after a few weeks.
