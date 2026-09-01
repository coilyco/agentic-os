---
name: tooling-issue-triage
description: Ground-level issue triage as one loop - scope and count the pool, tier it on three axes, publish a triage-board artifact for breadth, drain the blocking forks through batched AskUserQuestion rounds, write answers back, then decompose what survives into tracer-bullet slices. Triggers - triage, prioritize, backlog burn-down, priority/P0-P3, autonomy, role labels, consult queue, ask the human, triage board, decompose, make tickets, issue counts.
---

# Issue triage

Everything between "an issue exists" and "an agent can run it or a human has
answered it". Portfolio-level sequencing is a different job and lives in
`tooling-tpm-*`.

**The constraint this skill is shaped around is throughput, not correctness.**
Agents file faster than a human reads. A deferral owes its issue, a mining or
eval session files four to seven findings at once, and an epic cut into slices
lands a dozen in an afternoon. A triage method that costs the human one decision
per issue loses to that rate by construction, no matter how good each decision
is. So the loop below spends human attention in exactly two places and refuses
to spend it anywhere else.

## The two surfaces, and which one a decision goes to

**An artifact carries breadth. AskUserQuestion carries depth.** They are not two
renderings of the same thing, and picking wrong is the main way a triage pass
wastes the human.

- **Triage board (artifact).** Many rows, cheap per row. Every open issue in the
  pool gets a line with its gist, the evidence read, and a recommended
  disposition already selected. The human skims, overrides the few that are
  wrong, and hands back the deltas. Cost per issue approaches zero, which is the
  only property that keeps pace with the filing rate. Spec:
  [triage-board-artifact](references/triage-board-artifact.md).
- **AskUserQuestion.** Up to four genuine forks per round, each with its options
  closed and a recommendation first. Use it for a call that blocks work and that
  a recommendation cannot make on the human's behalf. Loop, option design, and
  failure modes: [askuserquestion-flow](references/askuserquestion-flow.md).
- **Consult queue (artifact).** The overflow for depth. When a fork needs more
  context than four option labels can hold, or when more than four are ready at
  once, render them as a page instead and let the human answer asynchronously.
  Spec: [consult-queue-artifact](references/consult-queue-artifact.md).

The routing rule: **if a recommendation would be right most of the time, it
belongs on the board, not in a question.** A question you would answer the same
way yourself is theatre, and it costs a round you do not get back.

## The loop

1. **Resolve the pool and count it.** One repository by default, or a declared
   portfolio. Get the authoritative open count before fetching anything, because
   the count is the coverage gate for everything after it. See
   [coverage-and-counts](references/coverage-and-counts.md) and, for a fleet
   pass, [global-forgejo-scope](references/global-forgejo-scope.md).
2. **Read, including the comments.** Fan discovery out one repository per worker,
   each carrying its exact N. The thread is where a design fork gets named and
   where a seat releases a claim, so a pass over titles and bodies alone
   over-promotes badly.
3. **Classify on three axes.** `priority/*` for urgency, `autonomy/*` for the
   agent-autonomy ceiling, `role/*` for whose queue it is. Plus readiness, which
   is not a label group. See [label-taxonomy](references/label-taxonomy.md),
   [target-shape](references/target-shape.md),
   [assignment-method](references/assignment-method.md),
   [automation-mode-axis](references/automation-mode-axis.md),
   [readiness-axis](references/readiness-axis.md).
4. **Publish the board.** One artifact, every issue in the pool, disposition
   pre-recommended. This is the deliverable of the read, and it replaces
   reporting the triage in prose.
5. **Ask the forks.** Batch them, cluster by decision rather than by issue, and
   never ask what the issue already answers.
6. **Write back from the parent.** Labels, comments, closes. Answers land on the
   tracker in the same turn they arrive. Mutations run in the parent, never in a
   fan-out worker. See [pruning-and-api](references/pruning-and-api.md).
7. **Decompose what survives.** An issue too large to dispatch becomes
   independently-grabbable tracer-bullet slices. See
   [decompose-to-slices](references/decompose-to-slices.md).

## Keeping up with the filing rate

Four rules, each earned against a real pass.

- **Triage the delta, not the backlog.** A full sweep is a periodic event, not
  the loop. Between sweeps, the pool is what was filed since the last pass. A
  board covering eleven days of filing is readable, and one covering 1104 open
  issues is not.
- **Audit the label before answering the question.** On a sampled director
  bucket, 4 of 7 issues carrying `autonomy/async-consult` were not consults at
  all: two were read-and-report tasks that belonged on `autonomy/headless`, one
  was a settled record, one was blocked behind agent prep that had not run. The
  queue never burned down because most of it was never waiting on a human.
  **Reclassifying is a higher-value move than answering, so make it the first
  row on every board.**
- **Read the shape before the rows.** A filing spike is usually a burst rather
  than a leak, and saying which one it is changes what the human does about it.
  Count filed against closed per day and put the number at the top of the board.
- **Duplication concentrates by origin.** Findings from one mining session
  routinely describe one failure from four angles. Cluster by origin, then fold,
  before you rank anything.

## Pruning

**Keep it or close it. There is no soft-prune tier**, since `priority/P4` was
deleted on 2026-09-01 for never behaving like the icebox it was named for. Keep
is the safe default and `priority/P3` is where unsure lands. Closing is the
prune, and reopen is its exact inverse, so it is not the hard call it sounds
like. See [pruning-and-api](references/pruning-and-api.md).

## The enforcing half

The autonomy labels are matched by string at the dispatch chokepoint, so
renaming one is a breaking change that fails silently. See
[dispatch-gate](references/dispatch-gate.md).
