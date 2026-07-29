# Role-orientation projections

Agent-compose owns roles, named seats, identity, and personalities. AOS uses a
public personality projection only for context-budget measurement. Ward does
not receive a role-seat or identity projection.

## Personality alignment

Normal agent-compose convergence emits a complete person snapshot at
`~/.agent-compose/sources/personality/person.json`. The AOS personality sync
reads its role order, ordered melds, and canonical skill bindings, then writes
[`role-personalities.json`](../aos/role-personalities.json).

The board does not select runtime personalities. Agent-compose still supplies
the selected composed role to the harness, including its seat and identity.
The context-bundle path preserves that behavioral material without turning it
into Ward configuration or authority.

Run `ward exec sync-role-personalities -- --check` after agent-compose
convergence for a read-only verification.

## See also

* [Agent-compose AOS provider](personality-provider.md)
* [AOS context-bundle adapter](aos-context-bundle.md)
