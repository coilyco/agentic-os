<!-- maintained under agentic-os -->
# agentic-os

![Sombra hacking skull](static/wallpaper.jpg)

Cross-platform shell and terminal setup plus cross-repo pre-commit hooks for
coilysiren/* repos. Zsh runs on Mac and Linux, with Bash through Git for
Windows. Alacritty provides the default direct terminal, while the transitional
Warp configuration remains available.

## Layout

- `shell/` - shared `common.sh` plus thin `zshrc` + `bashrc`, so bash and zsh match. `warp.zsh` is the zsh-only Warp dispatcher.
- `alacritty/` - portable Sombra appearance and terminal security defaults, with host preferences left to the local wrapper.
- `agent-terminal/` - static agent-compose identity branding for one Alacritty director window.
- `warp/` - transitional Warp config (`settings.toml`, `tab_configs/`) plus the `ward exec warp` Go module.
- `aos/` - the Go composition root for standalone and Ward-governed agent launches.
- `aos-say/` - the `ward exec aos-say` Go module for the speech helper client and relay.
- `karabiner/` - Karabiner-Elements complex modification assets (`brew install --cask karabiner-elements`), symlinked into the local Karabiner config tree.
- `scripts/` - portable utilities (gpg-ssm wrapper, agent-name + session-pulse hooks, aws-config lint).
- `.agents/skills/` - ordinary `SKILL.md` sources that every composed role can discover.
- `.agents/composed/` - role-scoped `COMPOSED.md` sources that agent-compose promotes only for allowlisted roles.
- `agentic_os/` - packaged hooks, generators, shared config/data, plus the hygiene guardrails that back the pre-commit suite.

Full breakdown: [docs/repo-layout.md](docs/repo-layout.md).

## Install

Host config is converged by Ansible (rollout lives in infrastructure, per [AGENTS.md](AGENTS.md)). Manual fallback:

```bash
ward exec apply-shell-links
```

Equivalent links on Mac and Linux:

```bash
ln -sf "$PWD/shell/zshrc"  ~/.zshrc      # both source shell/common.sh
ln -sf "$PWD/shell/bashrc" ~/.bashrc
ln -sf "$PWD/scripts/gpg-ssm" ~/.local/bin/gpg-ssm
ward exec warp apply                     # warp config
```

On Windows, `ward exec apply-shell-links` manages `~/.zshrc` and the `gpg-ssm.cmd`
shim only. It also links the Forgejo git credential helper; Git Bash popup shells
should not recreate `~/.bashrc`.

Agent self-name + session-pulse hooks, per-host steps, and gpg wiring: [docs/install.md](docs/install.md).

## aos CLI

AOS composes independent Ward, agent-compose, and aosguard capabilities behind
one launch surface:

```bash
aos --agent codex --role engineer --warded --composed --guarded -- owner/repo#267
```

AOS translates the shared role and selected capabilities. Ward remains the
fixed workflow and container lifecycle owner, agent-compose remains the context producer,
and cli-guard/specgen remains the guarded-tool generator. The role name selects
context in each layer and never unions authority.

The original standalone composed-container command remains available:

```bash
aos --role engineer acompose -- codex
```

That standalone path projects the resolved host mcporter inventory into the
ephemeral agent home. When configured HTTP MCP endpoints resolve into the
tailnet, AOS attaches the container to the shared tailnet network and bridges
those endpoints through the standing proxy without invoking Ward. See the
[standalone connectivity contract](docs/aos-standalone-connectivity.md).

For a bounded role check-in, AOS owns the agent's non-interactive defaults:

```bash
aos --role engineer --agent codex acompose-checkin
```

The released binary also resolves the committed model-opaque default for a
role-intent lane:

```bash
aos --role director harness-default --intent strategic-planning
aos --role director lane-default --intent strategic-planning
```

Ward is not part of that standalone path. See the
[launch and handoff contract](docs/aos-cli.md).
Homebrew and Scoop install both `aos` and `aosguard`. Direct release binaries and
the paired native update path are documented in the [CLI release walkthrough](docs/aos-cli-release.md).

## Secrets pattern

Fetch one parameter only when a command needs it:

```bash
ssm-get /eco/server-api-token
```

`ssm-get` prints the decrypted value to stdout without writing it to disk. The
AWS profile defaults to `default`; pass a profile and region as the second and
third arguments. The bulk shell-environment exporter and the legacy
cleartext-on-disk dump (`~/.cache/ssm-env.sh`) are removed.

## agent-compose

Opt-in tooling that composes global agent context and symlinks each harness load point to it. Sources are shared unless optional `harnesses` frontmatter selects a harness-specific slice. Agent-compose embeds the canonical personalities. AOS publishes the [capability provider](docs/personality-provider.md): ordinary skills, [role-composed skills](docs/role-composed-skills.md), and the public [harness capability registry](docs/harness-selection.md). Host composition stays inert until `~/.config/agent-compose/agent-compose.yaml` exists.

**Prior art.** The idea is fresh in the agentic space but well-trodden in config management, and agent-compose is best understood as **Hiera-for-agent-doctrine, deployed Stow-style, scoped chezmoi-style**:

- **[GNU Stow](https://www.gnu.org/software/stow/)** - symlink-farm manager. The "one canonical file, N symlinks" deployment mechanism.
- **[chezmoi](https://www.chezmoi.io/)** - dotfile manager with per-machine targeting. The model for scoping context to each host.
- **[Hiera](https://github.com/puppetlabs/hiera)** (Puppet) - hierarchical, scope-based data lookup. The conceptual twin of the machine-scope intersection that selects which sources compose.

**Naming.** The field's vocabulary is **weave / layer / compose / overlay / blend**. We chose `compose` over the working name `meld`, which collides with [GNOME Meld](https://meldmerge.org/) on both search and semantics.

## Credits

- `static/wallpaper.jpg` - Sombra hacking skull, from the [Overwatch](https://overwatch.blizzard.com) Sombra ARG promotional materials, Blizzard Entertainment, circa 2016. All Overwatch art and iconography © Blizzard Entertainment. Used here for personal terminal decoration only.

## See also

- [AGENTS.md](AGENTS.md) - public-safe agent operating conventions and the global load point.
- [docs/FEATURES.md](docs/FEATURES.md) - inventory of what ships today.
- [Repo maps](docs/repo-maps.md) - compact starting points for high-churn warded workflow areas.
- [CODE-REVIEW.md](CODE-REVIEW.md) - root review contract for repo-local invariants and historical issues.
- [.ward/ward.yaml](.ward/ward.yaml) - allowlisted dev commands. Agents route through ward.

Cross-reference convention from [features-release-tooling.md](docs/features-release-tooling.md).
