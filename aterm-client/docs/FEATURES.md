# Features

A coarse inventory of what ships. Architecture: [architecture.md](architecture.md).

* **Sidebar tabs** - hosts and seats as vertical tab lists with arrow-key focus, collapsing to two scrolling strips below 720px.
* **Live daemon** - "this Mac" speaks `aterm.daemon.v1` over the daemon's loopback websocket: roster, live sessions, attach with replay, typing, resize, and message states. A Demo host keeps a scripted copy for working with no daemon.
* **Host states** - checking, online, and not answering, each with its own panel and a retry. Hosts beyond this Mac wait on the daemon listening past loopback.
* **Seat launch** - a seat that is not running launches on any of its harnesses through the daemon's `launch` frame, which opens its window on the host. A refused or failed launch says which, with the daemon's reason.
* **Session terminal** - xterm.js with fit-to-pane resizing and the role's accent.
* **Activity** - a seat's creature spins a ring in its colour while output flows, and a seat that finishes off screen shows "done, your turn" with a dot, counted in the page title.
* **ask_choice** - a seat's structured `ask_choice` call shows as the same card, single or multi-select with an optional typed answer, and flags the seat "asking you" with a ? badge from any tab. The Demo host scripts two asks. Waiting on the daemon frames.
* **Native choices** - when a harness shows a select menu (Claude Code's AskUserQuestion, its trust prompt), a card offers the question and options as buttons, a typed answer for "Type something.", and Cancel. Picking one sends the harness its own keys.
* **Composer** - a real text field under the terminal, so dictation tools like Wispr Flow and phone keyboards work. Enter sends as one bracketed paste, with Enter a beat later, when the program asked for it.
* **Peer message marking** - envelope lines are matched in the terminal buffer and striped in the sender's colour, across wrapped rows.
* **Messages panel** - each seat's sent and received messages with queued, held, launching, typed in, and failed states, and the daemon's reason.
* **Installable** - a web manifest and icons so Android and the Mac can install it as an app.

Designed, not built: MCP Apps views, the shared browser, and the switchboard.

## See also

* [README.md](../README.md) - what this is and how to run it.
* [AGENTS.md](../AGENTS.md) - agent operating rules for this repo.
