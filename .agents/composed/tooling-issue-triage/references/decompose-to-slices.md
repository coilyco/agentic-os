# Decompose to tracer-bullet slices

Break a plan, a PRD, or an oversized issue into independently-grabbable tracker
issues using vertical slices. This is the tail of the triage loop: what survives
pruning and is too large to dispatch gets cut here.

Turning a broad outcome into an epic and a program of study, implementation, and
evidence lanes is a different job and lives in
`tooling-tpm-program-decomposition`.

## Size-check first

If this fired because a session opened with a bare issue reference, read the
issue through the repository's configured tracker surface and assess size before
doing anything else.

- Single, narrow acceptance criterion. Not oversized. Decline, hand control back, let the normal flow run.
- Multiple unrelated acceptance criteria, vague scope, or an estimate that touches many files across layers. Oversized. Say you flagged it and ask whether to split before executing.

The gate exists to avoid splitting issues that do not need it. Decomposition is
the right tool when an issue would otherwise burn a whole session on orientation
and mid-task compaction.

## Draft the slices

Each issue is a **tracer bullet**: a thin vertical slice cutting through all
integration layers end to end, never a horizontal slice of one layer.

- Each slice delivers a narrow but complete path through every layer (schema, API, UI, tests).
- A completed slice is demoable or verifiable on its own.
- Prefer many thin slices over few thick ones.

Tag each slice with its autonomy ceiling from
[automation-mode-axis](automation-mode-axis.md) rather than a private HITL/AFK
vocabulary, so a slice enters the same dispatch gate as everything else. Prefer
`autonomy/headless` where the work genuinely supports it, and fail closed to
`autonomy/async-consult` when unsure.

## Confirm the breakdown

Present the proposed slices as a numbered list, each showing title, autonomy
ceiling, what blocks it, and which user stories it covers if the source has
them. Then ask whether the granularity is right, whether the dependencies are
correct, whether anything should merge or split further, and whether the
autonomy ceilings are right.

**Ask this as a batched AskUserQuestion round rather than in prose**, or render
the slice list as a board when it runs long. The routing rule from the
entrypoint holds here: a recommendation that would be right most of the time
belongs on a board, not in a question. See
[triage-board-artifact](triage-board-artifact.md).

Iterate until the breakdown is approved.

## Create the issues

Create through the repository's configured tracker surface, in dependency order
(blockers first) so real issue numbers can go in the Blocked by field. coilyco
repositories use Forgejo. Follow `coding-core-git-workflow` for tracker
selection and write authority.

<issue-template>
## Parent

#<parent-issue-number> (if the source was an issue, otherwise omit this section)

## What to build

A concise description of this vertical slice. Describe the end-to-end behavior, not layer-by-layer implementation.

## Acceptance criteria

- [ ] Criterion 1
- [ ] Criterion 2
- [ ] Criterion 3

## Blocked by

- Blocked by #<issue-number> (if any)

Or "None - can start immediately" if no blockers.

</issue-template>

Do NOT close or modify any parent issue.

**Every slice you file lands in someone's backlog.** Decomposition is a filing
act, and it is one of the habits that produces the rate this skill exists to
absorb. Label each slice on all three axes as you create it, so the next triage
pass inherits a classified issue rather than another unlabeled row.
