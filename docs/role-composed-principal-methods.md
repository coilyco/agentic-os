# Role-composed principal methods

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
* **Community** - Discord, customer-success, and social-writing methods,
  including the editorial loop.
* **Portfolio Strategist** - the deduplicated advisor, PM, and CEO methods for
  evidence synthesis, portfolio allocation, program decomposition, scouts,
  skill authoring, and issue writing.
* **Content Manager** - the complete writing family for public-repository
  writing, editorial work, channel context, and voice, excluding issue
  decomposition after its move into the tooling taxonomy.

## Handoffs

Portfolio Strategist owns scout discovery, ranking, and portfolio recommendations. Engineer or
ops owns supply-chain verification, installation, implementation, validation,
and landing. Portfolio Strategist records returned evidence and outcomes without inheriting
execution authority.

Community owns routine member interaction and a clean handoff. Human stewards
retain moderation decisions, while Portfolio Strategist, ops, and engineering
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
