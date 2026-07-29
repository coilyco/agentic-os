# Agent-compose AOS capability provider

AOS publishes reusable ordinary and role-composed skills for agent-compose.
The provider is local and deterministic. A consumer reads the checkout
directly and fetches no context over the network.

## Ownership boundary

* **AOS** - owns ordinary skills, role-scoped skills, role bindings, and the public agent and harness capability registry.
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

The compose request supplies the stable source id `aos-public`, so adding an
ordinary skill or role binding needs no parallel provider inventory.

Skill frontmatter owns the [model-class decision](skill-model-classes.md).
Role selection runs first, so pruning never widens the role boundary.

The provider also publishes `.agents/harnesses.yaml` as the canonical
model-opaque registry of agent and harness identity, source links, and
compatible intents. AOSH consumes a generated mirror for offline scoring while
retaining role joins, lane selections, models, and backend routes.
Harness identity remains registry data rather than ordinary skill entries, so
every role does not pay a duplicate catalog routing cost.

Agent-compose activates the complete ordered personality meld for a selected
role. A request cannot select an arbitrary trait or change compatibility.
Agent-compose embeds the invariant and all 16 canonical definition trees under
its `person:kai` source. AOS commits a narrow 12-role alignment board that
verifies the ordered meld and skill ids in measured bundles without selecting
runtime behavior. See
[role-orientation projections](role-orientation-projections.md).

## Local consumer use

An agent-compose request stored under the provider root admits that root:

```kdl
compose {
    role "engineer"
    delivery "native-skills"
    source "aos-public" root="." required=#true
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

* [Agents and sessions](features-agents-sessions.md) - host context composition.
* [AOS and Ward boundary](ward-specs.md) - the authority boundary.
* [docs/FEATURES.md](FEATURES.md) - shipped AOS capability inventory.
