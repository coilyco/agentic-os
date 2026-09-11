# Opinionated authoring discipline

The high-signal authoring rules the handbook doesn't cover: why-encoding, flat-not-nested layout, and Python-helpers bias. Frontmatter-alias discipline lives in its own file: [frontmatter aliases](frontmatter-aliases.md). Each carries decision reasoning, not just procedure.

## Encode the why, not just the what

Skills exist because every agent session starts from zero. There is no Sarah to ask why a rule was written, so undocumented reasoning gets re-derived badly on each fresh context, or the rule gets deleted by an agent who cannot see why it mattered. Each rule below captures decision reasoning, not just procedure. Hold that line for new sections.

Shape: lead with the rule, then a **Why:** line (incident, constraint, prior failure mode that produced it), then a **How to apply:** line (when the rule fires). Date-stamp the flag where useful (e.g. "Flagged 2026-04-26") so a future read can judge whether the why is still load-bearing.

Framing reference: https://simme.dev/posts/the-end-of-just-ask-sarah/.

## Skills are flat, not nested

Every skill is a peer directory directly under `.agents/skills/`. Do **not** nest sub-skills inside another skill's directory (e.g. `meta-skill/sub-skill/SKILL.md`). Nested-skill discovery is poorly supported by the harness, and harness setup only projects top-level skill dirs into native surfaces such as Claude's `.claude/skills/<name>` and Codex's `~/.agents/skills/<name>`. Anything below the top level is invisible to the loader.

When a meta-skill needs to route to other skills, the routed skills live as **flat peers** alongside it. The meta's job is to name them and describe when each fires. The loader handles each one independently.

**Why:** caught early while building a meta-skill router. Initial design assumed sub-dir nesting per a team-coordination plugin pattern. That pattern relied on team-coordination plumbing (separate plugin repo, `commands/` symlinks, etc.) that doesn't apply in a single-operator personal-OS repo. Flat is the only shape the existing setup actually supports.

**How to apply:** when authoring a meta-skill, the routing table lists peer-skill names, not paths into the meta's own dir. New routed skills get their own top-level directory. If you find yourself wanting to nest, that's a signal the meta should instead be a thin SKILL.md that describes shared discipline, with the actual work split across peer skills.

## Bias toward Python helpers, not pure-prompt skills

When a skill parses files, walks directories, queries SQLite, or does any structured data manipulation, write a Python script in the skill directory and have SKILL.md call it. Pure prompt instructions are fine for narrative steps. Python is right for anything where determinism, speed, or testability matter.

Helpers go in the skill dir alongside SKILL.md, get committed to the personal-OS repo, run with the system `python3` (stdlib-first, reaching for trafilatura, lxml, etc. only when stdlib genuinely doesn't suffice).

Flagged 2026-04-26.

**Why:** procedure-as-prompt loses fidelity each time the LLM re-derives boilerplate (date math, path globbing, file IO). Committed Python is auditable, fast, and the same on every host. The LLM tier should focus on synthesis, not parsing.

**How to apply:** new skill that ingests data → start with `script.py` (or named subcommands), have SKILL.md document inputs/outputs/invocation. Existing skill that's been doing parsing in-prompt → migrate to Python on next touch.
