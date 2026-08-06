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

The two roots share the taxonomy in `.agents/skills/categories.yaml`. A name cannot
exist in both roots. No composed source may contain `SKILL.md`, including nested support directories.

## Role binding

AOS owns the allowlist in `.agents/roles.kdl`. This abridged example shows two role bindings:

```kdl
roles {
    role engineer {
        composed-skill coding-shape-cli
    }
    role design {
        composed-skill tooling-designer-interaction-shaping
    }
}
```

Each binding names an existing directory under `.agents/composed/`. Duplicate roles,
duplicate bindings, unknown roles, and missing sources fail composition. Every
AOS-deployed role has a block and at least one composed source.

A binding may use a quoted shell-style glob for a semantically homogeneous family,
such as `composed-skill "coding-*"`. Keep unrelated sources exact. Agent-compose expands
matches in lexical order and fails when a pattern is invalid, empty, or overlapping.
A future source matching the family is intentionally admitted without another role edit.

Role slices follow the complete coding, design, community, strategy, content,
verification, and operations boundaries recorded in `.agents/roles.kdl`. The
[role-skill coverage audit](role-skill-coverage-audit.md) records the second-pass
placement rationale and deliberate exclusions without duplicating that config.

The [principal workflow matrix](role-composed-principal-methods.md) records the broader role-gated method catalog and handoffs.

## Composition

Agent-compose selects ordinary skills for every role, then adds only the current role's
composed allowlist. It copies each selected source into the isolated output and renames
`COMPOSED.md` to `SKILL.md`. No unselected composed source appears in the role's skill
catalog, files, manifest, or selection trace. This makes the boundary about context load
and role focus, not lightweight routing hints.

## Authority boundary

`.agents/roles.kdl` grants knowledge only. It does not grant commands, credentials,
network access, mounts, or runtime permissions. Ward remains the authority layer.

## Validation

`check-skills` validates ordinary sources. `check-composed-skills` validates
composed layout and content. `documentation-layout`, `dead-cross-links`, and
`source-doc-refs` understand both entrypoint names.

## See also

* [Agent-compose AOS provider](personality-provider.md) - provider inference and delivery.
* [AOS and Ward boundary](ward-specs.md) - the separate authority model.
* [Skill discipline](skill-discipline.md) - authoring and validator catalog.
* [docs/FEATURES.md](FEATURES.md) - shipped AOS capability inventory.
