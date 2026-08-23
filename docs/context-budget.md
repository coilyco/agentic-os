# Context measurement and the capability provider

`check-context-budget` reports the eager startup context per harness, measuring
token cost without assigning a threshold or changing what Agent Compose selects.
The on-demand `ward context-budget` verb stays outside the universal commit
path, and its [role snapshot mode](context-budget-roles.md) measures any role
without requiring a harness executable.

## What it measures

The report sums everything a harness ingests at session start across three
axes, each with a different growth lever:

* **doc** - the installed AGENTS.md or CLAUDE.md load point, read directly so
  the bytes match what the harness receives. Edit the Agent Compose inputs.
* **skills** - every mounted skill's `SKILL.md` frontmatter. Names and
  descriptions are eager so the model can discover skills, while bodies load
  lazily. Prune or sharpen the skill catalog to change this surface.
* **mcp** - MCP tool schemas. The shared mcporter inventory is projected into
  each supported native registry. Harness schema discovery stays deferred, so
  the eager figure is approximately zero and the report shows a server count.

These axes form the **proactive** tier. The `immediate_walk` and
`peripheral_walk` primitives, plus the repeatable `--immediate` and
`--peripheral` flags, measure reachable working-directory and reference-repo
tiers. See [context tiers](agents-context-inventory.md).

### Skill scope follows the CWD

Skill roots can be global or CWD-scoped. Relative roots expand across the
workspace and deduplicate by resolved path. `DEFAULT_SKILL_ROOTS` supplies the
defaults. Agent Compose's `skill_load_points:` replaces a harness global root,
while legacy `skill_roots:` remains an explicit override.

## Composition is uniform

AOS does not select context by harness, model family, or context-window size.
Every harness receives the complete selected role bundle. Agent Compose still
requires one role-compatible tier token, so AOS uses the first tier from that
same roster role. The token does not come from a harness or runtime model, and
AOS skill sources no longer carry tier-pruning metadata. Role snapshots stop at
the composed bundle and never run a harness projection.

## Token counting

The report uses a deterministic characters-divided-by-four proxy. It is not an
exact model tokenizer, but it is hermetic and consistent enough for comparing
the same surfaces over time.

## Flags

`--mcporter` points at the shared inventory projected into each native harness
registry. `--role ROLE` enters role snapshot mode.
`--snapshot` writes evidence and `--compare` renders a deterministic component
delta against earlier evidence. The command has no threshold or over-budget
exit mode.

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
    role "platform"
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
valid actions. It never changes truthfulness, uncertainty reporting,
obligations, acceptance criteria, permissions, safety, escalation, rollback, or
completion, and a silent successful run needs no personality theater.
