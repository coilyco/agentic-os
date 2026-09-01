---
name: tooling-code-review
description: Code review for ward's in-container review agent. Use when asked to review a filesystem diff, do adversarial review, same-family review, non-iterative review, or a ward review gate. Triggers - code review, review this diff, diff review, adversarial review, same-family review, non-iterative review, filesystem diff review.
---

# Code Review

Review the current filesystem change set once, against the issue rule and the intended baseline implementation.

## Review stance

- **Prioritize** correctness bugs, behavioral regressions, missing tests, security and secret risks, operational risk, config or deploy mistakes, and mismatch against the issue rule.
- **Compare to the intended baseline**, not to a hypothetical rewrite. The question is whether this diff lands the issue cleanly.
- **Assume the reviewer may share a model family with the worker.** Force a refutation pass even when the patch looks good.

## Inputs

Read, in this order when available:

1. The issue rule or run seed.
2. `git status`.
3. `git diff --staged` and `git diff`.
4. Touched tests and docs.
5. Relevant local conventions, nearby code, and any repo-specific guidance needed to judge the change.

## Non-iterative rule

- Do not edit files.
- Do not ask the worker to patch anything.
- Do not enter a back-and-forth.
- Do not re-run the review as a negotiation loop.

If a required input is missing or the diff cannot be reviewed safely, block once with that reason.

## How to review

- Start from the issue rule and ask what must be true for the change to count as done.
- Read the diff as evidence, not as intent.
- Try to break the patch first. Look for the cheapest plausible failure mode, not the nicest interpretation.
- Check whether the tests cover the new behavior and the regression surface.
- Check whether docs, configs, migrations, deploy knobs, or operational expectations changed in lockstep.
- Prefer concrete findings over vibes. If nothing breaks, say why the patch still survives refutation.

## Output rule

Return one compact result in the format in [`references/output-format.md`](references/output-format.md).

## Conclusion-ready summary

Include a short paragraph the worker can lift into its final `WARD-OUTCOME` comment. It should say what landed, whether the review passed or blocked, and the most important reason.
