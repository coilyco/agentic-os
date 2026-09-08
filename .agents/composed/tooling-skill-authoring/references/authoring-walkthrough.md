# Authoring walkthrough: capture intent, draft, validate

This is the operational walkthrough for drafting a new skill. The personal-OS-specific rules in the parent SKILL.md still apply.

## Capture intent

Start by understanding what the human wants. The current conversation often already contains the workflow (an investigation that worked, a recurring debug pattern, a checklist the user keeps redoing). Extract from history first: tools used, sequence of steps, corrections, input/output formats observed. Confirm with the user before drafting.

Run the [admission test](admission.md) and
[placement decision](placement.md) before drafting. Stop when the proposed
content belongs in person context, `AGENTS.md`, docs, retrieval, tooling,
validation, or authority configuration instead.

Five questions to pin down before writing:

1. What should this skill enable the target agent to do?
2. When should this skill trigger? (user phrasings, contexts, scheduled routines)
3. What's the expected output format? (tracker record, in-session report, terminal command, files on disk)
4. Which prefix fits (see `categories.yaml` for the canonical list)? Or do we need a new category (require justification)?
5. Will it touch live systems (kubectl, AWS writes, Trello, Discord, Bluesky)? If yes, route through `ward` and design test prompts that stay read-only or dry-run by default.

## Interview and research

Ask about edge cases, input/output, example artifacts, success criteria, dependencies. Don't write test prompts until this part is settled.

If research helps (existing similar skills, related docs, prior investigations), use the Agent tool with `subagent_type=Explore` for read-only codebase exploration. Reference existing skills before authoring a new one. There's almost always prior art worth cross-linking.

## Write the SKILL.md

Start by copying the appropriate template from [`../templates/`](../templates/) when one exists. For free-form categories (most of them), draft from scratch using a similar existing skill as a model.

Frontmatter rules (validator-enforced):

* **name** must equal the directory name.
* **description** non-empty, the primary triggering field. Include what the
  skill does and concrete trigger conditions. Use a compact `Triggers - ...`
  tail only when aliases earn their bytes. Test both positive and near-miss
  prompts for accurate activation.

Status line (where enforced - currently `ops-investigation-*` and `ops-investigation`): directly under the H1, format `Status: <emoji> <Kind> | Last <updated|tested>: YYYY-MM-DD`. See handbook §4.

Required H2 sections per category: see handbook §5.

Body length: hard cap in [`check_documentation_layout.py`](../../../../agentic_os/pre_commit/check_documentation_layout.py). SKILL.md is not exempt. If it's growing past that, split into sibling reference files alongside SKILL.md and link from the SKILL.md.

## Evaluate behavior and triggering

After drafting, generate realistic should-trigger prompts and adjacent
should-not-trigger near misses from the description. Follow the
[behavioral evaluation loop](evaluation.md) to preserve the before description,
cap any model calls, compare matched with-skill and without-skill task runs when
the claimed delta is behavioral, and grade objective assertions in fresh
context. Record false triggers, missed triggers, task regressions, and the
before-and-after description.

Model execution is optional. The prompt set and deterministic structural gate
are not.

## Structure, style, validate, wrap-up

Anatomy of a skill, progressive-disclosure tiers, writing style, validation commands, and the wrap-up checklist live in [`authoring-walkthrough-finish.md`](authoring-walkthrough-finish.md).
