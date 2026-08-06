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
- `agent-terminal/` - `aosterm` and compatibility branding for one Alacritty window.
- `warp/` - transitional Warp config (`settings.toml`, `tab_configs/`) plus the `ward exec warp` Go module.
- `aos-cli/` - the Go composition root for standalone and Ward-governed agent launches.
- `aos-say/` - the `ward exec aos-say` Go module for the speech helper client and relay.
- `karabiner/` - Karabiner-Elements complex modification assets (`brew install --cask karabiner-elements`), symlinked into the local Karabiner config tree.
- `scripts/` - portable utilities (gpg-ssm wrapper, agent-name hooks, aws-config lint).
- `.agents/skills/` - ordinary `SKILL.md` sources that every composed role can discover.
- `.agents/composed/` - role-scoped `COMPOSED.md` sources that agent-compose promotes only for allowlisted roles.
- `.specgen/guardfiles/` - recursive specgen project for AOSguard policy and
  reproducible build locks.
- `agentic_os/` - the `aos-precommit` package, generators, shared config/data, and hygiene guardrails behind the independently released hook suite.

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
mkdir -p ~/.config/alacritty
ln -sf "$PWD/alacritty/alacritty.toml" ~/.config/alacritty/alacritty.toml
ward exec warp apply                     # warp config
```

On Windows, `ward exec apply-shell-links` manages `~/.zshrc` and the `gpg-ssm.cmd`
shim only. It also links the Forgejo git credential helper; Git Bash popup shells
should not recreate `~/.bashrc`.

Agent self-name and composition hooks, per-host steps, and gpg wiring: [docs/install.md](docs/install.md).

## aos CLI

AOS always composes the selected role and attaches generated `aosguard`:

```bash
aoscompose engineer --version
aoscompose engineer goose --version
aosward --agent codex --role engineer -- owner/repo#267
```

`aoscompose` is the canonical explicit alias of `aos`. The earlier `aoscomposed`
spelling remains as a compatibility alias. As a standalone convenience,
`aoscompose <role>` selects the role's default agent from
[`.agents/harness-launch-profiles.yaml`](.agents/harness-launch-profiles.yaml),
and `aoscompose <role> <harness>` overrides that default.
Auth is default-on, with `--auth=false` for startup checks that should not require a harness credential.
`aosward` adds `--warded`. Ward remains the fixed workflow and container
lifecycle owner, agent-compose remains the context producer, and
cli-guard/specgen remains the guarded-tool generator. Matching
role names never union authority between those layers. AOS applies its own
bounded standalone runtime gates, including [kubeconfig projection](docs/aos-kubeconfig.md).

AOS also converges host-aware runtime inputs with `aos converge`, while
`aos converge --check` detects drift. That surface owns verified remote
catalogue caching, a deterministic local manifest, native MCP projection, and
per-server Codex approval policy. See the [environment convergence contract](docs/aos-convergence.md).

The original standalone composed-container command remains available:

```bash
aos --role engineer acompose -- codex
```

The `aos` prefix is the container boundary. Bare
`acompose <role> <harness>` is the native-host role launcher and does not start
Docker. The standalone AOS path projects the resolved host mcporter inventory into the
ephemeral agent home. When configured HTTP MCP endpoints resolve into the
tailnet, AOS attaches the container to the shared tailnet network and bridges
those endpoints through the standing proxy without invoking Ward. See the
[standalone connectivity contract](docs/aos-standalone-connectivity.md).

For a bounded role check-in, AOS owns the agent's non-interactive defaults:

```bash
aos --role engineer --agent codex acompose-checkin
```

Goose is the proven local-model launch path. Select it explicitly with the
role whose context Agent Compose should materialize:

```bash
aos --agent goose --role ops -- --version
```

Ward is not part of that standalone path. See the [launch and handoff contract](docs/aos-cli.md).
Homebrew and Scoop install `aos`, `aoscompose`, `aoscomposed`, `aosward`, `aosguard`, `agent-terminal`, and `aosterm`.
Direct release binaries and the aligned native update path are documented in the [CLI release walkthrough](docs/aos-cli-release.md).

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

Opt-in tooling that composes global agent context and symlinks each harness load point to it. Sources are shared unless optional `harnesses` frontmatter selects a harness-specific slice. Agent-compose embeds the canonical personalities. AOS publishes the [capability provider](docs/personality-provider.md): ordinary skills and [role-composed skills](docs/role-composed-skills.md). Host composition stays inert until `~/.config/agent-compose/agent-compose.yaml` exists.

**Prior art.** The idea is fresh in the agentic space but well-trodden in config management, and agent-compose is best understood as **Hiera-for-agent-doctrine, deployed Stow-style, scoped chezmoi-style**:

- **[GNU Stow](https://www.gnu.org/software/stow/)** - symlink-farm manager. The "one canonical file, N symlinks" deployment mechanism.
- **[chezmoi](https://www.chezmoi.io/)** - dotfile manager with per-machine targeting. The model for scoping context to each host.
- **[Hiera](https://github.com/puppetlabs/hiera)** (Puppet) - hierarchical, scope-based data lookup. The conceptual twin of the machine-scope intersection that selects which sources compose.

**Naming.** The field's vocabulary is **weave / layer / compose / overlay / blend**. We chose `compose` over the working name `meld`, which collides with [GNOME Meld](https://meldmerge.org/) on both search and semantics.

The native assigned-role form is:

```bash
acompose design codex
```

The shared shell places that launch in a leased native workspace, then Agent
Compose supplies only the selected role, its full personality meld, and its
role-composed capability slice. Claude, Codex, Goose, and OpenCode share this
grammar.

## Credits

- `static/wallpaper.jpg` - Sombra hacking skull, from the [Overwatch](https://overwatch.blizzard.com) Sombra ARG promotional materials, Blizzard Entertainment, circa 2016. All Overwatch art and iconography © Blizzard Entertainment. Used here for personal terminal decoration only.

## See also

- [AGENTS.md](AGENTS.md) - public-safe agent operating conventions and the global load point.
- [docs/FEATURES.md](docs/FEATURES.md) - inventory of what ships today.
- [Repo maps](docs/repo-maps.md) - compact starting points for high-churn warded workflow areas.
- [CODE-REVIEW.md](CODE-REVIEW.md) - root review contract for repo-local invariants and historical issues.
- [.ward/ward.yaml](.ward/ward.yaml) - allowlisted dev commands. Agents route through ward.

Cross-reference convention from [features-release-tooling.md](docs/features-release-tooling.md).
