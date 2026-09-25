# aterm-client

The window onto aterm agent sessions, for the web, Android, and the Mac. Sessions run on a host under the aterm daemon. This client attaches to them.

## What it does

* Hosts and seats sit as tabs in a sidebar. Pick a host to attach, then a seat to open its terminal or launch it.
* Each session's terminal renders in xterm.js, with messages from other seats marked in the sender's colour.
* A messages panel lists what each seat sent and received, and whether it was typed in, queued, held, or bounced.

It talks `aterm.daemon.v1` to the aterm daemon's loopback websocket, so "this Mac" is the real thing. MCP Apps views and the shared browser are designed but not built yet. See [docs/FEATURES.md](docs/FEATURES.md).

## Alt-tab use

Made for tabbing out of a game to answer seats. The window title says who is waiting, tabbing in lands on the seat that needs you with its answer focused, and a number key answers and moves on. Using it from another machine, such as a gaming PC, waits on the daemon listening past loopback.

## Running it

```sh
just install
just dev        # http://localhost:5173
just gate       # check, test, build
```

In dev, "this Mac" needs `aterm daemon` running (any `aterm` launch starts it), and `VITE_ATERM_DAEMON_WS` points the client at another port. A build dials the origin that served it, so a daemon serving `dist/` needs no configuration, and the host tab names that machine. The Demo host is scripted and needs nothing. See [docs/architecture.md](docs/architecture.md).

## License

MIT.

## See also

* [AGENTS.md](AGENTS.md) - agent operating rules for this repo.
* [docs/FEATURES.md](docs/FEATURES.md) - what ships today.
