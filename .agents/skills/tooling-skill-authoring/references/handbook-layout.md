# Handbook: Layout and categories

## 1. Layout

```
<personal-os-repo>/
├── .agents/skills/
│   ├── <personal-prefix>-<topic>/                            # operating-context rules
│   ├── daily-<topic>/                          # cron'd inbox routines
│   ├── ops-social-gws-<verb>/                  # Gmail family
│   ├── ops-social-google-<topic>/              # Calendar family
│   ├── ops-eng-sentry-<topic>/                 # Sentry review playbooks
│   ├── ops-investigation-<topic>/              # per-topic investigation guides
│   ├── ops-investigation/                      # investigation router
│   ├── <ops-investigation-meta>/              # meta-discipline router
│   ├── gaming-eco-<topic>/                     # Eco modding
│   ├── gaming-steam/                           # Steam library
│   ├── gaming-factorio/                        # placeholder
│   ├── writing-<topic>/                        # prose / voice / issue authoring
│   ├── home-<system>/                          # smart-home control
│   ├── tooling-<topic>/                        # agent-ecosystem meta
│   ├── vault-<topic>/                          # Obsidian vault tooling
│   ├── categories.yaml                         # machine-readable spec (root)
│   └── skill-creator/                          # this skill (handbook + templates)
│       ├── SKILL.md                            # entrypoint, points at this handbook
│       ├── references/
│       │   ├── handbook.md                     # YOU ARE HERE
│       │   └── authoring-walkthrough.md        # how to draft a skill
│       └── templates/                          # one template per shaped category
├── .agents/composed/
│   └── <role-scoped-topic>/
│       └── COMPOSED.md                         # promoted only for selected roles
├── .agents/roles.kdl                           # composed-skill allowlists
├── scripts/
│   ├── check-em-dashes.py                      # local voice-rule hook
│   └── leak-check.py                           # local private-string denylist
└── .pre-commit-config.yaml                     # subscribes to coilyco-flight-deck/agentic-os hooks + local hooks
```

Ordinary sources live only under `.agents/skills/`. Role-scoped sources live
only under `.agents/composed/` and use `COMPOSED.md` so a harness cannot
discover them before composition. No source lives inside another source.
Agent-compose promotes selected composed entrypoints to `SKILL.md`.

## 2. Categories

The configured prefix families, exact-name skills, and how to pick a category
for a new source live in
[`handbook-categories.md`](handbook-categories.md).
