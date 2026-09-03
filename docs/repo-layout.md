# Repo layout, residency, and maps

## Repo layout

Full breakdown of what lives where. Summary in [the README](../README.md).

## shell

One shared core, two thin per-shell entries, so bash and zsh run identical env, PATH, aliases, and functions.

- `shell/common.sh` - env, per-OS PATH, aliases, git helpers, `rg`, SSM loading, workspace-root discovery, and auto-cd via `$AOS_STARTUP_DIR` (default `$PROJECTS_ROOT`). Env runs once per terminal tree via `_SIREN_SHELL_ENV`.
- `shell/zshrc` - zsh entry, symlinked to `~/.zshrc`. Sources `common.sh`, then zsh-only bits: `compinit`, the `vcs_info` siren prompt, and the `aterm` completion.
- `shell/bashrc` - bash entry, symlinked to `~/.bashrc`. Sources `common.sh`, then bash-only bits: completion, the `PROMPT_COMMAND` siren prompt.
- Host-local overrides: `~/.shellrc.local` (shared, sourced by `common.sh`), `~/.zshrc.local`, `~/.bashrc.local`. Untracked.

- `aos-say/` - speech helper client and relay.

## karabiner

- `karabiner/*.json` - Karabiner-Elements complex-modification assets. `control-escape-backtick.json`: Control+Escape -> backtick. `swap-option-command.json`: left_option <-> left_command on the external keyboard (`device_if`). `rdp-keyboard-capture.json`: command -> control while a Remote Desktop window is frontmost (`frontmost_application_if`), so Cmd shortcuts reach Windows as Ctrl.

Setup, after `brew install --cask karabiner-elements`:

1. Symlink each asset into `~/.config/karabiner/assets/complex_modifications/`.
2. Open Karabiner-Elements once and approve the system prompts (driver extension, Input Monitoring).
3. Complex Modifications -> Add rule, then enable the agentic-os rules.

## scripts

- `check-aws-config.py` - reject the `[profile default]` trap in `~/.aws/config` that surfaces later as a cryptic `NoRegion` from SSM/S3.
- `gpg-ssm` / `gpg-ssm.cmd` - GPG signing wrapper that pulls the passphrase from AWS SSM at `/coilysiren/gpg-passphrase` and bootstraps `/coilysiren/gpg-secret-key` when the configured signing key is missing locally. The `.cmd` is a bash.exe shim Git for Windows needs.
- `install-session-name.py` - idempotently wire the provider composer as Claude
  Code's status line and `docker/dev-base/session-name.sh` as its SessionStart
  hook, repointing a host still wired to the retired `agent-name.sh`.

## agentic_os

- `agentic_os/pre_commit/` - Python entry points for the hook suite exposed through `.pre-commit-hooks.yaml` and `[project.scripts]`.
- `agentic_os/generators/` - offline generators for managed blocks, repo-pointer skills, seed-skill data, and agent-compose output.
- `agentic_os/config.py` - shared repo config loader for hook opt-outs, excludes, and workspace scans. Also asks git what the repo carries, so a walking hook skips [build output](build-output-is-not-content.md) unasked.
- `agentic_os/seed_skills_data.py` - generated seed-skill table shipped with the package so consumer hooks run offline.

## skills

`.agents/skills/` - SKILL.md docs for the configs that live here (`tooling-zsh`, `tooling-gpg-ssm`, and the cross-repo skills). agentic-os-kai's skill mount walks this dir as a peer skill source, symlinking each entry into `~/.claude/skills/`. Co-located with the configs they describe so they don't drift.

## Repository residency

Agent Compose owns repository policy and emits the strict
`~/.agent-compose/repository-plan.yaml`. AOS validates that machine contract and
exposes its host-residency projection without parsing `.agents/roles.kdl`:

```sh
aos repositories --format lines
aos repositories --format json
```

The JSON surface uses `aos.repository-residency.v1` and retains the compiled
projects root plus each selection's source, scope, reason, and provider. Lines
output is sorted `owner/repository` identities for shell consumers.

AOS consumes Agent Compose's `agent-compose.repositories.v2` YAML and
temporarily accepts the preceding v1 JSON during host rollout, YAML winning
when both exist. It rejects unknown fields and formats, unsafe identities,
duplicate or unsorted selections, incomplete provenance, relative roots, and
paths outside the compiled root. Missing or invalid plan state fails closed,
with no embedded fallback, and doctrine source paths never become policy.

Native workspace projection and cleanup, the status-line tracker, and
infrastructure's clone-and-fetch all read that same compiled set. It is
distinct from Ward's baked container substrate and grants no role access on its
own: role selection is sealed into the verified bundle AOS adapts for Ward.

## Running tasks

`.ward/ward.yaml` and the [`justfile`](../justfile) carry the same verbs. Ward
is out-of-band flight control, so a clone with no ward on `PATH` still runs its
own tasks through `just`. Neither is authoritative over the other and CI uses
ward.

## Repo maps

Compact starting points for cross-repo traces, one per question that spans
repositories. They are search-first rather than frozen inventories: each names
the commands that rediscover the current files when a surface shifts. Read the
entry points, then run the first check. If the same repo also exists under
`/substrate`, `/workspace/agentic-os` is authoritative.

**Ward integration** - a change crossing AOS, Ward, agent-compose, or AOSguard.
Run `rg -n "context-bundle|--warded|--composed|--guarded" aos docs`, `ward
doctor`, `aosguard ops forgejo describe`. Ownership splits: `.agents/roles.kdl`
owns behavioral composition, `.agents/harness-launch-profiles.yaml` owns
role-to-default-agent mapping, `.ward/ward.yaml` owns repository commands and
the deployment image, agent-compose owns named seats and pronouns, and Ward and
AOSguard own their separate surfaces.

**Container startup and broker dispatch** - a run that starts wrong, mounts the
wrong root, or has stale dispatch wiring. Run `rg -n
"AOS_REPO_ROOT|ward agent|entrypoint" docker docs .ward`, then `just
dev-base-build` as the first check. `ward agent` is the runtime entry point.

**Forgejo ops surface discovery** - what Forgejo surface exists here now. Run
`aosguard ops forgejo describe` first, then `--help` and `rg -n "aosguard ops
forgejo" docs .umbra`. Prefer runtime help over guessing a verb, and update
[aosguard](../.agents/skills/tooling-aosguard/references/aosguard.md) when the surface
changes.

**Ward PR workflow and director merge** - PR lifecycle, director merge, or
burn-down. Run `rg -n "pull-request-and-merge|director merge|WARD-OUTCOME"
.ward docs` and `ward agent director --help`. Burndown containment is in
[ward-specs](ward-specs.md).
