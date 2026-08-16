# Agent SDK and capability provider

## Agent SDK patterns

Compact reference map for reusable agent-design patterns. These patterns are implementation options, not defaults. Apply the repository's role, authority, privacy, and validation rules before adopting one.

## Coordination and context

* **Specialist teams** - give subagents differentiated tools and scoped responsibilities, then make one coordinator synthesize their work.
* **Chief-of-staff orchestration** - use a top-level agent, subagents, hooks, and output styles when a task needs several specialist passes.
* **Context engineering** - choose deliberately between durable storage, in-conversation compaction, and tool-result clearing.
* **Manual and automatic compaction** - compact long-running sessions explicitly when the transition needs judgment, or automatically when uninterrupted work matters more.
* **User memory** - distinguish cross-session user preferences from durable repository doctrine and current task state.
* **Session browsers** - treat recorded sessions as a searchable audit source rather than rebuilding a transcript parser.
* **Self-verification** - use a stateless second pass to grade a result when grading costs less than producing it.

## Operations and security

* **Read-only observability** - connect agents to monitoring and CI evidence without granting mutation authority.
* **Incident response** - separate diagnosis, remediation, pull-request creation, and human approval into explicit gates.
* **Vulnerability investigation** - threat-model the target, collect evidence, and report findings structurally before proposing remediation.
* **Threat-intelligence enrichment** - fan out across several bounded read sources, then reconcile results into one report.

## Tools and execution

* **Tool search** - retrieve a small relevant subset of a large tool catalog rather than eagerly loading every definition.
* **Programmatic tool calling** - use a sandboxed program to orchestrate many dependent calls when it materially reduces latency and token cost.
* **Small research agents** - start with a narrow web-research loop before introducing specialist orchestration.

## Managed-agent delivery

* **Production setup** - model credentials, environment lifecycle, idled sessions, and human handoff as first-class concerns.
* **Prompt versioning** - evaluate labeled changes and retain a rollback path for deployed prompts.
* **Data analysis** - use sandboxed file mounts and constrained output formats for structured-data analysis.
* **Chat-surface analysis** - preserve session context across follow-ups while keeping channel authority bounded.
* **Failing-test iteration** - use a small known-bug exercise to verify the basic agent, environment, session, and streaming loop.

## Quality and experience

* **Custom skills** - package recurring organizational workflows into narrow, discoverable instruction surfaces.
* **Tool evaluation** - test one tool definition against many representative tasks rather than inferring quality from a single run.
* **Speculative prompt caching** - warm likely prompt prefixes only when the latency benefit justifies the prediction risk.

## Related

[Skill discipline](skill-discipline.md), [context budget](context-budget.md), [agents and sessions](features-agents.md), and [test harnesses](test-harness.md).

## Agent-compose AOS capability provider

AOS publishes reusable ordinary and role-composed skills for agent-compose.
The provider is local and deterministic. A consumer reads the checkout
directly and fetches no context over the network.

## Ownership boundary

* **AOS** - owns ordinary skills, role-scoped skills, and role bindings.
* **agent-compose** - owns roles, personality compatibility, seats, identity, colors, the personality invariant and definitions, selection, and materialization.
* **Ward and AOSguard** - Ward owns fixed workflow lifecycle and its broker. AOSguard owns the separately selected operator permissions and credentials.

Shared role and skill names join the first two layers without moving authority
into knowledge content.

## Provider contract

Agent-compose infers every ordinary skill from `.agents/skills/`. Every role
receives those skills. It separately reads `.agents/roles.kdl` and selects
only the current role's sources from `.agents/composed/`.

Composed sources use `COMPOSED.md`, never `SKILL.md`. Agent-compose renames the
selected entrypoint to `SKILL.md` while building the role's isolated bundle.
Unselected role sources leave no catalog metadata or trace in that bundle.
See [role-composed skills](role-composed-skills.md) for the complete contract.

The compose request supplies the stable source id `aos`, so adding an
ordinary skill or role binding needs no parallel provider inventory. AOS does
not prune that inventory by harness, model family, or context-window size.

Agent-compose activates the complete ordered personality meld for a selected
role. A request cannot select an arbitrary trait or change compatibility.
Agent-compose embeds the invariant and all 16 canonical definition trees under
its `roster:core` source. Context-budget capture verifies each measured
bundle's ordered meld and skill ids directly against Agent Compose's generated
person snapshot without creating an AOS-owned role or personality copy.

## Local consumer use

An agent-compose request stored under the provider root admits that root:

```kdl
compose {
    role "engineer"
    delivery "native-skills"
    source "aos" root="." required=#true
}
```

`compiled` delivery joins selected `SKILL.md` bodies into one document. Native
delivery preserves selected skill directories. Both modes carry the embedded
invariant and personality definitions without an AOS personality provider.

Host convergence needs no `roster_sources` entry for personalities. Optional roster sources remain overlays.
After skill changes, rerun agent-compose or AOS composition and start a new agent session. Existing bundles do not hot-reload.
Container composition admits the AOS root for capabilities and uses the embedded person source for personalities.

## Behavioral boundary

Personality affects attention, framing, tempo, voice, and tie-breaking among
valid actions. Personality does not change truthfulness, uncertainty
reporting, obligations, acceptance criteria, permissions, safety,
escalation, rollback, or completion. A silent successful run needs no
personality theater.

## See also

* [Agents and sessions](features-agents.md) - host context composition.
* [AOS and Ward boundary](ward-specs.md) - the authority boundary.
* [docs/FEATURES.md](FEATURES.md) - shipped AOS capability inventory.
