# Features: agents and sessions

## Features: managed AGENTS.md pointer block

Every per-repo `AGENTS.md` opens with a one-line pointer at the workspace base. Hand-written, that line drifted into three styles and several dangled: `See ../AGENTS.md` resolves to a non-existent `coilyco-bridge/AGENTS.md`, because the repos live in different org dirs with no guaranteed sibling layout. The canonical content already loads globally via `~/.claude/CLAUDE.md`, so the pointer's real job is to orient a reader **without** that load point - a foreign agent, a single-repo clone, CI. For that reader a relative path is useless and "loads globally" is false, so only an absolute URL is true.

## What it renders

`generate-agents-pointer` is the single source of truth for the pointer, rendered as a marker-delimited managed block (`<!-- BEGIN managed by ... -->` / `<!-- END ... -->`). Each org points at its own base, on the host that fits its trust tier:

- **`coilyco-flight-deck/*`** - the public base on its GitHub mirror (`coilyco-flight-deck/agentic-os/AGENTS.md`), the public face an unauthenticated reader can open.
- **`coilyco-bridge/*`** - the same public base, with the private `agentic-os-kai/AGENTS.md` overlay on canonical Forgejo layered on top. The wording carries the layering: aos-pub is the foundation, aos-kai layers Kai-specific context over it.

The canonical base repos themselves (`agentic-os`, `agentic-os-kai`) are exempt - a base does not point at itself. `coilysiren` stays **deliberately unmanaged**: Kai's public personal namespace outside the coilyco-* fleet. Since the org migration it holds one `.agentic-os-ignore`-exempt repo, the profile `coilysiren/coilysiren`, carrying a bespoke bootstrap rather than a one-line pointer. It is hand-maintained rather than templated, so it takes a hand-fix instead of a generator branch. `website` moved to `coilyco-flight-deck/website`.

## Enforcement and application

- **`agents-pointer`** (pre-commit hook) regenerates the expected block offline from the repo's org, derived from its git remotes, and fails on drift, a missing block, or a legacy intro line lingering beside it. It no-ops for unmanaged orgs, exempt base repos, a symlinked `AGENTS.md`, or a repo with no root `AGENTS.md`.
- **`apply-agents-pointer`** injects or refreshes the block in place. Idempotent: it strips any prior managed block and known legacy intro lines, then inserts the fresh block right after the first H1.

This repo ships the validator and the applier. The fleet rollout that lands the block on every managed repo's canonical `main` is `scripts/agents-pointer-migrate.py` in infrastructure (`just agents-pointer-migrate`), per the authoring-vs-rollout split. (The earlier report-only Ansible `agents-pointer` role was retired when fleet pre-commit and pointer rollout moved onto the ward container.)

## Features: managed git-workflow block

Every repo declares its landing lane once, as `ward.workflow` in the AGENTS.md frontmatter, and `generate-git-workflow` renders a marker-delimited managed block from it that states the standing authorization to commit, branch, push, and open a pull request. The fleet runs one lane, `pull-request-and-merge`, and `merge-remote-main` is retired. Lanes, enforcement, and the one deliberate exemption: [git-workflow lanes](git-workflow-lanes.md).

## Features: agents and sessions

Agent naming and composition state.

## Agent self-name

Every agent session names itself from its composition: `acompose whoami` prints the composed seat, its subject pronoun, and the dictatable session short id (`Angie [she] uz86`). `docker/dev-base/session-name.sh` wraps that in the SessionStart banner and prints nothing when there is no projection, rather than inventing a name.

This replaces a locally derived `<harness>-<os>-<hostname>-<tag>-<pronouns>` name. That scheme could not know the composed seat, so an agent introduced itself as one thing while its status line said another, and it existed as two copies that had to stay format-identical.

The claude-hooks ansible role wires Claude Code's status line and SessionStart
hook without clobbering an operator setting. Its base merge also disables
auto-memory and denies the `claude-in-chrome` computer-use MCP while preserving
other user denies. Other harnesses export `AOS_AGENT_HARNESS=<harness>` and use
the hook points they expose. Local computation stays authoritative. The fleet
permission denies and the issue-ref Stop hook it also converges are described in
[native-claude-credentials.md](native-claude-credentials.md).

### Composed status line

The [status-line composer](dev-base-agent-identity.md) discovers ordered providers on hosts
and in dev-base containers. Its built-in provider shows the active Agent
Compose seat and bundle health. User and repository provider directories can
add, replace, or mask rows without forking the composer.

## Composed cross-harness agent context

Opt-in composer (`agent-compose`) that synthesizes global context from a declared set of sources, then points each harness's global load point (Claude Code, Codex, OpenCode) at the result by symlink. Shared sources produce one canonical `~/.config/agent-compose/COMPOSED.md` with no content duplicated on disk. A source may declare `harnesses: [claude, codex, opencode]` in YAML frontmatter when its doctrine applies only to part of the fleet. If configured harnesses select different source slices, the composer writes `COMPOSED.<harness>.md` outputs instead. Sources are listed explicitly or discovered by walking declared roots for `AGENTS.COMPOSE.md` files, the disjoint always-global doctrine that no harness's own AGENTS.md/CLAUDE.md cascade loads. A root need not be a repo checkout: pointing one at an out-of-repo directory (the ansible role uses `~/.config/agent-compose/sources/`) gives host-local doctrine a home that stays untracked and uncommitted while still composing into every harness's global context. Scope tags independently pick what composes per machine: the machine declares `scopes`, each source declares its own `scopes` in frontmatter, and a source composes only where the two sets intersect. Activation is the presence of `~/.config/agent-compose/agent-compose.yaml`: with no config, the composer is a total no-op and every harness behaves exactly as it does without it. The agent-compose ansible role runs it idempotently on each host.

A missing source degrades rather than freezes: Agent Compose skips it with a
warning and composes from the rest, while still refusing an empty result.
Infrastructure convergence and bare `acompose` refresh the load points through
the Go product. AOS ships no second composer or SessionStart refresh hook.

## Features: shell and secret handling

Cross-platform shell, terminal, and secret-handling capabilities.
