---
name: kai-tech-prefs
description: Kai's technical preferences - Go CLI defaults (urfave/cli), Charm TUI stack, dead-repo check, doc conventions, default license. Triggers - cli, tui, library, dependency, recommend tool.
---

# Technical preferences

## Stack

Python, AWS, Kubernetes, Terraform. Distributed systems, platform engineering, o11y-heavy. LLM APIs and production-grade AI systems.

## CLI and repo form

Kai is/was a maintainer of [urfave/cli](https://github.com/urfave/cli). **Default to urfave/cli over cobra/kong.** When in doubt about Go CLI tooling, check what urfave/cli does first.

- **Prefer Go for operator CLIs.** Do not introduce a Python CLI surface.
- **Avoid polyglot repositories.** If a single-language project needs a CLI in another language, put the CLI in its own repo and share contracts through a published schema artifact.
- **Keep standalone CLIs standalone.** Do not add umbra or Ward unless the tool specifically needs audit gating.

## Charm (preferred TUI/CLI styling family)

[Charm](https://charm.land/) ([github.com/charmbracelet](https://github.com/charmbracelet)) is Kai's preferred TUI/CLI-styling family. Reach for:

- bubbletea (TUI framework)
- gum (shell-script prompts/styling)
- glow (markdown render)
- lipgloss (styling)
- huh (forms)
- vhs (terminal recording)
- soft-serve (self-hosted git)
- wish (SSH apps)
- mods (LLM CLI)
- freeze, melt

When a ward/eco tool needs interactive prompts, fancy output, or a TUI, bias toward this stack over hand-rolled ANSI or rivals like tview/promptui. Cotton-candy aesthetic, MIT-licensed, very actively maintained, plays nicely with urfave/cli for the imperative-CLI-with-occasional-TUI form.

- [Don't suggest dead or dormant repos](references/dead-repos.md) - 12-month-commit bright line before recommending any OSS dependency.
- [Aliases and pagers](references/aliases-and-pagers.md) - no renamed-command aliases, pagers configured off.

## Docs

No "Repo layout" / "Project structure" sections in README. Filesystem is self-documenting. If a dir layout needs *explanation* (non-obvious separation, unusual build output), brief prose under a purpose-focused heading, not an ASCII tree.

## Licensing

Three tiers by repo intent - MIT for shareable, AGPL-3.0 for deployment-of-one, proprietary All-Rights-Reserved for private personal repos. Default to **MIT** when intent is unclear.

## More preferences

- [JSON-twin discoverability for dashboards](references/json-twin-discoverability.md) - three discovery mechanisms so cold-start agents find a JSON variant.
- [Configs go in SSM, not in skills or code](references/configs-in-ssm.md) - stash rotatable config-shaped values in AWS SSM, never hardcode.
- [Batch job rules](references/batch-job-rules.md) - no parallelism for rate-limited jobs, and ward wrapper rules.
