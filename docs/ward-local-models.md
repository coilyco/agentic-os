# AOSH projections into AOS

AOSH owns hand selection and backend routing. AOS consumes two narrow,
authoring-time projections. Both commands read a sibling AOSH checkout, write an
AOS-owned artifact, and commit that result. Shipped AOS runtimes never fetch
config upward from AOSH.

## Role-intent harness board

AOSH's hand-owned `roles.yaml`, `agent-selections.yaml`, and `harnesses.yaml`
define the model-opaque board. `ward exec sync-harness-board` validates all ten
roles and sixteen lanes, then rewrites generated `intent` children inside the
agent-compose provider roles in [`.agents/roles.kdl`](../.agents/roles.kdl).
The same run updates
[`role-harnesses.json`](../aos/role-harnesses.json) as the compiled launcher
view.

The projection contains only its format, role-source provenance, counts, and
role-intent-harness assignments. Backend model, server, score, fallback,
hardware, orchestrator, and selection rationale do not cross the boundary.

The released `aos` binary embeds the compiled JSON view.
Ward's [`.ward/roles.kdl`](../.ward/roles.kdl) remains execution-only and does
not consume role-intent harness routes.
`aos --role ROLE harness-default --intent INTENT` resolves a lane and emits only
the harness slug. Role remains control-plane provenance and never becomes a
harness argument.

Run `ward exec sync-harness-board -- --check` for a read-only drift check. The
local pre-commit suite runs it with `--if-present`. A missing sibling AOSH
checkout skips visibly, while missing or malformed files inside a present
checkout fail closed.

## Local model overlay

AOSH's generated `94-pairings.yaml` is the Goose model-selection source. Its
generated `90-inventory.yaml` records provisioned model-server pairs. The AOS
sync rejects the selection unless the inventory contains exactly one matching
entry with `keep: true`.

The published value lives as a sparse top-level agent overlay in
[`agents.kdl`](../.ward/agents.kdl). Ward merges it with its generic Goose
launch definition. OpenCode remains deployment-local AOS backend policy and is
not rewritten from the engineer role's OpenHands route.

Run `ward exec sync-local-models` after AOSH selects a new Goose route. The
command discovers the sibling checkout, validates the selected model against
the provisioned inventory, and rewrites only the Goose model line.

Run `ward exec sync-local-models -- --check` for a read-only drift check. The
local pre-commit suite applies the same visible-skip behavior as the harness
board check.

## See also

* [Harness selection](harness-selection.md) - the projected board and resolver.
* [Ward spec bundle](ward-specs.md) - AOS-authored Ward overlays.
* [Ward profile assets](ward-profile-assets.md) - profile-provider inputs.
