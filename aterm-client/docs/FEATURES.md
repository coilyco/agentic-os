# Features

A coarse inventory of what ships. Architecture: [architecture.md](architecture.md).

* **Sidebar tabs** - hosts and seats as vertical tab lists with arrow-key focus, collapsing to two scrolling strips below 720px.
* **Host states** - online, not answering, and sign-in required, each with its own panel. Adding a host by address waits on the daemon.
* **Seat launch** - a seat that is not running opens a launch panel for each of its harnesses, and a failed launch shows its exit reason.
* **Session terminal** - xterm.js with fit-to-pane resizing and the role's accent.
* **Peer message marking** - envelope lines are matched in the terminal buffer and striped in the sender's colour, across wrapped rows.
* **Messages panel** - each seat's sent and received messages with typed in, queued, held, and bounced states.
* **Installable** - a web manifest and icons so Android and the Mac can install it as an app.

Designed, not built: MCP Apps views, the shared browser, and the switchboard.

## See also

* [README.md](../README.md) - what this is and how to run it.
* [AGENTS.md](../AGENTS.md) - agent operating rules for this repo.
