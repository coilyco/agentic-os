# aterm-client

The window onto aterm agent sessions, for the web, Android, and the Mac. Sessions run on a host under the aterm daemon. This client attaches to them.

## What it does

* Hosts and seats sit as tabs in a sidebar. Pick a host to attach, then a seat to open its terminal or launch it.
* Each session's terminal renders in xterm.js, with messages from other seats marked in the sender's colour.
* A messages panel lists what each seat sent and received, and whether it was typed in, queued, held, or bounced.

It talks `aterm.daemon.v1` to the aterm daemon's loopback websocket, so "this Mac" is the real thing. MCP Apps views and the shared browser are designed but not built yet. See [docs/FEATURES.md](docs/FEATURES.md).

## Running it

```sh
just install
just dev        # http://localhost:5173
just gate       # check, test, build
```

"this Mac" needs `aterm daemon` running (any `aterm` launch starts it). `VITE_ATERM_DAEMON_WS` points the client at another port. The Demo host is scripted and needs nothing. See [docs/architecture.md](docs/architecture.md).

## License

MIT.

## See also

* [AGENTS.md](AGENTS.md) - agent operating rules for this repo.
* [docs/FEATURES.md](docs/FEATURES.md) - what ships today.
