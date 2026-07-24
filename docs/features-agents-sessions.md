# Features: agents and sessions

Agent naming and session orientation.

## Agent self-name

Every agent session gets a stable name: `<harness>-<os>-<hostname>-<tag>-<pronouns>`, `<tag>` the last four characters of the session id and `<pronouns>` the harness's slug. The five: Claude `she-her`, Codex `he-him`, OpenCode `they-them`, Aider `they-them`, Goose `she-her`. `scripts/agent-name.sh` holds the registry and picks the harness from `AOS_AGENT_HARNESS` (default `claude`).

The claude-hooks ansible role wires Claude Code's status line and SessionStart
hook without clobbering an operator setting. Its base merge also disables
auto-memory and denies the `claude-in-chrome` computer-use MCP while preserving
other user denies. Other harnesses export `AOS_AGENT_HARNESS=<harness>` and use
the hook points they expose. Local computation stays authoritative.

### Second status-line row

Two hooks add optional rows: project-local `$project_dir/.agentic-os/statusline.sh` for per-project status, and `$AGENT_STATUSLINE_EXTRA` (an executable) for host-global. A harness wires its own; this repo ships a [repo-checkout tracker](repo-tracker.md).

## Session pulse

Generic SessionStart hook that cats `~/.cache/agentic-os/session-pulse.yaml` when present and no-ops otherwise. Zero compute at session start. Stale cache is acceptable signal - the file's mtime tells the operator how fresh the orientation is. The plugin point is "write to that path." Any consumer can hook in. YAML so secondary surfaces can reuse the same blob without re-parsing prose. The producer lives in consumer-specific tooling.

## Composed cross-harness agent context

Opt-in composer (`agent-compose`) that synthesizes global context from a declared set of sources, then points each harness's global load point (Claude Code, Codex, OpenCode) at the result by symlink. Shared sources produce one canonical `~/.config/agent-compose/COMPOSED.md` with no content duplicated on disk. A source may declare `harnesses: [claude, codex, opencode]` in YAML frontmatter when its doctrine applies only to part of the fleet. If configured harnesses select different source slices, the composer writes `COMPOSED.<harness>.md` outputs instead. Sources are listed explicitly or discovered by walking declared roots for `AGENTS.COMPOSE.md` files, the disjoint always-global doctrine that no harness's own AGENTS.md/CLAUDE.md cascade loads. A root need not be a repo checkout: pointing one at an out-of-repo directory (the ansible role uses `~/.config/agent-compose/sources/`) gives host-local doctrine a home that stays untracked and uncommitted while still composing into every harness's global context. Scope tags independently pick what composes per machine: the machine declares `scopes`, each source declares its own `scopes` in frontmatter, and a source composes only where the two sets intersect. Activation is the presence of `~/.config/agent-compose/agent-compose.yaml`: with no config, the composer is a total no-op and every harness behaves exactly as it does without it. The agent-compose ansible role runs it idempotently on each host.

A missing source **degrades** rather than freezes: the composer skips it with a warning and composes from the rest (an empty result is still refused), so one wrong entry - a work overlay listed on a personal mac, an overlay not yet cloned - can no longer silently freeze every harness's load point until the next converge. Editing a source between converges is covered by the **compose-freshen SessionStart hook** (`scripts/agent-compose-freshen.sh`, wired by the claude-hooks role): every session start re-runs the composer, which rewrites only when content actually changed (silent no-op otherwise) and prints any skipped-source warning into session context. The result is a load point that self-heals on the next session and stays loud at the point of use instead of drifting quietly.
