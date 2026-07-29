# Role-composed principal methods

Principal workflow methods stay out of roles that do not use them. AOS owns
the role allowlists in [`.agents/roles.kdl`](../.agents/roles.kdl).

## Matrix

* **CEO** - OSS stewardship, platform strategy, developer-tool adoption,
  decision architecture, prioritization, product signals, issue decomposition,
  brainstorming, scouts, and issue writing.
* **Director** - Git workflow, supply-chain audit, brainstorming,
  prioritization, all scouts, observer voice, voice linting,
  system-improvement vocabulary, issue decomposition, and skill authoring.
* **Engineer** - Git workflow, supply-chain audit, public-repository writing,
  and system-improvement vocabulary.
* **QA** - Git workflow, code review, and public-repository writing.
* **Ops** - Git workflow, supply-chain audit, and system-improvement
  vocabulary.
* **PM** - brainstorming, prioritization, all scouts, observer voice, voice
  linting, issue decomposition, and skill authoring.
* **Designer** - product brainstorming in addition to the frontend coding
  pair.
* **Social, sales, and customer-success** - observer voice and voice linting.
* **Community** - Discord hosting and architecture, cultural reading, trust
  boundaries, and customer-signal routing.

## Handoffs

PM owns scout discovery, ranking, and portfolio recommendations. Engineer or
ops owns supply-chain verification, installation, implementation, validation,
and landing. PM records returned evidence and outcomes without inheriting
execution authority.

Community owns routine member interaction and a clean handoff. Human stewards
retain moderation decisions, while customer-success, PM, ops, and engineering
receive signals that belong to their work.

Composition grants knowledge only. Ward's fixed workflow and the separately
selected AOSguard surface still control tools, credentials, and write
authority.

Role composition is the current coarse gate for skill authoring. Frontier
model refinement follows
[agent-compose#70](https://forgejo.coilysiren.me/coilyco-flight-deck/agent-compose/issues/70)
and
[agentic-os#716](https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os/issues/716).

## See also

* [Role-composed skills](role-composed-skills.md) - source layout and
  composition contract.
* [AOS and Ward boundary](ward-specs.md) - runtime authority.
