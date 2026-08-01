---
name: tooling-issue-prioritization
description: Tier and prune an issue backlog - P0-P4 definitions, target ranges, percentile-cut assignment, and an automation-mode axis (headless/interactive/consult). Triggers - prioritize, triage the backlog, P0/P1/P2/P3/P4, backlog ratio, icebox, burn down the backlog, tier the issues, automation mode, eligibility to dispatch.
low-context: required
---

# Issue Prioritization

How to put a priority on every open issue and keep the backlog honest. Trackers without a native priority field express priority as per-issue labels `P0`/`P1`/`P2`/`P3`/`P4` (exactly one per open issue) - agents apply them, humans sort and read. Define the labels once at org scope so every repo shares one set.

Resolve ranking scope before labels. See
[pool rules](references/priority-pool.md).

## Tier definitions

- **P0** - urgent AND blocking now. Active breakage/outage, security holes, data-loss risk, or blocks other committed work. Assigned by **a content-rule net, then a judgment confirm** (see Target shape), never a quota - whatever genuinely matches is P0.
- **P1** - important, the clear next thing once P0s clear. Concrete, committed-direction, near-term value.
- **P2** - backlog you genuinely intend to act on, just not yet. A real near-to-mid-term path to done.
- **P3** - the DEFAULT tier. Low but kept; also where unsure, unscored, or freshly-filed issues land. Requires no positive evidence - it is the fallback.
- **P4** - icebox, the lowest tier. The demotion sink: parked / speculative-but-kept / won't-do-soon (hobby toys, "fork X" / "try Z" wishes, reading-list adds, vague vision, far-future plays, one-line stubs). Unlike P3, P4 needs positive "parked" judgment - the async triage loop pushes here rather than closing.

## Target shape

**P0 has no quota - it is content-based, in two steps: net then confirm.**

1. **Net (recall, deterministic):** a script scans each issue's title+body for P0 signals - secret/token leak, arbitrary code execution or auth bypass, data loss, active outage/crashloop, broken deploy pipeline, "blocks committed work". The exact patterns live in [references/p0-content-rules.yaml](references/p0-content-rules.yaml). This casts a wide net.
2. **Confirm (precision, judgment):** keyword rules over-match badly (~40% of hits are *about* a topic, not incidents *of* it). So confirm each candidate with a one-line judgment call: **"active incident / live exposure, or just discussing it?"** Keep only the active ones - a bounded per-candidate decision a small local model can own.

You never force a P0 percentage - urgent is whatever genuinely is (a re-triage of ~750 issues confirmed ~19).

The **non-P0 remainder** splits to a global distribution across the resolved
priority pool, as equal-width ranges so the cut lands on a natural break, not a
forced percentage: **P1 0-20%, P2 10-30%, P3 20-40%, P4 30-50%** - same
20-point width, centers at **10 / 20 / 30 / 40**, summing to 100. Treat the
band, not a single number, as the target. P1 floors at zero on purpose: a
backlog with nothing important-and-near-term has an empty P1, and that is
correct. Small or urgent repositories may deviate past a band edge. The shape
holds on the resolved pool.

## Second axis: automation mode

Tier ranks urgency, not "can an agent land it unattended?" - independent questions. A second orthogonal axis labels each issue `headless` / `interactive` / `consult`, the agent-autonomy ceiling it is cleared for; unlabeled and unsure both fail-closed to `consult`. See [automation-mode-axis](references/automation-mode-axis.md).

## Assignment, pruning, and running it over an API

- [Assignment method](references/assignment-method.md) - the part that actually works: carve P0 by rule, score the rest by judgment, enforce the shape by percentile cut.
- [Pruning and running it over an API](references/pruning-and-api.md) - demote/merge/close decisions plus the hard-won lessons for driving triage over an issue tracker API.
