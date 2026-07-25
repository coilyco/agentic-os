# Features: shell and secret handling

Cross-platform shell, terminal, and secret-handling capabilities.

## Cross-platform shell

One shared core (`shell/common.sh`) owns shell setup. Zsh (`shell/zshrc`) and Bash (`shell/bashrc`) source it, then add their prompt and completion. The rendered Windows PowerShell profile reads its marked environment exports and derives Windows paths natively, without launching Bash. Setup boots cleanly on Mac, Linux (kai-server), and Windows. It carries identity, history, AWS defaults, commit-addressed `WARD_CONFIG_REF`, `WARD_LOCKDOWN_ROOT`, git helpers, aliases, an `rg` wrapper, and the SSM loader.

The core's env + PATH block runs once per terminal tree, gated by an exported `_SIREN_SHELL_ENV` guard: a nested shell inherits the env and skips re-running brew/pyenv/PATH, while still defining aliases and functions (per-shell, never inherited). Env + PATH load for non-interactive shells too (scripts, ssh exec, the Claude Code Bash tool). Prompt and completion are interactive-only. The core exports `PROJECTS_ROOT`, honoring a set value, then `~/projects`, then deriving the workspace umbrella from the AOS checkout. Both shells auto-cd from `$HOME` to an existing `$WARP_STARTUP_DIR`, or `$PROJECTS_ROOT` when unset. The same operator-local override drives Warp's new-tab directory. Host-specific lines live in untracked `~/.shellrc.local` (shared) or `~/.{bash,zsh}rc.local` (per shell).

## Agent-CLI repo gate

`claude`, `codex`, and `opencode` are wrapped in shell functions that refuse to launch outside a git work tree. The gate lives in the shell - outside the agent - because a harness boundary can only be widened from outside the agent, never by the agent rewriting its own rules, and the shell is the one chokepoint shared across harnesses, so a single `git rev-parse --is-inside-work-tree` check covers them uniformly. Override deliberately with `AOS_ALLOW_ANY=1` for the intentional elevated-cwd cases (a session above the org dirs, or non-repo automation). Fresh shells land at the workspace root by default. Pointing `WARP_STARTUP_DIR` at a checkout lets an agent CLI launch without a separate `cd`.

`ward-kdl agents <cli>` (the cli-guard launchers) execs the real binary directly, so a sibling `ward-kdl` shell function re-applies the gate before it runs - in the shell, not baked into ward. `agents ui` and `ops` are not launches and stay ungated.

## In-process AWS SSM secret loader

Pull secrets directly into the shell environment, never to disk. `ssm-load` reads every SecureString under the configured prefix and `load-env`s them. `ssm-get <name>` fetches a single value to stdout, a memory-only path.

## Cross-platform terminal

Alacritty supplies the portable Sombra rendering baseline. Zellij supplies the
long-lived workspace, vertical tabs, panes, and session serialization. The
[workspace walkthrough](zellij-workspace.md) carries the recovery behavior and
daily controls.

Transitional Warp config still renders into `~/.warp-preview/` on Mac or
`~/.warp/` on Windows. Repo state disables cloud sync and owns its theme,
vertical tabs, agent toggles, and redaction rules for network identifiers,
credentials, tokens, keys, JWTs, and phone numbers.

## GPG signing without disk-cached passphrases

`gpg-ssm` is a wrapper around `gpg` that pulls the signing passphrase from AWS SSM at sign time instead of caching it on disk. When the configured signing key is missing locally, it bootstraps `/coilysiren/gpg-secret-key` in memory before signing. Mac/Linux + Windows (`.cmd` shim for Git for Windows). Wire it in once with `git config --global gpg.program`.

## Install surface

[README.md](../README.md) carries per-OS install steps. `ward exec apply-shell-links` repairs shell links, `gpg-ssm`, and the Forgejo git credential helper. Windows skips `~/.bashrc` so Git Bash hook launchers cannot recreate stale state. Mac/Linux can also use plain `ln -sf`. Windows symlinks need Developer Mode or elevation.
