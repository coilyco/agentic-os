# Features

A coarse inventory of what ships. Architecture: [architecture.md](architecture.md).

* **Sidebar tabs** - hosts, running sessions, and roles to start as tab lists with arrow keys. Below 720px a seat row and a Menu button hold the seat first and put hosts, alerts, and roles one tap away.
* **Sessions as peers** - one tab per live session with its harness and code, so instances of a role on claude and codex sit side by side. A degraded startup names its skipped steps in the tab and header.
* **Live daemon** - "this Mac" speaks `aterm.daemon.v1` over the daemon's loopback websocket: roster, live sessions, attach with replay, typing, resize, and message states. A Demo host keeps a scripted copy for working with no daemon.
* **Host states** - checking, online, and not answering, each with its own panel and a retry. A host that answered before and goes quiet reads as stopped answering, not as never started.
* **Reconnect** - a daemon restart under an open window redials with backoff, keeps the open seat and roster greyed as last known, attaches seats again with replay, and offers a reload when the daemon came back as a newer build. [architecture.md](architecture.md).
* **Seat launch** - any role launches on any of its harnesses through the daemon's `launch` frame, beside running instances. A refused or failed launch says which, with the daemon's reason.
* **Session terminal** - xterm.js, fit to its pane.
* **Activity** - a seat's creature spins a ring in its colour while output flows, and a seat that finishes off screen shows "done, your turn" with a dot, counted in the page title.
* **Alt-tab triage** - the window title names who is waiting, e.g. "(2) Frog-Ox asking // aterm", and an installed app badges its icon. Tabbing in opens the seat that most needs you with its answer focused, but only after the page was left for 2 seconds or more and never over a draft, so a phone keyboard or paste menu cannot swap the seat under a thumb, number keys answer, Enter submits a multi-select, and each answer moves to the next waiting seat.
* **Alerts** - two switches in the sidebar, both off by default and kept per browser: Sound plays one short tone, and Strong visual draws an edge gradient plus a tinted tab in each waiting seat's role colour until it is opened (static under reduced motion). A cue fires when a seat enters waiting, so a seat already waiting at connect, the seat open on a visible tab, and asks a read-only browser cannot answer stay quiet. A browser holding audio back shows a notice on the switch (COI-2491).
* **ask_choice** - a seat's structured `ask_choice` call shows as the same card, single or multi-select with an optional typed answer, and flags the seat "asking you" with a ? badge from any tab. The Demo host scripts two asks.
* **Native choices** - when a harness shows a select menu (Claude Code's AskUserQuestion, its trust prompt), a card offers the question and options as buttons, a typed answer for "Type something.", and Cancel. Picking one sends the harness its own keys.
* **Composer** - a real text field under the terminal, so dictation tools like Wispr Flow and phone keyboards work. Enter sends as one bracketed paste, with Enter a beat later, when the program asked for it. Typing `@` and a prefix offers the live sessions from the sidebar's list, with role and identity. Up and Down choose, Tab or Enter inserts the full name, Escape closes. The draft is kept per seat, outside the field, so a lock or seat change never takes it. Typed input the daemon would not take, or that met a closed socket, is said aloud above the field. While a seat works and the field is empty, Send becomes Stop in the same slot: a tap sends Escape, a touch hold sends Control-C, and Shift+Enter does from a keyboard. A draft keeps it Send. It adds no daemon verb.
* **Peer message marking** - envelope lines are matched in the terminal buffer and striped in the sender's colour, across wrapped rows.
* **Messages** - each seat's sent and received messages with queued, held, launching, typed in, and failed states, and the daemon's reason.
* **Views and Browser tabs** - MCP Apps views from a daemon that lists `mcp-apps`, and the seat's streamed browser, awaiting its frames. [views-and-browser.md](views-and-browser.md).
* **Phone layout** - below 720px one switcher bar names the seat and holds the seats, alerts, roles to start, and hosts. At 412 by 800 the box shows 30 rows with the keyboard closed and 16 open, with one text input. [architecture.md](architecture.md) says how the harness's own box is hidden.
* **PTY size** - when the daemon lists `pty-size`, a terminal reports its box's capacity (`fit.proposeDimensions()`) with `scales: true` on attach, and sets xterm to exactly the `size` frame the daemon answers with. A PTY larger than the box pans, anchored bottom-left, so the input and newest text stay in view. An older daemon keeps fitting and sending.
* **Read-only state** - when the daemon's typing guard refuses this browser (`welcome.typing`, or an `error` with `session_descendant` or `peer_unread`), the terminal, composer, choice cards, and launch buttons say read only and send nothing.
* **Passkey unlock** - a locked remote device enrolls and asserts a passkey from the hosted build, then types. [passkey.md](passkey.md).
* **Terminal tab and split** - a plain shell beside a seat, a resizable split, and the side panel opening on a tab by role (sysadmin Terminal, frontend Browser). [terminal-split.md](terminal-split.md).
* **Hosted build** - the client under a path prefix behind a deployment's own sign-in, hosts added per device. [deploy.md](deploy.md).
* **Installable** - a manifest, icons, and an app-shell service worker, so Chrome installs it from the daemon's address or the hosted build into its own window. With no daemon answering, a launch opens to the "not answering" panel instead of a browser error. An install card on the host panel says what installing does and hides once installed. [deploy.md](deploy.md).

Designed, not built: the switchboard.

## See also

* [running.md](running.md) - what this is and how to run it.
* [SKILL.md](../SKILL.md) - the rules for changing the client.
