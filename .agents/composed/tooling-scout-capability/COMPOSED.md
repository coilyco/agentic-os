---
name: tooling-scout-capability
description: Inbound half of the scout pair. Capability gap analysis for skills and MCP servers - what to ADD. Sweeps repos, uses the SkillsMP and Glama MCPs as primary catalogs, audits silver/gold tiers, and installs one-issue-one-commit. Paired with tooling-scout-displacement (what to SHED).
---

# scout-capability

The **inbound** half of the scout pair. This skill answers "what capability is
missing that I should acquire?" - external catalog in, matched to your repos.
Its sibling `tooling-scout-displacement` runs the opposite direction: your
custom code in, matched to off-the-shelf OSS,
answering "what custom thing should I retire because something already does it?"
Same scouting instinct, inverted flow. See **Paired with** below.

## Triggers

capability scout, scout capability, find me skills, find me mcps, gap analysis, missing capabilities.

Six-phase routine. Each phase runs independently and checkpoints to a
task-scoped scratch directory, so the human can run "scout-capability phase 3"
from the train and resume without re-running phase 1.

**Why six phases:** the full pipeline is too large for one model run, and
each phase has a different cost/risk profile. Phase 1 is open-ended
ideation, phase 4 is a security gate, phase 6 mutates the human's personal-OS repo. Mixing
them into one invocation either blows context or makes the security gate
easier to skip. Splitting forces an explicit checkpoint between
"speculate" and "install."

**Outputs go to the notes/scratch location, not the personal-OS repo.** Speculative discovery
incidentally surfaces private repo intent and personal context. Only the
final per-install commits land in the personal-OS repo. Inbox path:
`<notes-dir>/YYYY-MM-DD-scout-capability-{phase}.md` (parameterize to the human's scratch/notes location).

## Phases

- [Phase 1 - Grounded sweep + speculative ideation](references/phase-1-sweep.md) - walk the working surface, build a candidate list, brainstorm what doesn't exist yet.
- [Phase 2 - Hydration](references/phase-2-hydration.md) - query the SkillsMP and Glama MCPs first, verify against first-party and curated backstops, dedup against installs.
- [Phase 3 - Categorize and rank](references/phase-3-rank.md) - semantic categories, global 3:2:1 medal ranking.
- [Phase 4 - Security audit](references/phase-4-audit.md) - 🥈/🥇 only, supply-chain-audit skill, 🟢🟡🔴 safety prefix (blocked on execution verification).
- [Phase 5 - Present 🥇🟢 inline](references/phase-5-present.md) - flatten the top tier to chat, explicit-deny approval.
- [Phase 6 - Install approved entries](references/phase-6-install.md) - one issue + one commit + one push per entry, defense-in-depth re-check.

## PM handoff wall

PM owns phases 1 through 3 and phase 5. PM hands phase 4 to engineer or ops
for supply-chain verification, then hands approved phase 6 work to engineer
for placement, implementation, validation, and landing. PM records the returned
verdicts and outcomes without impersonating either execution role.

## Paired with

`tooling-scout-displacement` is the **outbound** half. The two scouts are one
portfolio-management activity run in opposite directions:

- **scout-capability** (this skill) - catalog in, acquire. Finds capability you lack and should add. The input is the world's skills/MCPs.
- **scout-displacement** - inventory in, retire. Finds custom code you maintain that off-the-shelf OSS already does, and proposes shedding it. The input is your own subsystems.

Run scout-capability when the tooling surface feels too small. Run scout-displacement when it feels too big or too custom. They share the back half (research-a-candidate, security-audit, one-issue-one-commit landing) and differ only in what feeds phase 1.

## See also

- [Scrub-on-reject and notes](references/scrub-and-notes.md) - reject ordering, run cadence, resume model, ward wrapper paths, why speculative entries matter.
