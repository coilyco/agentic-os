# AOSH projections into AOS

Agent-compose owns behavioral roles, seats, and identity. AOSH owns backend
selection and model-opaque role intents. AOS owns both the public harness
capability registry and the concrete deployment tuning that its launchers
apply. Shipped AOS runtimes embed their projections and never fetch an
authoring repository at launch.

## Role-intent harness board

AOS's [`.agents/harnesses.yaml`](../.agents/harnesses.yaml) defines public
harness identity and intent compatibility. AOSH's `roles.yaml` and
`agent-selections.yaml` define the model-opaque role joins and lane choices.
`ward exec sync-harness-board` validates the joined inputs, updates generated
`intent` children in [`.agents/roles.kdl`](../.agents/roles.kdl), and rewrites
[`role-harnesses.json`](../aos/role-harnesses.json) as the compiled launcher
view.

The board projection has role, intent, harness, and route data only. Models,
endpoints, reasoning, hardware, permission, and routing rationale do not cross
that projection boundary.

The released `aos` binary embeds the compiled view. `aos --role ROLE
lane-default --intent INTENT` emits the selected harness and stable route
without backend identity. Run `ward exec sync-harness-board -- --check` for a
read-only drift check.

## Concrete launch profiles

[`harness_launch_profiles.json`](../aos/harness_launch_profiles.json) is the
AOS-owned deployment source for model, reasoning-effort, verbosity, and local
endpoint values. Its role overrides preserve the launch tuning formerly stored
under `.ward`, while its defaults preserve harness-wide values.

For a Ward launch, `aos` resolves one role and harness profile and passes it
through Ward's typed `--config` inputs. Ward does not load the registry and the
profile grants no authority. Standalone Codex bootstrap reads the same embedded
registry, so the two launch paths cannot silently choose different tuning.

## Personality alignment

`ward exec sync-role-personalities` reads agent-compose's emitted person
snapshot and writes [`role-personalities.json`](../aos/role-personalities.json)
for context-budget verification. Agent-compose remains the runtime owner of
personalities and named seats.

## See also

* [Harness selection](harness-selection.md)
* [Role orientation projections](role-orientation-projections.md)
