# Skill Discipline Handbook

These are the skill-discipline rules the pre-commit hooks in this repo
enforce. The machine-readable spec is
[`skill-discipline-example-categories.yaml`](skill-discipline-example-categories.yaml)
and the hooks are declared in [`.pre-commit-hooks.yaml`](../.pre-commit-hooks.yaml).
Where this file and the spec disagree, the spec is authoritative for the
validator and this file is what needs fixing.

Sections 6 to 9 are in
[`skill-discipline-conventions.md`](skill-discipline-conventions.md) and the
hooks themselves are in
[`skill-discipline-hooks.md`](skill-discipline-hooks.md).

## 1. Layout

A repo using these hooks ships ordinary skills under `.agents/skills/`.
Role-scoped sources may live under `.agents/composed/`, where harnesses cannot
discover them directly. The layout the hooks expect:

```
<your-repo>/
├── .agents/skills/
│   ├── categories.yaml        # spec consumed by skill-conventions hook
│   ├── <skill-name>/
│   │   ├── SKILL.md           # frontmatter + body
│   │   └── references/        # optional, for content that overflows SKILL.md
│   └── <another-skill>/
│       └── SKILL.md
├── .agents/composed/
│   └── <role-scoped-skill>/
│       └── COMPOSED.md        # promoted to SKILL.md only when selected
├── .agents/roles.kdl          # role-to-composed-skill allowlist
└── .pre-commit-config.yaml    # declares this repo's hook subscriptions
```

Every source is a peer directory directly under its owning root. **Sources
must be flat**, never nested inside another source. Ordinary skills are
globally discoverable. Composed sources are visible only after agent-compose
selects the current role and promotes `COMPOSED.md` to `SKILL.md`.

## 2. Categories

`categories.yaml` lists the families of skills the repo allows. Two kinds:

* **Prefix family**: every directory whose name starts with the prefix matches. Example: `coding-` matches `coding-typescript`, `coding-rust`, etc.
* **Exact-name**: a single named skill. Use for routers, meta-skills, and one-offs that do not fit a family.

The validator rejects any skill whose name does not match an allowed prefix or exact entry. This is the point. If you have a genuinely new shape:

1. Add the new prefix or exact entry to `categories.yaml`.
2. Update this handbook (or your project's version of it) so future authors know the family exists and what it is for.
3. Then create the first skill in the new family.

Do not bypass the spec by silently adding a skill with an unrecognized name.

## 3. SKILL.md frontmatter

Every SKILL.md begins with YAML frontmatter. Two fields required:

```yaml
---
name: <directory-name>
description: <one paragraph; pack keyword aliases for discoverability>
---
```

Rules (validator-enforced):

* `name` MUST equal the directory name.
* `description` MUST be non-empty.
* `description` is what an agent harness keyword-matches when deciding whether to invoke the skill. Lead with the canonical name, then pack 5-10 natural-language phrasings users (and agents) might reach for. End with a packed `Triggers - foo, bar, baz.` line.

Bias toward over-triggering. Harnesses tend to under-invoke skills.

## 4. The status line (where enforced)

Per-category. If `enforce_status: true` in the spec, every SKILL.md in that category needs a status line directly under the H1:

```markdown
# <Title>

Status: <emoji> <Kind> | Last <updated|tested>: YYYY-MM-DD
```

The emoji is part of the canonical format and pairs one-to-one with the kind. The validator rejects any other pairing. Pick kinds for your taxonomy (e.g. `Active 🟢`, `Stub ⚪`, `Runbook 🛠`, `Router 🗺`) in `categories.yaml`.

Categories without status enforcement are free-form. Add a status line voluntarily if it carries information (e.g. a digest skill noting when its data sources last changed shape), but the validator does not require one.

## 5. Required H2 sections (where enforced)

Per category and per status kind. If `required_sections.by_status` is set for a category, every SKILL.md of that status kind must contain the listed H2s. Section names match case-insensitively, leading and trailing whitespace ignored. Order is recommended but not enforced.

Use this for shaped categories where a missing section is a real problem (a runbook with no procedure, a router with no routing table). Free-form categories should leave this alone.
