# Features: shell and secret handling

Cross-platform shell, terminal, and secret-handling capabilities.

## Cross-platform shell

One shared core (`shell/common.sh`) owns shell setup. Zsh (`shell/zshrc`) and Bash (`shell/bashrc`) source it, then add their prompt and completion. The rendered Windows PowerShell profile reads its marked environment exports and derives Windows paths natively, without launching Bash. Setup boots cleanly on Mac, Linux (kai-server), and Windows. It carries identity, history, AWS defaults, `WARD_LOCKDOWN_ROOT`, git helpers, aliases, an `rg` wrapper, and on-demand SSM reads.

The core's env + PATH block runs once per terminal tree, gated by an exported `_SIREN_SHELL_ENV` guard: a nested shell inherits the env and skips re-running brew/pyenv/PATH, while still defining aliases and functions (per-shell, never inherited). Env + PATH load for non-interactive shells too (scripts, ssh exec, the Claude Code Bash tool). Prompt and completion are interactive-only. The core exports `PROJECTS_ROOT`, honoring a set value, then `~/projects`, then deriving the workspace umbrella from the AOS checkout. Both shells auto-cd from `$HOME` to an existing `$WARP_STARTUP_DIR`, or `$PROJECTS_ROOT` when unset. The same operator-local override drives Warp's new-tab directory. Host-specific lines live in untracked `~/.shellrc.local` (shared) or `~/.{bash,zsh}rc.local` (per shell).

## Agent-CLI compose preflight

`claude`, `codex`, `goose`, and `opencode` are wrapped in shell functions that
launch each harness through `acompose -- <cli>`, so
agent-compose converges doctrine, skills, and native MCP registries before the
real binary starts. A host without the opt-in agent-compose product
falls back to the real binary. The wrappers apply in any working directory,
including paths outside a git work tree.

## On-demand AWS SSM secret reads

`ssm-get <name> [profile] [region]` fetches one decrypted parameter to stdout
without writing it to disk. The shell exposes no bulk parameter-tree loader and
does not populate secret environment variables at startup.

The integrated `aos --warded` launch uses the same on-demand posture for its
deployment-owned Forgejo broker credential. AOS resolves the value through the
host AWS session only when `FORGEJO_TOKEN` is absent, passes it to Ward's
privileged broker launch environment, and never projects it into the agent
harness. See the [AOS to Ward credential handoff](aos-ward-credentials.md).

## Cross-platform terminal

Alacritty supplies the portable Sombra rendering baseline. On Windows,
infrastructure renders Git Bash as Alacritty's direct shell and keeps the
terminal free of an intermediate multiplexer.

Transitional Warp config still renders into `~/.warp-preview/` on Mac or
`~/.warp/` on Windows. Repo state disables cloud sync and owns its theme,
native tabs, agent toggles, and redaction rules for network identifiers,
credentials, tokens, keys, JWTs, and phone numbers.

## GPG signing without disk-cached passphrases

`gpg-ssm` is a wrapper around `gpg` that pulls the signing passphrase from AWS SSM at sign time instead of caching it on disk. When the configured signing key is missing locally, it bootstraps `/coilysiren/gpg-secret-key` in memory before signing. Mac/Linux + Windows (`.cmd` shim for Git for Windows). Wire it in once with `git config --global gpg.program`.

## Install surface

[README.md](../README.md) carries per-OS install steps. `ward exec apply-shell-links` repairs shell links, `gpg-ssm`, and the Forgejo git credential helper. Windows skips `~/.bashrc` so Git Bash hook launchers cannot recreate stale state. Mac/Linux can also use plain `ln -sf`. Windows symlinks need Developer Mode or elevation.
