# Agent-compose personality provider

AOS publishes the reusable personality bodies that agent-compose selects for
Kai's synthetic company. The provider is local and deterministic. A consumer
reads one committed declaration and fetches no context over the network.

## Ownership boundary

* **AOS** - owns the shared invariant plus full and brief role-neutral bodies.
* **agent-compose** - owns roles, compatibility, seats, names, pronouns, colors, and selection.
* **Ward** - owns executable authority, permissions, credentials, and runtime risk controls.

The shared slugs join the first two layers without moving authority into
personality content.

## Provider contract

[agent-compose-source.kdl](../agent-compose-source.kdl) declares
the stable source id `aos-public`, one `personality-invariant` instruction,
and all sixteen `personality-<name>` skills. Every path is relative to the AOS
repository root and stays beneath it.

Each personality directory carries two densities:

* `SKILL.md` - the full presence, attention, tempo, and voice definition.
* `BRIEF.md` - the same posture compressed into one short paragraph.

Agent-compose activates the complete ordered personality meld for a selected
role. A request cannot select an arbitrary trait or change compatibility.

## Local consumer use

An agent-compose request admits the committed declaration:

```kdl
compose {
    role "engineer"
    delivery "native-skills"
    density "full"
    source "aos-public" declaration="/path/to/agentic-os/agent-compose-source.kdl" required=#true
}
```

`compiled` delivery uses `BRIEF.md` when the request asks for brief density
and `SKILL.md` for full density. Native delivery preserves the selected skill
directories. Both modes always carry the shared invariant as source
instructions.

Host convergence points `roster_sources` at the same declaration. The roster
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
