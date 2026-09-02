# Role context snapshots

The per-role snapshots and how they are captured.

The context report measures one composed role. The role selects the complete
briefing, personality meld, and role-scoped skills. Harness, projection layout,
model route, model family, and context-window size are not inputs.

The command asks the local `agent-compose` binary to perform two operations:

* `roster` validates that the requested role exists.
* `compose` builds the role's complete `native-skills` bundle. AOS supplies the
  first role-supported tier from that roster only as Agent Compose compatibility
  metadata.

The measurement validates the bundle's ordered personality meld against the
person snapshot emitted by the same roster call, then reads the bundle directly.
It does not run Agent Compose's `project` operation. The command never invokes a
harness, agent, inference backend, model, hardware endpoint, or live service.

The AOS checkout is the default provider, repository, and CWD. `--provider`,
`--repo`, and `--cwd` make those inputs explicit for fixtures or reproduction.
Named multi-provider capture is part of the
[role capture workflow](context-budget-roles.md).

## Measurement boundary

The snapshot separates:

* **Eager context** - role instructions, the harness-neutral repository
  `AGENTS.md` root-to-CWD cascade, the composed operating base, and ordinary,
  role, personality, role-composed, and optional plugin skill frontmatter.
* **Operating base** - each provider's root `AGENTS.md`, the files a harness
  composes into its global load point. The active repository is measured by the
  cascade above and skipped here, so no file is counted twice. Before this
  existed the snapshot counted only the active repository, which under-reported
  a real seat by roughly 30,000 bytes and hid the largest eager surface in the
  estate from the instrument built to rank it.
* **Lazy context** - skill bodies and resources, optional plugin bodies and
  resources, and the count of deferred mcporter registrations.
* **MCP schemas** - zero eager schemas. The mcporter inventory contributes only
  a deferred registration count. The snapshot copies no configuration and
  queries no server.

The YAML artifact groups non-skill components and one top-level breakdown by
**eager** or **lazy** delivery and kind. Each skill appears once under `skills`
with its class, eager tokens, lazy tokens, and resource count. Stable ordering,
no timestamps, and no absolute source locators make identical inputs produce the
same payload hash.

Still outside the boundary: the sections Agent Compose generates into the host
global rather than reading from a provider, which are the seat dispatch table
and the personality invariant. Measuring those needs a composition rather than a
file read.

Snapshots use `context-budget-<role>-<phase>.yaml`. AOS owns the reusable
measurement and validation contract. The consumer that determines the complete
provider combination owns its current snapshots and aggregate inventory.

## Capture and compare

The [role capture workflow](context-budget-roles.md) gives the
single-provider, multi-provider, and comparison commands plus their fail-closed
validation contract.

## Role context capture

The role context report can capture one AOS bundle, combine named capability
providers, or compare a new bundle with a committed baseline. It invokes Agent
Compose but never projects to or launches a harness, agent, or model.

## Single provider

Capture the default AOS provider:

```sh
just context-budget --role sysadmin \
  --snapshot /tmp/context-budget-sysadmin-current.yaml
```

## Multiple providers

Repeatable `--additional-provider ID=PATH` arguments admit other capability
providers into the same verified bundle:

```sh
just context-budget --role sysadmin \
  --additional-provider private-context=/path/to/provider \
  --snapshot /path/to/private/context-budget-sysadmin-current.yaml
```

Agent Compose performs ordinary, role-composed, collision, and shadowing
decisions across the complete source set. The snapshot preserves each source id,
attributes selected skills to that provider, and records the provider identity
map without copying private source content into AOS.

## Compare

Copy the current snapshot to a task-scoped baseline, then capture and compare:

```sh
just context-budget --role sysadmin \
  --compare /tmp/context-budget-ops-baseline.yaml \
  --snapshot /tmp/context-budget-sysadmin-current.yaml
```

`--skill-root` remains the separate input for plugin skills declared outside the
verified bundle. Duplicate skill ids, an unknown role, personality drift, an
unnamed or malformed provider, a wrong bundle role, or a mismatched comparison
subject fails closed.
