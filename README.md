<!-- maintained under agentic-os -->
# agentic-os

![Sombra hacking skull](static/wallpaper.jpg)

The host layer everything else here runs on: shell and terminal configuration,
the `aos` launcher, and the cross-repo pre-commit suite.

**This is a reference implementation rather than a product.** It is the working
setup for one fleet, grab bag and all, and it is public so the shape is
readable rather than because it is meant to be adopted whole. The one piece
built to travel is the hook suite below, which every repo in the fleet consumes
by upstream ref.

Zsh runs on Mac and Linux, with Bash through Git for Windows. kitty hosts the
`aterm` windows on Mac and Linux, and Alacritty is the direct terminal on
Windows.

## Layout

- `shell/` - shared `common.sh` plus thin `zshrc` + `bashrc`, so bash and zsh match.
- `kitty/` - portable Sombra appearance and terminal security defaults for `aterm` windows, with host preferences left to the local wrapper.
- `alacritty/` - the same baseline for Alacritty, retained for Windows, where kitty does not ship.
- `aterm/` - `aterm`, the branded launcher for one composed agent session, and the macOS `.app` bundle it generates per role.
- `aos-cli/` - the Go composition root for standalone and Ward-governed agent launches.
- `aos-say/` - the `just aos-say` Go module for the speech helper client and relay.
- `karabiner/` - Karabiner-Elements complex modification assets (`brew install --cask karabiner-elements`), symlinked into the local Karabiner config tree.
- `scripts/` - portable utilities (gpg-ssm wrapper, session-name hooks, aws-config lint).
- `.agents/skills/` - ordinary `SKILL.md` sources that every composed role can discover.
- `.agents/composed/` - role-scoped `COMPOSED.md` sources that agent-compose promotes only for allowlisted roles.
- `.umbra/guardfiles/` - recursive umbra project for AOSguard policy and
  reproducible build locks.
- `agentic_os/` - the `aos-precommit` package, generators, shared config/data, and hygiene guardrails behind the independently released hook suite.

Full breakdown: [docs/repo-layout.md](docs/repo-layout.md).

## The pre-commit suite

`agentic_os/` ships `aos-precommit`, released independently as `aos-precommit-v*`
and consumed by every repo in the fleet through
[`.pre-commit-config.yaml`](.pre-commit-config.yaml) rather than forked. It
validates repository layout and documentation shape (`documentation-layout`,
`catalog-doc-size`), the README, AGENTS, and FEATURES trifecta plus its
cross-links (`catalog-trifecta`, `dead-cross-links`, `source-doc-refs`), skill
and composed-skill conventions (`check-skills`, `check-composed-skills`),
comment density (`code-comments`), and an offline secret scan.

Size caps come from a declared band. A repo picks `small` or `large` and there
is no default to fall into, because an undeclared repo and a deliberately small
one would otherwise be the same file. See
[docs/documentation-bands.md](docs/documentation-bands.md) and
[docs/catalog-caps-reference.md](docs/catalog-caps-reference.md).

## Install

Host config is converged by Ansible (rollout lives in infrastructure, per [AGENTS.md](AGENTS.md)). Manual fallback:

```bash
just apply-shell-links
```

Equivalent links on Mac and Linux:

```bash
ln -sf "$PWD/shell/zshrc"  ~/.zshrc      # both source shell/common.sh
ln -sf "$PWD/shell/bashrc" ~/.bashrc
ln -sf "$PWD/scripts/gpg-ssm" ~/.local/bin/gpg-ssm
mkdir -p ~/.config/kitty
ln -sf "$PWD/kitty/kitty.conf" ~/.config/kitty/kitty.conf
```

On Windows, `just apply-shell-links` manages `~/.zshrc` and the `gpg-ssm.cmd`
shim only. It also links the Forgejo git credential helper; Git Bash popup shells
should not recreate `~/.bashrc`.

Agent self-name and composition hooks, per-host steps, and gpg wiring: [docs/install.md](docs/install.md).

## aos CLI

`aos` is the launcher. It always composes the selected role and attaches a
generated `aosguard`, so a launch carries context and a guarded tool surface
together.

```bash
aoscompose platform                          # role's default agent
aoscompose platform goose                    # override the agent
aosward --agent codex --role platform -- owner/repo#267
aos converge                                 # host runtime inputs, --check for drift
```

`aoscompose` is the canonical explicit alias of `aos`, and `aoscomposed`
survives as a compatibility spelling. `aosward` adds `--warded`. Bare
`acompose <role> <harness>` is the native-host launcher and starts no Docker,
while an `aos` prefix is the container boundary.

Role names never union authority across layers. Ward owns fixed workflows and
container lifecycle, agent-compose produces context, and umbra generates the
guarded tools. AOS applies its own bounded standalone gates on top.

Homebrew and Scoop install `aos`, `aoscompose`, `aoscomposed`, `aosward`,
`aosguard`, and `aterm`. The launch and handoff contract,
release binaries, and the native update path are in
[docs/aos-cli.md](docs/aos-cli.md). Convergence is in
[docs/aos-convergence.md](docs/aos-convergence.md), standalone connectivity in
[docs/aos-context-bundle.md](docs/aos-context-bundle.md), and cluster access in
[docs/aos-cluster-access.md](docs/aos-cluster-access.md).

## Secrets pattern

Fetch one parameter only when a command needs it:

```bash
ssm-get /eco/server-api-token
```

`ssm-get` prints the decrypted value to stdout without writing it to disk. The
AWS profile defaults to `default`; pass a profile and region as the second and
third arguments. The implementation is `scripts/ssm-get` on `PATH`, which the
shell function of the same name delegates to, so scripts and non-interactive
shells reach it too. The bulk shell-environment exporter and the legacy
cleartext-on-disk dump (`~/.cache/ssm-env.sh`) are removed.

## agent-compose

Opt-in tooling that composes global agent context and symlinks each harness load point to it. Sources are shared unless optional `harnesses` frontmatter selects a harness-specific slice. Agent-compose embeds the canonical personalities. AOS publishes the [capability provider](docs/context-budget.md): ordinary skills and [role-composed skills](docs/role-composed-skills.md). Host composition stays inert until `~/.config/agent-compose/agent-compose.yaml` exists.

**Prior art.** The idea is fresh in the agentic space but well-trodden in config management, and agent-compose is best understood as **Hiera-for-agent-doctrine, deployed Stow-style, scoped chezmoi-style**:

- **[GNU Stow](https://www.gnu.org/software/stow/)** - symlink-farm manager. The "one canonical file, N symlinks" deployment mechanism.
- **[chezmoi](https://www.chezmoi.io/)** - dotfile manager with per-machine targeting. The model for scoping context to each host.
- **[Hiera](https://github.com/puppetlabs/hiera)** (Puppet) - hierarchical, scope-based data lookup. The conceptual twin of the machine-scope intersection that selects which sources compose.

**Naming.** The field's vocabulary is **weave / layer / compose / overlay / blend**. We chose `compose` over the working name `meld`, which collides with [GNOME Meld](https://meldmerge.org/) on both search and semantics.

The native assigned-role form is:

```bash
acompose frontend codex
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
- [Repo maps](docs/repo-layout.md) - compact starting points for high-churn warded workflow areas.
- [CODE-REVIEW.md](CODE-REVIEW.md) - root review contract for repo-local invariants and historical issues.
- [justfile](justfile) - dev verbs, which agents route through - and [.ward/ward.yaml](.ward/ward.yaml), catalog metadata only.

Cross-reference convention from [release.md](docs/release.md).
