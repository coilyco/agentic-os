---
name: tooling-skill-authoring
description: Author, place, prune, and validate skills. Covers admission, ordinary vs composed scope, triggers, structure, why-encoding, and deterministic helpers.
---

# Skill authoring

## Handbook

Start with [skill admission](references/admission.md) and
[placement](references/placement.md). Structural rules live in the
[handbook](references/handbook.md), drafting procedure in the
[authoring walkthrough](references/authoring-walkthrough.md), and shaped
sources in [`templates/`](templates/).

This entrypoint carries the highest-frequency authoring discipline.

## Location

Ordinary skills live at `<provider-repo>/.agents/skills/`. They use `SKILL.md`
and form the harness-discoverable surface.

Role-scoped sources live at `<provider-repo>/.agents/composed/`. They use
`COMPOSED.md`, bind in `.agents/roles.kdl`, and become `SKILL.md` only after
agent-compose selects the role. Never place `SKILL.md` under `.agents/composed/`.

Per-repo co-location is only for design or usage reference scoped to that repo.
Runbooks and partial-failure playbooks stay central. Requirements live in
[`references/co-location.md`](references/co-location.md).

## Authoring

Create an ordinary source with `SKILL.md`, or a role-scoped source with
`COMPOSED.md`. Both use the same `name` and `description` frontmatter. Bind
each role-scoped source in `.agents/roles.kdl`.

Run the validator before committing for fast feedback:

```sh
pre-commit run skill-conventions --all-files
pre-commit run dead-cross-links --all-files
pre-commit run em-dash-check --all-files
```

The structural validators enforce the category taxonomy from
`.agents/skills/categories.yaml`. `check-skills` covers ordinary sources.
`check-composed-skills` covers role-scoped sources and their isolation.

## Opinionated discipline

Decision reasoning for these rules lives in
[`references/opinionated-discipline.md`](references/opinionated-discipline.md):

- **Encode why** - preserve reasoning across fresh sessions.
- **Keep sources flat** - every source is a peer directory.
- **Use deterministic helpers** - scripts parse and transform, models synthesize.

Description budgets and router exceptions live in
[`references/frontmatter-aliases.md`](references/frontmatter-aliases.md).

## Extended discipline

Extended rules for runbooks, plugins, and documentation live in
[`references/discipline.md`](references/discipline.md).

## Triggers

skill, SKILL.md, COMPOSED.md, composed skill, role-scoped skill, roles.kdl,
frontmatter, authoring skill, validator, categories.yaml, skill admission,
skill placement, catalog pruning, model-native knowledge, volatile knowledge,
specialist upskilling, flat layout, deterministic helpers.
