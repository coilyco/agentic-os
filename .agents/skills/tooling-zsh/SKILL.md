---
name: tooling-zsh
description: Zsh + bash are Kai's shells (zsh interactive, bash for ssh/non-interactive). One shared agentic-os/shell/common.sh sourced by both, thin zshrc/bashrc entries add prompt+completion. Use when drafting shell commands, editing agentic-os/shell/*, configuring PATH or prompt, fetching one AWS secret via ssm-get, debugging shell startup. Triggers - zsh, bash, zshrc, bashrc, .zshrc, .bashrc, $PATH, common.sh, ssm-get, vcs_info, prompt.
---

# Shell (zsh + bash)

zsh and bash share one core, so drafted commands work in both.

## Config location

Canonical files live at `~/projects/coilyco-flight-deck/agentic-os/shell/`, symlinked per host (`~/.zshrc -> shell/zshrc`, `~/.bashrc -> shell/bashrc`) by the ansible `shell` role. Windows runs both under Git Bash (zsh via MSYS `pacman -S zsh`).

Files:

- `common.sh` - the shared core (bash/zsh common subset). Sets env, per-OS PATH (via `uname -s`), aliases, git helpers, the `rg` wrapper, `ssm-get`, resolves `$PROJECTS_ROOT`, and auto-cds to `$AOS_STARTUP_DIR` with the projects root as its fallback. The env + PATH block runs once per terminal tree, gated by the exported `_SIREN_SHELL_ENV` guard; a nested shell inherits the env and skips it but still defines the aliases/functions.
- `zshrc` - zsh entry. Sources `common.sh`, then zsh-only: `compinit`, the `vcs_info` siren prompt, the `aterm` completion.
- `bashrc` - bash entry. Sources `common.sh`, then bash-only: completion, the `PROMPT_COMMAND` siren prompt.
- Host-local overrides: `~/.shellrc.local` (shared, sourced by `common.sh`), `~/.zshrc.local`, `~/.bashrc.local`. Untracked.

## Functions

Available in any interactive zsh:

- `gt`, `gush`, `..`, `...`, `....` - aliases
- `rg` - wrapper with `--hidden --glob '!.git' --glob '!*.svg' --glob '!.vscode'`
- `git-default-branch`, `git-pr-title`, `git-merge-default-branch`, `git-checkpoint`, `git-squash`, `gt-conflicts`
- `docker-bash <container-name>`, `rg-code <pattern>`, `pull-all-repos`, `count-lines`
- `ssm-get <name> [profile] [region]` - delegates to `scripts/ssm-get`, so scripts and agent tool calls reach the same implementation via `~/.local/bin`
- `github-token-load` - lazy. Call when something needs `$GITHUB_PERSONAL_ACCESS_TOKEN`; not eager on every shell start.


## Prompt

Two-line, siren-motif:

```
🕐 HH:MM:SS  🧜 user@host  📂 cwd  ⚓ branch ✨  💥 N
$
```

- ⚓ branch shows only inside a git repo.
- ✨ marks a dirty working tree.
- 💥 N shows only when the previous command exited non-zero.

Built on `vcs_info` + `PROMPT_SUBST`. No starship dependency.

## Editing

- Edit the files in `~/projects/coilyco-flight-deck/agentic-os/shell/` (the symlinks resolve there).
- Reload with `exec zsh` / `exec bash`, or open a new terminal tab.
- Errors at startup surface immediately. `zsh -x` / `bash -x` traces line-by-line if a function silently misbehaves.

## Common edits

- **Add an alias or function** - put it in `common.sh` so both shells get it. Keep it in the bash/zsh common subset (no `typeset -U`, no bash arrays).
- **Change PATH** - per-OS entries go in `common.sh`'s `case "$(uname -s)"` block; cross-platform env vars go in the env block above it.
- **Change the prompt** - zsh in `zshrc` (`PROMPT=`, `vcs_info`), bash in `bashrc` (`PROMPT_COMMAND`). They are intentionally separate.
- **Add a zsh-only completion / dispatcher** - a new zsh-only file sourced from `zshrc` after `compinit`, since a `compdef` call before it fails in every non-interactive zsh.

## Terminal

kitty hosts `aterm` windows on Mac and Linux, and Alacritty is the terminal on Windows. Their config lives in `kitty/` and `alacritty/`. Shell config and terminal config are intentionally separate - the same `shell/` tree should work under any POSIX terminal.
