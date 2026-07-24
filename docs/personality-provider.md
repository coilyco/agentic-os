# Agent-compose AOS provider

AOS publishes reusable skills and personality bodies for agent-compose. The
provider is local and deterministic. A consumer reads the checkout directly
and fetches no context over the network.

## Ownership boundary

* **AOS** - owns ordinary skills, role-scoped skills, role bindings, and the shared personality bodies.
* **agent-compose** - owns roles, compatibility, seats, names, pronouns, colors, selection, and materialization.
* **Ward** - owns executable authority, permissions, credentials, and runtime risk controls.

Shared role and skill names join the first two layers without moving authority
into knowledge content.

## Provider contract

Agent-compose infers every ordinary skill from `.agents/skills/`. Every role
receives those skills, apart from inactive personality bodies. It separately
reads `.agents/roles.kdl` and selects only the current role's sources from
`.agents/composed/`.

Composed sources use `COMPOSED.md`, never `SKILL.md`. Agent-compose renames the
selected entrypoint to `SKILL.md` while building the role's isolated bundle.
Unselected role sources leave no catalog metadata or trace in that bundle.
See [role-composed skills](role-composed-skills.md) for the complete contract.

The compose request supplies the stable source id `aos-public`, so adding an
ordinary skill or role binding needs no parallel hand-authored provider
inventory.

Each personality directory carries one canonical `SKILL.md` with its presence,
attention, tempo, voice, and balance definition.

Agent-compose activates the complete ordered personality meld for a selected
role. A request cannot select an arbitrary trait or change compatibility.
Agent-compose owns canonical role-to-personality bindings. AOS commits a narrow
alignment board that verifies each selected body and the actual bundle
manifest without selecting runtime behavior. See
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

`compiled` delivery joins the selected canonical `SKILL.md` bodies into one
document. Native delivery preserves the selected skill directories. Both modes
always carry the shared invariant as source instructions.

Host convergence points `roster_sources` at the AOS checkout. The roster
artifact supplies seat dispatch and linked personality definitions to the
global cascade. Container composition uses the same provider to build one
immutable selected bundle.

## Behavioral boundary

Personality affects attention, framing, tempo, voice, and tie-breaking among
valid actions. Personality does not change truthfulness, uncertainty
reporting, obligations, acceptance criteria, permissions, safety,
escalation, rollback, or completion. A silent successful run needs no
personality theater.

## See also

* [Agents and sessions](features-agents-sessions.md) - host context composition.
* [Role surface tiers](role-surface-tiers.md) - the separate authority boundary.
* [docs/FEATURES.md](FEATURES.md) - shipped AOS capability inventory.
