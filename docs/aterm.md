# The native agent terminal

`aterm` opens the window a native agent session runs in. The status-line composer filling the rows inside it is documented with the image that bakes it, in [in-container agent identity](dev-base-agent-identity.md).

## `aterm`

`aterm` opens one composed agent session in its own branded kitty window. It is the windowed sibling of `acompose`, runs the same leased shadow wrapping `agent-compose launch`, and leaves the terminal you typed in free. Name a role without a seat and both take the agent from [`harness-launch-profiles.yaml`](../.agents/harness-launch-profiles.yaml), which `aos` owns and both ask, so neither carries a second parser.

```text
aterm                              # pick a role, then a seat
aterm platform                     # the role's default seat
aterm platform codex -- --resume   # arguments for the harness
aterm --list                       # the live roster, no window
aterm --list --json                # the same roster, for a script
```

The window opens fullscreen at font size 14.5, which `--start-as` and `--font-size` override, since kitty's default 11.0 suits a terminal rather than a session you read all day. It needs `agent-compose` and kitty on `PATH` and bundles neither. Without `aos` it still launches, unleased. `--dry-run` prints the plan and opens nothing. `--list --json` prints the launchable projection, every live role carrying only the seats `agent-compose launch` can start, under contract `aterm.roster.v1`.

**It refuses a stale role before it opens anything.** Role slugs turn over, so `aterm` reads `agent-compose catalog roles --json` on every run and names the live roster in the refusal. A transposed slug comes back as `is not a live role. Did you mean platform?` plus every live slug. A seat is checked twice: it must belong to the role and be a harness `agent-compose launch` can start. `penpot` is real but not launchable, and the refusal says which check it failed.

**Tab completes from the same roster.** `aterm <TAB>` offers the live slugs, `aterm sysadmin <TAB>` only that role's launchable seats, so a slug that turned over stops completing rather than completing into a refusal. The read is under 10ms, so no cache goes stale. `shell/common.sh` registers bash and zsh through `aterm completion <shell>`, after `compinit` in zsh. A missing `agent-compose` yields silence, never a diagnostic mid-keystroke.

**A slow pre-flight names itself.** `aterm` shells out for a seat, roster, and overlay before opening anything and captures their output, so a wrapped `aos` converging the host read as a launcher that had stopped. After two seconds it names the command it waits on.

**A failing launch stays on screen.** A terminal closes the window the moment its child exits, so a failure used to vanish before anyone read why. `aterm` runs the child through its own `_session` stage rather than handing the harness to kitty. That stage passes the exit code through and holds the window on any non-zero exit, and `--hold` holds after a clean one too. The launcher watches for a startup failure, so "no window appeared" names its cause.

**The title leads with what separates two windows.** A window manager truncates near 30 characters, so segments run workspace, task title, role, emblems and seat name, expression. The workspace is `repo@branch` for the checkout `--working-directory` names, left out when that is the default projects root.

**`--dry-run` reads for a person, and failures split by code.** The default renders the identity, workspace, brand swatches, each personality in its color, and the child argv. `--dry-run --json` keeps the machine plan `scripts/check-aos-release.sh` asserts against. Exit codes: 2 usage, 3 off-roster role or seat, 4 a missing dependency, 5 the window failed to open, 1 anything else, and a child's own code passes through.

**The picker follows the terminal, not stdout.** `aterm > log` used to refuse with "a role is required", because the check wanted stdin and stdout both to be character devices. It runs on `/dev/tty`.

**The sound mark is opt-in, because a window opens many times a day.** `--sound` plays the role's pair and `ATERM_SOUND` turns it on for a shell, and neither reaches a log, a recording, or CI, where the terminal check refuses it whatever was asked. The samples still ship and `just aterm-sounds` still renders them, so this is a default rather than a removal. An env var alone could not have delivered silence: the `.app` wrapper harvests only `PATH` from a login shell, so a Dock or Spotlight launch never sees the caller's environment and only a default reaches both ways in.

**`aterm doctor` preflights the whole chain**, exits 1 on a broken link, and names the unleased-shadow case a launch makes silently. `--json` is `aterm.doctor.v1`.

**It decodes the whole identity overlay, and renders rather than derives.** A struct naming fewer fields than the overlay ships drops the rest in silence, so `aterm/overlay.go` declares every leaf and a round-trip test fails on any that does not. The window background is the roster's `background`: separation is a property of the set, and seven accents tinted alike land inside each other's JND. aterm tints only for an agent-compose too old to ship one (agent-compose#358).

**The fixtures do not catch upstream drift, so `just aterm-contract` walks the live roster.** Every launchable seat resolves, every unlaunchable one refuses with exit 3, every timbre has a sample, and the closest background pair holds a dE 3.0 floor. Its own reason: it diffs the live overlay against the typed struct leaf by leaf, so a dropped field fails the day it is added. `ATERM_LIVE_ROSTER` makes a missing `agent-compose` fail rather than skip.

## The creature background

Every window stands its role's own creature behind the session. See
[the aterm creature background](aterm-creature.md).

## macOS app bundles

`aterm bundles` writes one `.app` per live role into `~/Applications`. See
[aterm macOS app bundles](aterm-bundles.md).
