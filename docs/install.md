# Install details

Quickstart is in [the README](../README.md). This covers per-host steps and the git gpg wiring.

## What the host install wires

The ansible `shell` role (in the infrastructure repo) symlinks, by `ansible_system`:

- `~/.zshrc` from `shell/zshrc` on all hosts
- `~/.bashrc` from `shell/bashrc` on Mac and Linux
- `~/.local/bin/gpg-ssm` from `scripts/gpg-ssm` (Mac, Linux) or `scripts/gpg-ssm.cmd` (Windows)
- the Forgejo git credential helper from `scripts/git-credential-forgejo-ssm.*`
- the Forgejo Docker credential helper from `scripts/docker-credential-forgejo-ssm*`
- the `~/.local/bin` PATH helpers

A pre-existing regular `~/.zshrc` / `~/.bashrc` is backed up to `<path>.bak` before linking. The `claude-hooks` role then runs `install-agent-name.py` (status line + SessionStart self-name hook) and `install-session-pulse.py` (SessionStart hook surfacing `~/.cache/agentic-os/session-pulse.yaml` if a producer wrote one). Both are idempotent and never clobber a status line you set yourself. The manual symlink fallback (no ansible) is in [the README](../README.md).

`ward exec apply-shell-links` is the local repair path for the same shell links. It repoints stale symlinks, backs up pre-existing regular files, and supports `ward exec apply-shell-links -- --check` for drift detection. On Windows it intentionally skips `~/.bashrc` and links each `.cmd` wrapper with its sibling Bash implementation; Git Bash popup launchers should not recreate that file.

Wire the helper into Git once:

```bash
git config --global credential.https://forgejo.coilysiren.me.helper \
  "!C:/Users/firem/.local/bin/git-credential-forgejo-ssm.cmd"
```

## Per-host notes

- **Linux (kai-server)** - Login-shell switch is on the operator: `chsh -s "$(command -v zsh)"`.
- **Windows (Git Bash)** - Install zsh via MSYS first: `pacman -S zsh`. Symlinks need either an elevated Git Bash or Settings -> Privacy and Security -> For developers -> Developer Mode toggled on.

## Wire gpg-ssm into git

After the gpg-ssm symlink lands:

```bash
git config --global gpg.program "$HOME/.local/bin/gpg-ssm"      # Mac, Linux
git config --global gpg.program "$HOME/.local/bin/gpg-ssm.cmd"  # Windows
```
