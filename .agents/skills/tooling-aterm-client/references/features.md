# Features

A coarse inventory of what ships. Architecture: [architecture.md](architecture.md).

* **Sidebar tabs** - hosts, running sessions, and roles to start as tab lists with arrow keys, scrolling strips below 720px.
* **Sessions as peers** - one tab per live session with its harness and code, so instances of a role on claude and codex sit side by side. A degraded startup names its skipped steps in the tab and header.
* **Live daemon** - "this Mac" speaks `aterm.daemon.v1` over the daemon's loopback websocket: roster, live sessions, attach with replay, typing, resize, and message states. A Demo host keeps a scripted copy for working with no daemon.
* **Host states** - checking, online, and not answering, each with its own panel and a retry.
* **Seat launch** - any role launches on any of its harnesses through the daemon's `launch` frame, beside running instances. A refused or failed launch says which, with the daemon's reason.
* **Session terminal** - xterm.js, fit to its pane.
* **Activity** - a seat's creature spins a ring in its colour while output flows, and a seat that finishes off screen shows "done, your turn" with a dot, counted in the page title.
* **Alt-tab triage** - the window title names who is waiting, e.g. "(2) Frog-Ox asking // aterm", and an installed app badges its icon. Tabbing in opens the seat that most needs you with its answer focused, number keys answer, Enter submits a multi-select, and each answer moves to the next waiting seat.
* **ask_choice** - a seat's structured `ask_choice` call shows as the same card, single or multi-select with an optional typed answer, and flags the seat "asking you" with a ? badge from any tab. The Demo host scripts two asks.
* **Native choices** - when a harness shows a select menu (Claude Code's AskUserQuestion, its trust prompt), a card offers the question and options as buttons, a typed answer for "Type something.", and Cancel. Picking one sends the harness its own keys.
* **Composer** - a real text field under the terminal, so dictation tools like Wispr Flow and phone keyboards work. Enter sends as one bracketed paste, with Enter a beat later, when the program asked for it. Typing `@` and a prefix offers the live sessions from the sidebar's list, with role and identity. Up and Down choose, Tab or Enter inserts the full name, Escape closes. It adds no daemon verb.
* **Peer message marking** - envelope lines are matched in the terminal buffer and striped in the sender's colour, across wrapped rows.
* **Messages** - each seat's sent and received messages with queued, held, launching, typed in, and failed states, and the daemon's reason.
* **Views and Browser tabs** - MCP Apps views and the seat's streamed browser, awaiting daemon frames. [views-and-browser.md](views-and-browser.md).
* **Read-only state** - when the daemon's typing guard refuses this browser (`welcome.typing`, or an `error` with `session_descendant` or `peer_unread`), the terminal, composer, choice cards, and launch buttons say read only and send nothing. The passkey locked state waits on accepted frames (COI-2484).
* **Hosted build** - the client under a path prefix behind a deployment's own sign-in, hosts added per device. [deploy.md](deploy.md).
* **Installable** - a web manifest and icons so Android and the Mac can install it as an app.

Designed, not built: the switchboard.

## See also

* [running.md](running.md) - what this is and how to run it.
* [SKILL.md](../SKILL.md) - the rules for changing the client.
