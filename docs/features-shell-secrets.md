# Features: shell and secret handling

Cross-platform shell, terminal, and secret-handling capabilities.

## Cross-platform shell

One shared config core (`shell/common.sh`) sourced by both shells, so bash and zsh run identical setup. zsh (`shell/zshrc`) and bash (`shell/bashrc`) are thin entries that source the core and add only their own prompt + completion. Boots cleanly on Mac, Linux (kai-server), and Windows (Git Bash), picking the right per-OS PATH via `uname -s`. Carries identity, history, AWS defaults, git helpers, aliases, an `rg` wrapper, the SSM loader, and `WARD_LOCKDOWN_ROOT`.

The core's env + PATH block runs once per terminal tree, gated by an exported `_SIREN_SHELL_ENV` guard: a nested shell inherits the env and skips re-running brew/pyenv/PATH, while still defining aliases and functions (per-shell, never inherited). Env + PATH load for non-interactive shells too (scripts, ssh exec, the Claude Code Bash tool); prompt and completion are interactive-only. Both shells auto-cd a fresh interactive shell landing at `$HOME` or the projects root into the agentic-os repo (a gate-accepted work tree), or `~/projects` if that checkout is missing or the host is `AOS_HOST_CLASS=work`. Host-specific lines live in untracked `~/.shellrc.local` (shared) or `~/.{bash,zsh}rc.local` (per shell).

## Agent-CLI repo gate

`claude`, `codex`, and `opencode` are wrapped in shell functions that refuse to launch outside a git work tree. The gate lives in the shell - outside the agent - because a harness boundary can only be widened from outside the agent, never by the agent rewriting its own rules, and the shell is the one chokepoint shared across harnesses, so a single `git rev-parse --is-inside-work-tree` check covers them uniformly. Override deliberately with `AOS_ALLOW_ANY=1` for the intentional elevated-cwd cases (a session above the org dirs, or non-repo automation). The auto-cd lands fresh shells in agentic-os so the common case already satisfies the gate; a shell in a non-repo path still needs a `cd` or the override.

`ward-kdl agents <cli>` (the cli-guard launchers) execs the real binary directly, so a sibling `ward-kdl` shell function re-applies the gate before it runs - in the shell, not baked into ward. `agents ui` and `ops` are not launches and stay ungated.

## In-process AWS SSM secret loader

Pull secrets directly into the shell environment, never to disk. `ssm-load` reads every SecureString under the configured prefix and `load-env`s them. `ssm-get <name>` fetches a single value to stdout, a memory-only path.

## Cross-platform terminal

Single Warp config tree rendered into the host's Warp config dir - `~/.warp-preview/` on the Mac daily driver (Preview channel; Stable at `~/.warp/` is the fallback, selectable with `--channel`) and `~/.warp/` on Windows. The repo wins over cloud sync (`is_settings_sync_enabled = false`) so theme, font, vertical tabs, AI/agent toggles, and the secret-redaction regex list stay reproducible across hosts. The redaction surface covers IPv4/IPv6, MAC, AWS keys, GitHub tokens (every variant), Stripe, Firebase, JWT, OpenAI/Anthropic/Fireworks/Google keys, Slack tokens, phone numbers.

## GPG signing without disk-cached passphrases

`gpg-ssm` is a wrapper around `gpg` that pulls the signing passphrase from AWS SSM at sign time instead of caching it on disk. When the configured signing key is missing locally, it bootstraps `/coilysiren/gpg-secret-key` in memory before signing. Mac/Linux + Windows (`.cmd` shim for Git for Windows). Wire it in once with `git config --global gpg.program`.

## Install surface

[README.md](../README.md) carries per-OS install steps. `ward exec apply-shell-links` repairs the local shell entry symlinks (`~/.zshrc`, `~/.bashrc`, `gpg-ssm`) when a host drifts from the ansible-owned layout. Mac/Linux can also use plain `ln -sf`. Windows uses symlinks via Git Bash, which requires Developer Mode + `MSYS=winsymlinks:nativestrict`.
