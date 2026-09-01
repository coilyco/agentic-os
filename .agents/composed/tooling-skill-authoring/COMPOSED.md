---
name: tooling-skill-authoring
description: Author, evaluate, place, prune, and validate skills. Covers admission, scope, trigger quality, behavioral deltas, structure, provenance, and deterministic gates.
license: Apache-2.0
metadata:
  source-url: https://github.com/anthropics/skills/tree/main/skills/skill-creator
---

# Skill authoring

Director and PM use this method to decide whether a capability deserves a skill,
where it belongs, and what rule an implementation role receives. Repo
changes go to engineer as a bounded source change, not this whole catalog.

## Handbook

Start with [skill admission](references/admission.md) and
[placement](references/placement.md). Structural rules live in the
[handbook](references/handbook.md), with drafting in the
[authoring walkthrough](references/authoring-walkthrough.md) and behavioral
checks in the [evaluation loop](references/evaluation.md). Shaped sources live
in [`templates/`](templates/).

## Location

Ordinary skills use `SKILL.md` under `<provider-repo>/.agents/skills/`.

Role-scoped sources live at `<provider-repo>/.agents/composed/`. They use
`COMPOSED.md`, bind in `.agents/roles.kdl`, and become `SKILL.md` only after
agent-compose selects the role. Never place `SKILL.md` under `.agents/composed/`.

Per-repo co-location is only for design or usage reference scoped to that repo.
Runbooks and partial-failure playbooks stay central. Requirements live in
[`references/co-location.md`](references/co-location.md).

## Authoring

Create an ordinary `SKILL.md` or role-scoped `COMPOSED.md`. Both use the same
`name` and `description` frontmatter. Bind role-scoped sources in `.agents/roles.kdl`.

Run the validator before committing for fast feedback:

```sh
pre-commit run skill-conventions --all-files
pre-commit run dead-cross-links --all-files
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
specialist upskilling, trigger evaluation, behavioral evaluation, false trigger,
missed trigger, task regression, deterministic helpers.
