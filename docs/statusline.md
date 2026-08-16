# Status-line composer

A provider-discovery framework that auto-mounts the **full segment-composed
status line** into every warded container, so an in-container agent session
shows the same line a host session does.

## The problem it replaces

The retired host `agent-name.sh` hand-wired the
second row: it ran `$project_dir/.agentic-os/statusline.sh` and
`$AGENT_STATUSLINE_EXTRA` and appended their output. The container copy did not,
and `statusline.sh` was never shipped into the [dev-base image](dev-base-image.md),
so everything past the name row was absent in containers. Adding each segment by
hand does not scale and forces external users to fork the composer to customize.

## How it works

A **composer** ([`docker/dev-base/statusline.sh`](../docker/dev-base/statusline.sh))
is Claude Code's `statusLine` command. It reads the `statusLine` JSON payload on
stdin, runs each discovered **provider** in filename order (handing it the same
payload on stdin), and joins their output into the multi-row line.

**Provider contract:** exit 0 with stdout = that segment; empty stdout or a
non-zero exit = skipped. So a segment **self-suppresses** when irrelevant - the
Agent Compose provider renders nothing outside a projected workspace, or where
`acompose` is absent.

The built-in provider is:

* `15-agent-compose.sh` - asks `acompose statusline` to render the immutable
  bundle identity, role and harness, selected catalog footprint, and
  composition health.

* `20-container.sh` - names the warded container from `WARD_CONTAINER_NAME`,
  since inside a container the hostname is an opaque id. Silent on a host.

Two earlier base providers were removed. `10-agent-name.sh` rendered the
pre-acompose `<harness>-<os>-<host>-<tag>-<pronouns>` self-name row, which
duplicated the identity `acompose statusline` already renders. `20-repos.sh`
rendered a stray-checkout count, which is residency scanning rather than
session state. `agent-name.sh` itself is now retired: the
[SessionStart banner](dev-base-agent-identity.md) reads `acompose whoami`.

Agent Compose owns the row's content and bundle semantics. AOS only
discovers the provider and passes the project directory, so the status line
does not grow a second projection parser or identity cache.

## Discovery and overlays

The composer walks three provider dirs, lowest precedence first:

1. **base** - `<composer-dir>/statusline.d` (baked into the image; override with `AOS_STATUSLINE_DIR`).
2. **user** - `${XDG_CONFIG_HOME:-$HOME/.config}/agentic-os/statusline.d`.
3. **repo** - `<project_dir>/.agentic-os/statusline.d`.

A same-named file in a higher dir **overrides** the lower one; a new `NN-*.sh`
**adds** a row. A shadowing file that is not executable **masks** the lower
provider. So a project or an external user customizes the line by dropping in a
provider - **no fork** of the composer. Use 2-digit prefixes (lexical sort puts
`100` before `20`).

## Why it auto-mounts everywhere

Every warded container runs dev-base, and the baked policy-tier
[`managed-settings.json`](dev-base-agent-identity.md) points `statusLine` at the
composer. ward injects no `statusLine` of its own, so the baked one is
authoritative. A new base provider rides the next image build to **all**
containers at once, with no per-container edit. On hosts,
`install-session-name.py` conservatively migrates its legacy direct self-name
command to this composer, and repoints a SessionStart hook still wired to the
retired `agent-name.sh`. The infrastructure claude-hooks role invokes that
installer, keeping rollout separate from the provider authored here.

## See also

- [docs/dev-base-agent-identity.md](dev-base-agent-identity.md) - the agent self-name script + the baked managed settings.
- [docs/features-agents.md](features-agents.md) - the host self-name feature.
- [docs/dev-base-image.md](dev-base-image.md) - the image this rides in.
- [docs/FEATURES.md](FEATURES.md) - feature inventory.
