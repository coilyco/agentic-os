# Role-seat context capture

The role-seat context report can capture one AOS bundle, combine named
capability providers, or compare a new projection with a committed baseline.
It invokes Agent Compose but never launches an agent or model.

## Single provider

Capture the default AOS provider:

```sh
ward exec context-budget -- --role ops --seat codex \
  --snapshot docs/context-budget-ops-codex-current.yaml
```

## Multiple providers

Repeatable `--additional-provider ID=PATH` arguments admit other capability
providers into the same verified bundle:

```sh
ward exec context-budget -- --role ops --seat codex \
  --additional-provider private-context=/path/to/provider \
  --snapshot /path/to/private/context-budget-ops-codex-current.yaml
```

Agent Compose performs ordinary, role-composed, model-tier, collision, and
shadowing decisions across the complete source set. The snapshot preserves each
source id, attributes selected skills to that provider, and records the provider
identity map without copying private source content into AOS.

## Compare

Copy the current snapshot to a task-scoped baseline, then capture and compare:

```sh
ward exec context-budget -- --role ops --seat codex \
  --compare /tmp/context-budget-ops-codex-baseline.yaml \
  --snapshot docs/context-budget-ops-codex-current.yaml
```

`--skill-root` remains the separate input for seat or plugin skills declared
outside the verified bundle. Duplicate skill ids, an unknown role or seat, a
model-tier mismatch, personality drift, an unnamed or malformed provider, a
wrong bundle role, an unsafe projected entry point, or a mismatched comparison
subject fails closed.

## See also

* [Role-seat context snapshots](context-budget-role-seat.md) - measurement and artifact contract.
* [Context budget](context-budget.md) - the complete context-tier model.
