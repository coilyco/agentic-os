---
name: tooling-scout-autonomy
description: Build leg of the scout family - what novel software to BUILD on your existing toolset, ranked for a long autonomous run, biased to refactors and wide backfills. Triggers - autonomy scout, scout autonomy, what should I build.
---

# scout-autonomy

The **build** leg of the scout family. Its two siblings move tooling in and out:
`tooling-scout-capability` finds what to **add** (external catalog in), while
`tooling-scout-displacement` finds what to **shed** (your custom code out).
This leg answers the third question:
given the toolset you already have, **what novel software should you build on top of it?**
Add / shed / build. The output is a ranked, buildable backlog and a ready-to-run long-run
rule, sized to hand straight to an overnight autonomous session.

## Triggers

autonomy scout, scout autonomy, what should I build, build leg, plan a long run, overnight build plan.

## The bias that defines this leg

Novel does not mean net-new. The ranking is deliberately weighted so that:

- **Refactors and progressive improvements** of existing subsystems outrank greenfield.
- **Wide backfills** - author or extend one thing, roll it across N repos (a new pre-commit validator, a config migration, a doc-trifecta sweep) - rank highest, because reach is the dominant term.
- **Net-new** must clear a higher bar: it only ranks if it **composes** at least one tool you already maintain. Standalone greenfield is demoted by default.

This is leverage scouting, not idea generation. Detail in [references/scoring-and-config.md](references/scoring-and-config.md).

## Phases

Five phases, each checkpointing to a single-day notes/scratch file so a run resumes
without re-running phase 1. Inbox path: `<notes-dir>/YYYY-MM-DD-scout-autonomy-{phase}.md`.
Outputs go to notes/scratch, **not** the personal-OS repo - speculative projection
surfaces private intent, and only an actual build lands commits.

- [Phase 1 - Grounded sweep](references/phase-1-sweep.md) - walk the working surface, ingest sibling-scout and backlog outputs if present, list buildable candidates tagged refactor / backfill / net-new.
- [Phase 2 - Projection](references/phase-2-project.md) - project software that does not exist yet but composes the existing toolset. Composition is required, not optional.
- [Phase 3 - Leverage scoring](references/phase-3-score.md) - score value x reach / cost with the refactor + backfill multipliers, apply the reserved-surface fence, rank.
- [Phase 4 - Rule synthesis](references/phase-4-contract.md) - for the top picks, write a long-run rule (goal / done-condition / non-goals) ready for an overnight run.
- [Phase 5 - Present and hand off](references/phase-5-present.md) - flatten the top 3 to chat, explicit-deny approval, the chosen rule seeds a long-run session.

## Run config

Supplied at invoke, not hardcoded (this is a public-safe method): target repo list,
notes/scratch path, and a **reserved-surface fence** - subsystems this run must not touch
or propose work on, the human's way to keep a scout off reserved or wall code.
Schema in [references/scoring-and-config.md](references/scoring-and-config.md).

## PM handoff

PM owns the rule, then hands implementation to engineer as phase 5 details.

## Paired with

The scout family is one portfolio activity in three directions:

- **scout-capability** - catalog in, acquire what you lack.
- **scout-displacement** - inventory in, retire what OSS already does.
- **scout-autonomy** (this skill) - toolset in, build the high-leverage thing on top.

## See also

- [references/scoring-and-config.md](references/scoring-and-config.md) - scoring weights, run-config schema, cadence and resume.
- `tooling-issue-prioritization` - the backlog signal consumed during the
  grounded sweep.
- `tooling-scout-capability` / `tooling-scout-displacement` - the add / shed siblings.
