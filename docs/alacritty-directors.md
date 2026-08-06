# Branded Alacritty directors

`aosterm` launches one composed session in one statically branded Alacritty
window. Agent-compose supplies canonical identity, agentic-os renders the
terminal brand, and `aoscompose` remains the child process. `agent-terminal`
stays as the compatibility command name.

## Base configuration

[`alacritty/alacritty.toml`](../alacritty/alacritty.toml) carries the portable
Sombra palette, opaque window treatment, padding, font size, live reload, and
copy-only OSC 52 policy. It deliberately leaves shell selection, startup
directory, and scrollback at the owning host or Alacritty defaults. It also
defines no tabs, panes, or terminal multiplexer.

The host-local root config may import or copy this baseline, then add only its
local shell and startup directory. Infrastructure owns converging that layout
across native director hosts.

## Launch

The repository-scoped entry point is:

```text
aosterm \
  --expression acting \
  --task-title agentic-os#730 \
  --working-directory . \
  director codex -- --version
```

Homebrew and Scoop install `aosterm` and `agent-terminal` on `PATH` beside the
other AOS commands. Installation, upgrades, rollbacks, direct release assets,
and version checks are documented in the
[native launcher walkthrough](agent-terminal-native.md).

`--working-directory` defaults to `$PROJECTS_ROOT`, with `~/projects` as the
portable fallback. A caller passes the flag explicitly when one director should
open inside a specific checkout.

The launcher calls `agent-compose overlay --json`, validates the overlay, then
launches Alacritty with `aoscompose <role> <seat> ...` as its executable tail.
It derives:

* a title from personality glyphs, the named seat, expression, and task
* the canonical melded favorite color as the cursor and selection accent
* a subtle opaque background tint
* readable selection text selected by contrast

The launcher passes every value to Alacritty as an argument. It invokes no
shell and emits no terminal control sequences into the director process.

## Inspect

Add `--dry-run` before the `aoscompose` arguments to print
`agent-terminal.launch.v1` JSON without requiring Alacritty. The document
contains the selected identity, derived brand, working directory, and complete
Alacritty argument vector.

`AGENT_COMPOSE_BIN`, `AOSCOMPOSE_BIN`, and `ALACRITTY_BIN` may select
non-default executable locations.

## Ownership

Agent-compose owns renderer-neutral identity and validates role, seat, and
expression. Agentic-os owns this Alacritty adapter. Ward owns runtime authority
and the director lifecycle. Infrastructure owns fleet installation and
default-terminal rollout.

## Deliberate first-slice limits

Branding is fixed at launch. The adapter manages no tabs, panes, sessions,
avatars, background images, or interactive loop. It does not remap the ANSI
palette because director TUIs use those colors semantically.

Runtime expression updates can follow only after the static surface has
cross-platform acceptance evidence.
