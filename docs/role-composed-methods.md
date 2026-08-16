# Role-composed methods and coverage

## Role-composed principal methods

Principal workflow methods stay out of roles that do not use them. AOS owns
the role allowlists in [`.agents/roles.kdl`](../.agents/roles.kdl).

## Matrix

* **Director** - core Git, licensing, supply-chain, and architecture-shape
  guidance, plus decision architecture, PM methods, prioritization, all scouts,
  observer voice, voice linting, system-improvement vocabulary, issue
  decomposition, and skill authoring.
* **Engineer** - Git workflow, supply-chain audit, public-repository writing,
  and system-improvement vocabulary.
* **QA** - Git workflow, supply-chain audit, code review, and
  public-repository writing.
* **Ops** - Git workflow, supply-chain audit, and system-improvement
  vocabulary.
* **Designer** - the PM family and design methods in addition to the frontend
  coding pair.
* **Executive Strategist** - the deduplicated advisor, PM, and CEO methods for
  evidence synthesis, portfolio allocation, program decomposition, scouts,
  skill authoring, and issue writing.
* **Content Creator** - the complete writing family for public-repository
  writing, editorial work, channel context, and voice, excluding issue
  decomposition after its move into the tooling taxonomy. It also carries the
  Discord and customer-success methods, which it inherited when Community
  stopped being a deployed role.

## Handoffs

Executive Strategist owns scout discovery, ranking, and portfolio recommendations. Engineer or
ops owns supply-chain verification, installation, implementation, validation,
and landing. Executive Strategist records returned evidence and outcomes without inheriting
execution authority.

Content Creator owns routine member interaction and a clean handoff. Human stewards
retain moderation decisions, while Executive Strategist, ops, and engineering
receive signals that belong to their work.

Composition grants knowledge only. Ward's fixed workflow and the separately
selected AOSguard surface still control tools, credentials, and write
authority.

Role composition is the current coarse gate for skill authoring. Frontier
model refinement follows
[agent-compose#70](https://forgejo.coilysiren.me/coilyco-flight-deck/agent-compose/issues/70)
and
[agentic-os#716](https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os/issues/716).
The skill-authoring method keeps deterministic structure checks as the required
gate, then offers budget-bounded trigger and behavioral evaluation for
admission or revision decisions.

## See also

* [Role-composed skills](role-composed-skills.md) - source layout and
  composition contract.
* [AOS and Ward boundary](ward-specs.md) - runtime authority.

## Role-skill coverage audit

The 2026-08 second pass reviewed every AOS ordinary and composed source against
all eight deployed Agent Compose roles.

## Placement result

The 12 ordinary sources remain ordinary. Each is a cross-role tool adapter whose
trigger and operating boundary are useful regardless of the selected role.

Every composed source is selected by at least one role except the two sales
methods. AOS has no deployed sales role, and those B2B commercial methods do not match
the career or portfolio-investment boundary closely enough to burden the
Executive Strategist.

## Corrections

* **Director** - narrows coding guidance to the decision-relevant core and
  architecture shapes, while adding the PM family used for product decisions.
* **QA** - retains `coding-core-supply-chain-audit` because dependency trust is
  an independent-verification concern.
* **Design** - gains the PM signal-triangulation and program-decomposition methods
  alongside product brainstorming.
* **Content Creator** - carries the social editorial loop alongside
  customer-signal, trust-repair, and Discord methods, inherited when Community
  stopped being a deployed role. Drops issue decomposition, because converting
  plans into tracker work belongs with decision and strategy roles rather than
  content production.
* **AI Engineer** - receives the coding family used to build benchmark runners,
  probes, aggregation tools, and other AI-measurement artifacts.

`.agents/roles.kdl` remains the authoritative selection configuration. This page
records rationale only, so it does not become a second role matrix.
