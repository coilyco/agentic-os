# Role-seat context snapshots

The context-budget report measures a requested `(role, seat)` pair. The role
selects the composed briefing and role-scoped skills. The seat selects the
agent-compose projection layout and the AGENTS-family cascade for that seat.
Harness routing and intent selection are not measurement inputs.

The command asks the local `agent-compose` binary to perform three operations:

* `roster` validates that the requested seat exists under the requested role.
* `compose` builds the role's exact `native-skills` bundle.
* `project` materializes that bundle into the seat's home-scope load points.

The measurement compares the bundle's ordered personality meld and selected
personality skill ids with the committed
[AOS role-personality projection](../aos/role-personalities.json), then reads
the generated files. The snapshot records that validated meld under
`bundle.personalities`. The seat's agent executable does not need to be
installed. The command never invokes an agent, inference, a backend model,
hardware, an endpoint, or another live service.

The AOS checkout is the default provider, repository, and CWD. `--provider`,
`--repo`, and `--cwd` make those inputs explicit for fixtures or reproduction.

## Measurement boundary

The snapshot separates:

* **Eager context** - the projected role instructions, the
  [AGENTS inventory](agents-context-inventory.md) global and root-to-CWD
  cascade for the selected seat, and selected ordinary, personality,
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

The shared AGENTS inventory preserves a source that arrives through both global
and repository delivery paths as two occurrences.

Concrete snapshot files use
`context-budget-<role>-<seat>-<phase>.yaml`. The checked-in
[ops/codex baseline](context-budget-ops-codex-before.yaml) is the current
comparison point.

## Capture and compare

Capture the `ops/codex` baseline:

```sh
ward exec context-budget -- --role ops --seat codex \
  --snapshot docs/context-budget-ops-codex-before.yaml
```

After a refactor, render the component and total delta:

```sh
ward exec context-budget -- --role ops --seat codex \
  --compare docs/context-budget-ops-codex-before.yaml \
  --snapshot /tmp/context-budget-ops-codex-after.yaml
```

`--skill-root` adds a seat or plugin skill root when the projected runtime
declares one outside the verified bundle. Duplicate skill ids, an unknown
role-seat pair, personality drift, a malformed provider, a wrong bundle role,
an unsafe projected entry point, or a mismatched comparison subject fails
closed.
