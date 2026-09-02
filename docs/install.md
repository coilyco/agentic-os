# Install details

Quickstart is in [the README](../README.md). This covers per-host steps and the git gpg wiring.

## What the host install wires

The ansible `shell` role (in the infrastructure repo) symlinks, by `ansible_system`:

- `~/.zshrc` from `shell/zshrc` on all hosts
- `~/.bashrc` from `shell/bashrc` on Mac and Linux
- `~/.config/alacritty/alacritty.toml` from `alacritty/alacritty.toml` on Mac and Linux workstations
- `~/.local/bin/gpg-ssm` from `scripts/gpg-ssm` (Mac, Linux) or `scripts/gpg-ssm.cmd` (Windows)
- the Forgejo git credential helper from `scripts/git-credential-forgejo-ssm.*`
- the Forgejo Docker credential helper from `scripts/docker-credential-forgejo-ssm*`
- the `~/.local/bin` PATH helpers

A pre-existing regular shell, kitty, or Alacritty config is backed up to `<path>.bak`
before linking. The `claude-hooks` role runs `install-session-name.py` for the
provider-composed status line and SessionStart self-name hook. It is idempotent
and never clobbers a status line you set yourself. Agent Compose owns context
refresh through host convergence and native launch. The manual symlink fallback
is in [the README](../README.md).

`just apply-shell-links` is the local repair path for the same shell links, and it also owns the git settings whose value is one of those paths. It repoints stale symlinks, backs up pre-existing regular files, wires `gpg.program`, and supports `just apply-shell-links -- --check` for drift detection. On Windows it intentionally skips `~/.bashrc` and links each `.cmd` wrapper with its sibling Bash implementation; Git Bash popup launchers should not recreate that file.

It refuses to run from a native-session checkout, and writes the durable host home into `gpg.program` rather than `$HOME`. Both guard the same failure: a session-scoped path is durable enough to write into global config and gone by the time anything resolves it (agentic-os#1137).

Wire the helper into Git once:

```bash
git config --global credential.https://forgejo.coilysiren.me.helper \
  "!C:/Users/firem/.local/bin/git-credential-forgejo-ssm.cmd"
```

## Per-host notes

- **Linux (kai-server)** - Login-shell switch is on the operator: `chsh -s "$(command -v zsh)"`.
- **Windows (Git Bash)** - Install zsh via MSYS first: `pacman -S zsh`. Symlinks need either an elevated Git Bash or Settings -> Privacy and Security -> For developers -> Developer Mode toggled on.

## Wire gpg-ssm into git

Run from the canonical checkout, never a native session:

```bash
just apply-shell-links
```

That links the wrapper and sets `gpg.program` to it in one step, on every OS. Verify with `git log -1 --format=%G?` on a signed commit, which returns `G`.

## Features: shell and secret handling

## Cross-platform shell

One shared core (`shell/common.sh`) owns shell setup. Zsh (`shell/zshrc`) and Bash (`shell/bashrc`) source it, then add their prompt and completion. The rendered Windows PowerShell profile reads its marked blocks and derives Windows paths natively, without launching Bash: `shared-environment` carries exports and stays declarative, and `shared-environment-clear` carries bare `unset NAME` lines, so a name Unix clears is cleared on Windows too (agentic-os#849). Setup boots cleanly on Mac, Linux (kai-server), and Windows. It carries identity, history, AWS defaults, `WARD_LOCKDOWN_ROOT`, git helpers, aliases, an `rg` wrapper, and on-demand SSM reads.

The core's env + PATH block runs once per terminal tree, gated by an exported `_SIREN_SHELL_ENV` guard: a nested shell inherits the env and skips re-running brew/pyenv/PATH, while still defining aliases and functions (per-shell, never inherited). Env + PATH load for non-interactive shells too (scripts, ssh exec, the Claude Code Bash tool). Prompt and completion are interactive-only. The core exports `PROJECTS_ROOT`, honoring a set value, then `~/projects`, then deriving the workspace umbrella from the AOS checkout. Fresh shells at `$HOME` or an AOS session root auto-cd to `$AOS_STARTUP_DIR` or `$PROJECTS_ROOT`; nested and checkout shells stay put. Host-specific lines live in untracked `~/.shellrc.local` (shared) or `~/.{bash,zsh}rc.local` (per shell).

## Agent-CLI compose preflight

`claude`, `codex`, `goose`, and `opencode` are wrapped in shell functions that
launch through Agent Compose, so doctrine, skills, and native MCP registries
converge before the real binary starts. They retain inferred-role behavior.
`acompose <role> <harness> [args...]` selects one caller-assigned native bundle.
Without Agent Compose, the wrappers fall back to the harness.

Supported AOS binaries also create a leased
[native session workspace](native-agent-workspaces.md) and clean recoverable
predecessors. Explicit role launches use the same workspace without a container.

## On-demand AWS SSM secret reads

`ssm-get <name> [profile] [region]` fetches one decrypted parameter to stdout
without writing it to disk. The shell exposes no bulk parameter-tree loader and
does not populate secret environment variables at startup.

The implementation is `scripts/ssm-get`, reaching `PATH` through the same
`~/.local/bin` link as the other wrappers. The shell function of the same name
delegates to it rather than carrying its own copy: a function alone is
invisible to scripts and agent tool calls, which then restate the raw
`aws ssm get-parameter` call (agentic-os#1308). The function prefers the
converged link and falls back to the checkout, so a host that has not run
`just apply-shell-links` yet still resolves it.

The integrated `aos --warded` launch uses the same on-demand posture for its
deployment-owned Forgejo broker credential. AOS resolves the value through the
host AWS session only when `FORGEJO_TOKEN` is absent, passes it to Ward's
privileged broker launch environment, and never projects it into the agent
harness. See the [AOS to Ward credential handoff](aos-cluster-access.md).

## Cross-platform terminal

kitty supplies the portable Sombra rendering baseline for `aterm` windows. Mac
and Linux workstations symlink the live kitty config to the canonical AOS file.
kitty does not ship on Windows, so **`aterm` is Mac and Linux only** and is not
built or packaged for Windows (agentic-os#1264). Alacritty keeps the Windows
terminal baseline for everything else: infrastructure renders Git Bash as
Alacritty's direct shell and keeps the terminal free of an intermediate
multiplexer.

## GPG signing without disk-cached passphrases

`gpg-ssm` is a wrapper around `gpg` that pulls the signing passphrase from AWS SSM at sign time instead of caching it on disk. When the configured signing key is missing locally, it bootstraps `/coilysiren/gpg-secret-key` in memory before signing. Mac/Linux + Windows (`.cmd` shim for Git for Windows). Wire it in once with `just apply-shell-links`.

## Install surface

[README.md](../README.md) carries per-OS install steps. `just apply-shell-links` repairs shell links, `gpg-ssm`, and the Forgejo git credential helper. Windows skips `~/.bashrc` so Git Bash hook launchers cannot recreate stale state. Mac/Linux can also use plain `ln -sf`. Windows symlinks need Developer Mode or elevation.
