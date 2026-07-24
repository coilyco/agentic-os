# Role-orientation projections

AOS consumes public role orientation at authoring time while agent-compose
remains the canonical owner of roles, personalities, and seats. Shipped AOS
runtimes never fetch configuration from AOSH.

## Temporary source

Agent-compose#49 will make normal convergence emit its complete structured
person artifact. Until that lands, AOSH's `role-orientation.yaml` mirrors the
needed public fields from agent-compose's embedded person configuration.

The AOS sync reads only role slugs, ordered personalities, and named seats.
Purpose text is not projected. Models, endpoints, hardware, permissions, and
private routing never cross this boundary.

## Named seats

`ward exec sync-role-seats` projects each seat's harness, display name, and
pronouns into marker-bounded `agent` identity fields in
[`.ward/roles.kdl`](../.ward/roles.kdl). AOS continues to own surrounding
guardfiles, models, reasoning effort, and execution policy.

Every projected seat must already have a Ward agent block in that role. A new
harness therefore cannot silently become executable. Duplicate, malformed,
missing, or unconfigured seats fail closed.

## Personality alignment

`ward exec sync-role-personalities` writes
[`role-personalities.json`](../aos/role-personalities.json). The generated board
preserves every role's ordered personality meld and maps each personality slug
to its conventional AOS `personality-*` skill.

The sync requires every selected personality to provide:

* a full `SKILL.md` with matching name and description
* the one shared personality invariant

The board does not select runtime personality. Agent-compose continues to do
that from its person configuration.

## Context verification

Role-seat context capture reads the personality order and selected
`personality-*` skill ids from the actual agent-compose bundle. It compares
both with the generated AOS board before counting context. A missing, added,
reordered, or misbound personality fails closed. A successful snapshot records
the verified meld under `bundle.personalities`.

The seat's agent executable is still unnecessary. Agent-compose materializes
the configuration, while the measurement invokes no agent or model.

## Drift checks

Run `ward exec sync-role-seats -- --check` and `ward exec
sync-role-personalities -- --check` for read-only verification. Local
pre-commit runs both with visible skip behavior when the sibling AOSH checkout
is absent. A present but malformed source always fails.

When agent-compose#49 replaces the temporary AOSH snapshot, the sync input can
change without moving ownership into AOS.

## See also

* [Agent-compose AOS provider](personality-provider.md) - personality bodies and behavioral boundary.
* [Role-seat context snapshots](context-budget-role-seat.md) - measured output contract.
* [AOSH projections](ward-local-models.md) - all authoring-time projection surfaces.
