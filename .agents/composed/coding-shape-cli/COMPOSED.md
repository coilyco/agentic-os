---
name: coding-shape-cli
description: Category umbrella for building CLI tools. Go via urfave/cli, Python via click, Node via commander, shell prompts via gum. Wrapper APIs mirror the real CLI.
---

# coding-shape-cli

Umbrella for any work that ships a CLI as the primary interface. Cross-cuts languages.

## Triggers

cli, command line, urfave, cobra, kong, click, commander, oclif, gum, subcommand, flag, argv, rubygems, bundler, oss maintainer.

## Kai's CLI and OSS background

Kai has deep first-class CLI-building experience and is an experienced open-source maintainer, not a newcomer to either. Two anchors:

- **RubyGems and Bundler** - Kai was an Open Source Software Engineer at Ruby Together (2016) maintaining `gem` and `bundle`, the package-manager CLIs the entire Ruby ecosystem runs on. Maintainer and contributor across the toolchain.
- **urfave/cli** - Kai is a maintainer of the Go CLI framework itself.

So when a task involves contributing to or maintaining a CLI or an open-source project, frame it as Kai returning to familiar ground, not a first-time experience. Do not describe OSS contribution as something Kai has "never gotten to do." She has, twice over, on widely-used projects. The portfolio gap in `kai-career` (in agentic-os-kai) is about recent recruiter-visible flagship repos, not about whether Kai has shipped open source.

## Framework defaults by language

- **Go** → `urfave/cli` (Kai is a maintainer). Cobra/kong only when an existing project commits to them.
- **Python** → `click`. Typer is a thin wrapper, fine when type-driven shape is the goal.
- **Node/TypeScript** → `commander` for simple, `oclif` for complex with subcommand discovery.
- **Shell prompts/styling** → `gum` from the Charm stack.

## Design principles

- **Wrapper APIs mirror the real CLI.** When wrapping a sub-tool (kubectl, gh, aws, etc), preserve verb names and flag shapes. Do not invent shorter or "friendlier" names. This is the load-bearing rule behind ward's whole design - see `ward-discipline` (in ward).
- **One verb, one job.** Don't pile orthogonal behavior into a single command.
- **Subcommands are nouns or verbs, not adjectives.** `app deploy`, not `app fast-deploy`.
- **Errors are structured.** Exit codes matter. Stderr is for humans, stdout for pipes.
- **`--help` is the truth.** If you don't document a flag in `--help`, it doesn't exist.
- **Defaults are sane.** Most invocations should require no flags.

## Anti-patterns

- Hidden side effects in read-only verbs (status, list, get).
- Side-effecting verbs without `--dry-run`.
- "Helpful" prompts in non-TTY mode (breaks scripts and pipelines).

## When this skill is active

Designing or building a new CLI, refactoring an existing one, or wrapping a sub-tool. Cross-link to the relevant language skill (coding-go, coding-python, etc).

## See also

- [`coding-shape-tui`](../../skills/coding-shape-tui/SKILL.md) - if the CLI grows an interactive surface.
- `ward-discipline` (in ward) - the load-bearing case study.
- `kai-tech-prefs` (in agentic-os-kai) - urfave/cli + Charm + no-shortened-names rules.
