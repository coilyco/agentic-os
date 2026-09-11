# Authoring discipline (extended)

Opinionated discipline that overflows `SKILL.md`. The SKILL.md keeps the high-frequency rules (why-encoding, flat-not-nested, Python-helpers, frontmatter aliases). This file holds the longer-form rationale.

## Investigation skills live centrally, not co-located with the tool

Investigation / runbook-shaped skills go under the personal-OS repo's `.agents/skills/`, even when the tool they investigate lives in a different repo. A routed ops-investigation meta-skill with peer skills per failure-domain is the canonical shape.

**Why:** real failures cross component boundaries. A single command can fail in a way that implicates several services and hosts at once. An investigator under pressure should not have to clone three repos to find the right runbook. The runbook-monorepo pattern is well-established in SRE practice (Google SRE book, [sre.google/sre-book/](https://sre.google/sre-book/), chapter "Being On-Call"), and downstream tooling (Backstage, incident.io, FireHydrant, OpenTelemetry) all use the same emit-locally / investigate-centrally split. Co-locating optimizes for the runbook **author**. Centralizing optimizes for the runbook **consumer**, who is always the one operating under partial-failure conditions.

**How to apply:** when a new skill is shaped like a runbook (anti-signals, case library, version-pin discipline, "what to check when X breaks"), it lives here regardless of which repo X is in. Co-location is appropriate only for skills that are pure tool-usage reference and never get invoked under failure.

## Plugin marketplace installs (gauntlet etc.)

When editing a plugin's source repo (e.g. a sibling clone), also fast-forward the active marketplace clone at `~/.claude/plugins/marketplaces/<plugin>/` after pushing. Plugin work feels agile this way - same-session edit and use, no waiting for the plugin manager's next refresh.

Push source first, then `git -C ~/.claude/plugins/marketplaces/<plugin> pull --ff-only`.

Only safe for plugins you own (origin in your own namespace). Third-party marketplace clones stay hands-off.

## Documentation discipline (all docs, not just skills)

Four biases:

* **structure over sprawl** - named sections, short files, split when in doubt
* **consistency over uniqueness** - keep standard headings even when the section defaults to a one-line rule
* **strict validation over convention** - encode any rule a script can check, prefer strict failures over advisory drift
* **deduplicate by pointer** - canonical file plus pointer, never two copies of the same list

Markdown layout: root only the universal allow-list, prose in flat `docs/*.md`, skill content in flat skill folders (no subdirs inside a skill). Numeric caps live in the generated [`docs/catalog-caps-reference.md`](../../../../docs/catalog-caps-reference.md) (rendered from the `documentation-layout` / `code-comments` constants by `just gen-caps-reference`). Point there, don't restate. AGENTS.md is the one exception to the default Markdown cap: it gets the larger trifecta cap, since it is loader-bound and holds universal-fire doctrine.

Code comments: a short standalone block, durable explanation moved to `docs/` with the code pointing at the doc. The line-length and contiguous-block caps live in the same generated reference, not restated here.
