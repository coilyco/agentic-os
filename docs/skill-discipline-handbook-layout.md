# Skill Discipline Handbook - Layout and Categories

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
