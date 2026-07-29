# Role-composed skills

AOS can carry deep knowledge that only selected agent roles receive. The source
stays invisible to harness discovery until agent-compose builds a role bundle.

## Source layout

Ordinary knowledge remains globally discoverable:

```text
.agents/skills/<name>/SKILL.md
```

Role-scoped knowledge uses a distinct root and entrypoint:

```text
.agents/composed/<name>/COMPOSED.md
```

The two roots share the taxonomy in `.agents/skills/categories.yaml`. A name
cannot exist in both roots. A composed source may not contain `SKILL.md`
anywhere, including nested support directories.

## Role binding

AOS owns the allowlist in `.agents/roles.kdl`. This abridged example shows two
role bindings:

```kdl
roles {
    role engineer {
        composed-skill coding-shape-cli
    }
    role designer {
        composed-skill tooling-designer-interaction-shaping
    }
}
```

Each binding names an existing directory under `.agents/composed/`. Duplicate
roles, duplicate bindings, unknown roles, and missing sources fail composition.
Every canonical agent-compose role has a block and at least one composed
source.

The complete `coding-*` family and `mcp-server-patterns` are composed for engineer,
director, QA, and ops. `design-system` is composed for designer, engineer, and QA.
Community receives only the Discord architecture source.

CEO receives OSS stewardship, platform strategy, and developer-tool adoption methods. Specialist sources also serve advisor,
designer, QA, ops, PM, social, community, sales, and customer-success.

The [principal workflow matrix](role-composed-principal-methods.md) records the broader role-gated method catalog and handoffs.

## Composition

Agent-compose selects ordinary skills for every role, then adds only the
current role's composed allowlist. It copies each selected source into the
isolated output and renames `COMPOSED.md` to `SKILL.md`.
No unselected composed source appears in the role's skill catalog, files,
manifest, or selection trace. This makes the boundary about context load and
role focus, not lightweight routing hints.

## Authority boundary

`.agents/roles.kdl` grants knowledge only. It does not grant commands,
credentials, network access, mounts, or runtime permissions. Ward remains the
authority layer for those capabilities.

## Validation

`check-skills` validates ordinary sources. `check-composed-skills` validates
composed layout and content. `documentation-layout`, `dead-cross-links`, and
`source-doc-refs` understand both entrypoint names.

## See also

* [Agent-compose AOS provider](personality-provider.md) - provider inference and delivery.
* [AOS and Ward boundary](ward-specs.md) - the separate authority model.
* [Skill discipline](skill-discipline.md) - authoring and validator catalog.
* [docs/FEATURES.md](FEATURES.md) - shipped AOS capability inventory.
