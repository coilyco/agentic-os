# Architecture

The client is one Svelte app with no router. State lives in `src/lib/app.svelte.ts`, and the sidebar's tabs choose what the main pane shows.

## The host connection

`HostConnection` in `src/lib/protocol.ts` is the whole seam: subscribe, attach, detach, type, resize, launch. `DaemonHost` maps it onto `aterm.daemon.v1` over the daemon's loopback websocket (agentic-os `docs/aterm-daemon.md`, Wire contract), and `MockHost` scripts the same events for the Demo host. The terminal subscribes before it attaches, so the replay an attach triggers is never missed. Launching sends the daemon's `launch` frame, which runs `aterm <role> [seat]` on the host and opens a real window there. Selection is by session name, never role, since a role runs any number of instances on any harness.

## Why envelopes are matched, not reported

A peer message is typed into the target harness's input, and the harness decides where it echoes it, often behind its own prompt. The daemon cannot know which screen rows hold it, and its message events carry no body. The daemon does escape envelope-shaped lines inside a body with a leading backslash, so the client marks a logical line holding an unescaped `[from <role> <identity>]` from a sender it has a message event for. Wrapped rows are joined first, so a narrow pane still marks the whole line. A seat quoting another's envelope in its own output would also be marked, which is cosmetic.

## Activity is output, not a flag

The daemon's `ready` means a seat has once been ready for a message, and some TUIs keep bracketed paste on while they work, so neither says whether a seat is busy. A working agent redraws constantly and an idle one prints nothing, so the client attaches to every live seat without replay or resize and calls it busy for 1.5 seconds after its last output. Output within 800ms of this client's own attach, resize, or typing is echo or redraw, and does not count. Attaches are reference counted so a closing terminal tab never detaches the monitor.

## Choices

Native choice cards read a harness's own select menu off the screen. See [choices.md](choices.md).

## The browser never answers terminal queries

A program reads the reply to a terminal query as input. The seat's kitty window already answers, so a browser reply is a second copy, and replayed history re-answers old queries: a live test typed `^[]11;rgb:...^[[4;1R` into a seat. The browser swallows status, attribute, and colour queries.

## Why a composer

Dictation tools insert text into a focused text field. xterm.js takes keys through a hidden textarea it clears as it reads, which dictation does not reliably drive. The composer is an ordinary textarea, so whatever can type into a browser can type to a seat.

## Views and the browser

See [views-and-browser.md](views-and-browser.md).

## Narrow screens

Below 720px the sidebar becomes one row of seat tabs and a Menu button, and the menu (hosts, alert switches, roles to start) opens over the seat. With no seat on screen those lists stay in the page, because the host's own panel sits below them. The session header folds to two rows, the composer's hint shortens, and a waiting card may use the whole terminal area (COI-2550). Below 1000px the side panel becomes a bottom sheet over the terminal (COI-2506, [views-and-browser.md](views-and-browser.md)).

## A restart is not a loss

A daemon restart is routine: launchd revives a crash, an upgrade kickstarts it, and the holders keep every seat alive. So `DaemonHost` redials itself once a welcome has come, and the window keeps the open seat, the roster, and the connection object. Only a host that never answered reports `closed`.

* **Backoff** - 1, 2, 4, 8, 16, then 30 seconds, spread 20% either way (`src/lib/backoff.ts`). A dial that has not welcomed in 10 seconds is dropped. Retry, the browser coming back online, and the tab becoming visible dial at once.
* **What comes back** - after the welcome the host subscribes again, and the next sessions list triggers one `attach` per seat it held. A seat with a terminal replays at its last size into a cleared screen. The rest are watched without replay, and a seat the daemon no longer lists is dropped. Browser watches are sent again.
* **What is not sent** - frames typed or clicked into the dead link are dropped, never queued, so a keystroke cannot land after the redial. The terminal, composer, choice cards, and launch buttons say reconnecting meanwhile, and a draft stays in the composer.
* **Newer build** - the first welcome's `version` is the build that served the page. A later welcome with a higher one sets `app.newerBuild`, and the banner offers Reload. A reload reaches the new build because the worker asks the network first for a navigation. A hosted page is not served by the daemon, so it never shows the cue.
