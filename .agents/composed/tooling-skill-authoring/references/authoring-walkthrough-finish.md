# Authoring walkthrough: structure, style, validate, wrap-up

Continues [`authoring-walkthrough.md`](authoring-walkthrough.md).

## Anatomy of a skill

```
skill-name/
├── SKILL.md (required)
│   ├── YAML frontmatter (name, description required)
│   └── Markdown instructions
├── scripts/        - executable code for deterministic / repetitive tasks
├── references/     - docs loaded into context as needed
├── assets/         - files used in output (templates, snippets)
└── results/        - dated writeups for skills that run periodically
```

## Progressive disclosure

Skills load in three tiers:

1. **Metadata** (name + description) - always in context (~100 words).
2. **SKILL.md body** - in context whenever the skill triggers (cap in `check_documentation_layout.py`, same as all Markdown).
3. **Bundled resources** - pulled in as needed. Scripts can execute without their source loading.

Patterns:

* Reference files clearly from SKILL.md with guidance on when to read them.
* For large reference files (over 300 lines), include a table of contents.
* For domain-spanning skills, organize by variant and route from the SKILL.md.

## Writing style

Imperative voice. Explain the why behind each instruction. Today's models respond to reasoning better than to ALL-CAPS MUSTs. If you find yourself writing rigid scaffolding, that's a yellow flag - reframe and explain instead.

Voice-match the personal-OS AGENTS.md: no em-dashes, no italics, no semicolons in prose, no tables. Bullet lists in the shape `* anchor - tag1 / tag2 - detail`.

## Validate before commit

```sh
python3 scripts/validate_skills.py <skill-name>
python3 scripts/check_dead_links.py .agents/skills/<skill-name>/
```

Both run automatically in pre-commit. Run them by hand during iteration to keep the feedback loop tight.

## Wrap-up

* Refresh the applicable harness projection. The fleet refresh updates Claude's `.claude/skills/<name>` surface and canonical catalog links. Bare agent-compose convergence updates Codex's `~/.agents/skills/<name>` user surface.
* Restart your harness so the loader picks up the new skill.
* File the GitHub issue first if you haven't (every commit closes one).
* Commit, push to main, the commit message closes the issue.
