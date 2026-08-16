# Context measurement report

`check-context-budget` reports the eager startup context installed for each
harness. It measures token cost without assigning a harness-specific threshold
or changing what Agent Compose selects. The on-demand `ward context-budget`
verb stays outside the universal commit path.

The [role snapshot mode](context-budget-roles.md) measures any Agent Compose role
without selecting or requiring a harness executable.

## What it measures

The report sums everything a harness ingests at session start across three
axes, each with a different growth lever:

* **doc** - the installed AGENTS.md or CLAUDE.md load point. It reads the file
  directly, so the bytes match what the harness receives. Edit the inputs owned
  by Agent Compose to change this surface.
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

## See also

* [Agents and sessions](features-agents.md) - Agent Compose, the
  composer this measures.
* [Role-composed skills](role-composed-skills.md) - role-gated skills selected
  into a role bundle.
* [.ward/ward.yaml](../.ward/ward.yaml) - allowlisted commands.
