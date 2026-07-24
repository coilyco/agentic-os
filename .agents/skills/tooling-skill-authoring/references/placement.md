# Skill placement and freshness

## Placement

Place material at the lowest-cost layer that preserves its intended audience
and behavior:

* **Person context** - identity and preferences relevant to nearly every
  session.
* **`AGENTS.md`** - unconditional repo or workspace operating doctrine.
* **Ordinary `SKILL.md`** - task-specific capability that multiple roles may
  need to discover.
* **Role-scoped `COMPOSED.md`** - deep capability selected roles need and other
  roles should not see in their candidate catalog.
* **Repository docs** - durable explanation without another capability trigger.
* **Runtime retrieval** - current, live, large, or frequently changing facts.
* **Bundled scripts and tools** - deterministic parsing and execution.
* **Validators and code** - mechanically enforceable invariants.
* **Authority systems** - permission to perform an action.

A skill may explain a permission boundary, but the skill never grants
authority. Composed placement controls exposure, not permission or quality.

## Rapidly moving knowledge

Encode the retrieval contract rather than copying volatile facts into prose.
Name the authoritative sources, version check, freshness expectation, conflict
rule, and provenance the agent reports.

Use a dated snapshot only when offline or ambient context is genuinely useful.
Label its source and as-of date, treat it as a fallback, and provide a path to
fresher truth. The refreshing system owns generated snapshots.

Stable methods can remain in a skill. Release details, live state, prices,
inventories, compatibility matrices, and similar facts belong in runtime
retrieval or generated references.

## Ordinary versus composed

Choose ordinary placement when cross-role discovery is useful and the
description earns its place in every catalog. Choose composed placement when
the capability is foundational for one role but irrelevant or overwhelming
for others.

Do not use composed placement to shelter generic public knowledge. Baseline
HTML, accessibility, language syntax, framework usage, and similar common
material remain model knowledge unless local practice, specialist methodology,
or a measured quality delta justifies instruction.

Model class changes instruction density, not the admission rule. A smaller
local model may need narrower rails and more examples. Gate that support to the
model, role, or repository that needs it instead of charging every
frontier-model session.

Return to the [admission test](admission.md) when placement reveals that the
proposed skill has no distinct capability boundary.
