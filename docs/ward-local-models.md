# AOSH projections into AOS

Agent-compose owns behavioral roles, seats, identity, and backend selection.
AOS owns the public harness capability registry and consumes only the
model-opaque role-intent projection needed by its standalone lane resolver.
Shipped AOS runtimes never fetch either source at launch.

## Role-intent harness board

AOS's [`.agents/harnesses.yaml`](../.agents/harnesses.yaml) defines public
harness identity and intent compatibility. AOSH's `roles.yaml` and
`agent-selections.yaml` define the model-opaque role joins and lane choices.
`ward exec sync-harness-board` validates the joined inputs, updates generated
`intent` children in [`.agents/roles.kdl`](../.agents/roles.kdl), and rewrites
[`role-harnesses.json`](../aos/role-harnesses.json) as the compiled launcher
view.

The projection has role, intent, harness, and route data only. Models,
endpoints, reasoning, hardware, permission, and routing rationale do not cross
into AOS or Ward configuration.

The released `aos` binary embeds the compiled view. `aos --role ROLE
lane-default --intent INTENT` emits the selected harness and stable route
without backend identity. Run `ward exec sync-harness-board -- --check` for a
read-only drift check.

## Personality alignment

`ward exec sync-role-personalities` reads agent-compose's emitted person
snapshot and writes [`role-personalities.json`](../aos/role-personalities.json)
for context-budget verification. Agent-compose remains the runtime owner of
personalities and named seats.

## See also

* [Harness selection](harness-selection.md)
* [Role orientation projections](role-orientation-projections.md)
