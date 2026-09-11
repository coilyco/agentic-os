# In-container agent identity

Who a warded agent is to git and to itself.

## Warded-agent git identity

A warded aos session keeps its self-name on the status line and the session
banner. Git identity is separate: the committer NAME and EMAIL resolve to the
deployment bot so Forgejo links the commit to the coilyco-ops account instead of
an example fallback.

## What runs

The Dockerfile owns `AOS_GIT_NAME` and `AOS_GIT_EMAIL` once. Every language
target writes them into Git's system config through
[`install-common.sh`](../docker/dev-base/install-common.sh), so every user
inherits the deployment identity without a runtime write. The
[`ward-shell-entrypoint.sh`](../docker/dev-base/ward-shell-entrypoint.sh) maps
the same image-owned values onto Ward's provider-neutral `WARD_GIT_*` transport
seam before Ward bootstraps the container. The baked
[`git-identity.sh`](../docker/dev-base/git-identity.sh)
is the fallback for older or custom images, and the
policy-tier [`managed-settings.json`](../docker/dev-base/claude-managed-settings.json)
wires it as a second `SessionStart` hook, right after the self-name banner.

## Why SessionStart remains

SessionStart is the earliest the self-name banner and bot identity hook can run
with the full agent environment in place. The distinguishing `<tag>` is derived
from the `session_id`, which the entrypoint cannot know before the agent starts.
The identity hook backfills only when the system config is absent, so it
respects the Dockerfile-owned config instead of overwriting it.

## Why the bot identity is explicit

The committer identity must not drift back to ward's example bot defaults. The
image-level `AOS_GIT_NAME` and `AOS_GIT_EMAIL` config owns the deployment
identity. Ward carries those values only through its generic runtime contract.

## Scope and limits

- **Best-effort.** A git failure is swallowed so it never breaks session start.
- **all warded harnesses.** The entrypoint establishes the image-owned identity
  before any harness starts. Claude's SessionStart hook remains a fallback.
- **Dockerfile owns the baseline.** The runtime hook is fallback only.

## In-container agent self-name

A host session self-names through a Claude Code SessionStart + statusLine hook
the claude-hooks ansible role wires into `~/.claude/settings.json`. That role
never runs inside a `ward agent` container, so a warded explorer / headless
agent in the [dev-base image](dev-base-image.md) would otherwise stay nameless.
The image bakes the same slice in directly.

## Agent Compose owns the name

The name comes from `acompose whoami`, which prints the composed seat and the
session short id (`Angie [she] uz86`). Nothing derives a name locally.

That is the change worth knowing about. The retired `agent-name.sh` computed a
name from harness, OS, hostname, and a tag sliced out of the raw session UUID.
It could not know the composed seat, so an agent introduced itself as one thing
while its status line said another, and the tag used the full alphabet rather
than the [dictatable](https://forgejo.coilysiren.me/coilyco-flight-deck/agent-compose/src/branch/main/docs/short-id.md)
one, so it was not reliably speakable.

It also had to exist twice - a host copy and a format-identical container copy -
with a documented drift hazard between them. There is now one script and one
authority.

## What the image bakes

- [`session-name.sh`](../docker/dev-base/session-name.sh) - the SessionStart
  banner, landed world-readable at `/opt/agentic-os/session-name.sh` so it runs
  as any uid. It reads the project directory from the hook payload, calls
  `acompose whoami`, and prints nothing when there is no projection or no
  `acompose` on PATH. The same file is what the host wires, so the two surfaces
  cannot drift.
- [`git-identity.sh`](../docker/dev-base/git-identity.sh) - the git-identity
  stamp above, split out because it never had anything to do with naming
  beyond sharing a hook event.
- [`statusline.d/20-container.sh`](../docker/dev-base/statusline.d/20-container.sh) -
  a status-line row naming the warded container from `WARD_CONTAINER_NAME`,
  since inside a container the hostname is an opaque id. Silent on a host.
- [`managed-settings.json`](../docker/dev-base/claude-managed-settings.json) -
  landed at the fixed `/etc/claude-code/managed-settings.json`, adding the
  `statusLine` composer and the two `SessionStart` hooks.

## Status-line composer

A provider-discovery framework that auto-mounts the **full segment-composed status line** into every warded container, so an in-container session shows the same line a host one does. A **composer** ([`docker/dev-base/statusline.sh`](../docker/dev-base/statusline.sh)) is Claude Code's `statusLine` command. It reads the `statusLine` JSON payload on stdin, runs each discovered **provider** in filename order (handing it the same payload on stdin), and joins their output into the multi-row line.

**Provider contract:** exit 0 with stdout = that segment, and empty stdout or a non-zero exit = skipped. So a segment **self-suppresses** when irrelevant: the Agent Compose provider renders nothing outside a projected workspace or where `acompose` is absent. `15-agent-compose.sh` asks `acompose statusline` for the immutable bundle identity, role and harness, catalog footprint, and composition health, and `20-container.sh` names the container as above. `10-agent-name.sh` and `20-repos.sh` were removed as a duplicate identity and a residency scan.

Agent Compose owns the row's content and bundle semantics, and AOS only discovers the provider and passes the project directory, so the line grows no second projection parser or identity cache.

**Discovery and overlays.** The composer walks three provider dirs, lowest first: **base** `<composer-dir>/statusline.d`, baked in and overridden by `AOS_STATUSLINE_DIR`, **user** `${XDG_CONFIG_HOME:-$HOME/.config}/agentic-os/statusline.d`, and **repo** `<project_dir>/.agentic-os/statusline.d`. A same-named file in a higher dir **overrides** the lower one, a new `NN-*.sh` **adds** a row, and a shadowing file that is not executable **masks** the lower provider. So a project or an external user customizes the line by dropping in a provider, **no fork** of the composer. Use 2-digit prefixes, since a lexical sort puts `100` before `20`.

**Why it auto-mounts everywhere.** ward injects no `statusLine` of its own, so the baked one is authoritative and a new base provider rides the next image build to **all** containers at once. On hosts, `install-session-name.py` migrates a legacy self-name command to this composer and repoints a SessionStart hook still wired to the retired `agent-name.sh`. The infrastructure claude-hooks role invokes it, keeping rollout separate from the provider authored here.

## Why policy-tier settings

The image bakes in no user: ward owns the run-as-uid, mount set, and config
injection. Policy settings are the one Claude Code surface whose location does
not depend on the run-as-uid, `HOME`, or `CLAUDE_CONFIG_DIR` ward picks, so a
fixed `/etc/claude-code/managed-settings.json` reaches the agent regardless. It
adds only those keys, so it layers over whatever ward injects into user settings
rather than replacing it.
