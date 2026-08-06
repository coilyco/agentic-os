# Role-seat context snapshots

The context-budget report measures a requested `(role, seat)` pair. The role selects the composed briefing and role-scoped skills. The seat selects one of
AOS's four agent-compose projection layouts and its AGENTS-family cascade.
No model route selector is a measurement input.

The command asks the local `agent-compose` binary to perform three operations:

* `roster` validates that the requested role exists.
* `compose` builds the role's exact `native-skills` bundle. Claude and Codex
  request the frontier class. Goose and OpenCode request the low-context class.
* `project` materializes that bundle into the seat's home-scope load points.

The measurement compares the bundle's ordered personality meld and selected
personality skill ids directly with the person snapshot emitted by the same
`agent-compose roster` call, then reads the generated files. The snapshot
records that validated meld under `bundle.personalities`. Personality skills
are attributed to `roster:core`. The seat's agent executable does not need to
be installed. The command never invokes an agent, inference, a backend model,
hardware, an endpoint, or another live service.

The AOS checkout is the default provider, repository, and CWD. `--provider`,
`--repo`, and `--cwd` make those inputs explicit for fixtures or reproduction.
Named multi-provider capture is part of the
[role-seat capture workflow](context-budget-role-seat-capture.md).

## Measurement boundary

The snapshot separates:

* **Eager context** - the projected role instructions, the
  [AGENTS inventory](agents-context-inventory.md) global and root-to-CWD
  cascade for the selected seat, and selected ordinary, role, personality,
  role-composed, and optional plugin skill frontmatter.
* **Lazy context** - selected skill bodies and resources, optional plugin
  bodies and resources, and the count of deferred mcporter registrations.
* **MCP schemas** - zero eager schemas. The mcporter inventory contributes only
  a deferred registration count. The snapshot copies no configuration and
  queries no server.

The YAML artifact groups non-skill components and one top-level breakdown by
**eager** or **lazy** delivery and kind. Each skill appears once under `skills`
with its class, eager tokens, lazy tokens, and resource count. The top-level
payload hash remains content-sensitive. Stable ordering and the absence of
timestamps and absolute source locators make identical inputs produce the same
payload hash.

The shared inventory describes global and repository delivery. Role-seat snapshots
omit projected global context and retain the repository root-to-CWD AGENTS cascade.

Snapshots use `context-budget-<role>-<seat>-<phase>.yaml`. Every current
role-seat snapshot remains individually addressable by that stable filename.
Aggregate role reports link every checked-in current snapshot.

The [current role-class inventory](context-budget-role-seat-current.md) links
one report per role in
[the AOS launch profiles](../.agents/harness-launch-profiles.yaml). The report
generator derives available model classes from the checked-in snapshots and
validates every snapshot before including its measurements. Snapshot
availability remains measurement evidence, not a runtime compatibility rule.

## Capture and compare

The [role-seat capture workflow](context-budget-role-seat-capture.md) gives the
single-provider, multi-provider, and comparison commands plus their fail-closed
validation contract.
