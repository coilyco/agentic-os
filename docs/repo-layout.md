# Repo layout and residency

## Repo layout

Full breakdown of what lives where. Summary in [the README](../README.md).

## shell

One shared core, two thin per-shell entries, so bash and zsh run identical env, PATH, aliases, and functions.

- `shell/common.sh` - env, per-OS PATH, aliases, git helpers, `rg`, SSM loading, workspace-root discovery, and auto-cd via `$WARP_STARTUP_DIR` (default `$PROJECTS_ROOT`). Env runs once per terminal tree via `_SIREN_SHELL_ENV`.
- `shell/zshrc` - zsh entry, symlinked to `~/.zshrc`. Sources `common.sh`, then zsh-only bits: `compinit`, the `vcs_info` siren prompt, `warp.zsh`.
- `shell/bashrc` - bash entry, symlinked to `~/.bashrc`. Sources `common.sh`, then bash-only bits: completion, the `PROMPT_COMMAND` siren prompt.
- `shell/warp.zsh` - the zsh-only `warp` dispatcher + completion.
- Host-local overrides: `~/.shellrc.local` (shared, sourced by `common.sh`), `~/.zshrc.local`, `~/.bashrc.local`. Untracked.

## warp

- `warp/settings.toml` - Warp config. Vertical tabs, theme, font, custom secret-regex list, AI/agent toggles. `[account] is_settings_sync_enabled = false` so the repo wins over cloud sync.
- `warp/tab_configs/startup_config.toml` - default new-tab pane setup.

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

The JSON surface uses `aos.repository-residency.v1`. It retains the compiled
absolute projects root and each owner-qualified selection's source, scope,
reason, and provider details. Lines output contains only sorted
`owner/repository` identities for shell consumers.

AOS consumes Agent Compose's `agent-compose.repositories.v2` YAML contract and
temporarily accepts the preceding v1 JSON contract during host rollout. When
both files exist, YAML wins. AOS rejects unknown fields, unknown formats, unsafe
identities, duplicate or unsorted selections, incomplete provenance, relative
roots, and paths outside the compiled projects root. Missing or invalid plan
state fails closed. There is no embedded repository fallback and doctrine
source paths never become repository policy.

Native workspace projection and cleanup use the same compiled residency set.
The status-line tracker compares exact owner-qualified checkouts against it.
Infrastructure uses it to clone missing resident repositories and fetch
existing remotes without changing a worktree.

This host-residency projection is distinct from Ward's baked container
substrate. It also grants no role access by itself. Role selection is sealed
into the verified Agent Compose bundle that AOS adapts for Ward.

## See also

* [Native workspaces](native-agent-workspaces.md) - worktree projection and cleanup.
* [AOS context bundle](aos-context-bundle.md) - selected role handoff.
