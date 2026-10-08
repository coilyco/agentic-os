# Running the aterm client

The window onto aterm agent sessions, for the web, Android, and the Mac. Sessions run on a host under the aterm daemon in [`aterm/`](../../../../aterm), beside `aterm-client/`. This client attaches to them.

## What it does

* Hosts, running sessions, and roles to start sit as tabs in a sidebar. Every session is its own tab with its harness named, so instances of one role on different harnesses sit as peers.
* Each session's terminal renders in xterm.js, with messages from other seats marked in the sender's colour.
* A messages panel lists what each seat sent and received, and whether it was typed in, queued, held, or bounced.

It talks `aterm.daemon.v1` to the aterm daemon's loopback websocket, so "this Mac" is the real thing. MCP Apps views and the shared browser are designed but not built yet. See [features.md](features.md).

## Alt-tab use

Made for tabbing out of a game to answer seats. The window title says who is waiting, tabbing in lands on the seat that needs you with its answer focused, and a number key answers and moves on.

From another machine on the tailnet, such as a gaming PC: open `https://<mac tailnet name>:7419/` and install it as an app for its own alt-tab entry and taskbar badge. The daemon serves it over HTTPS and admits only your own devices, by `tailscale whois`.

## Running it

```sh
just aterm-client-install
just aterm-client-dev    # http://localhost:5173
just aterm-client-gate   # check, test, build
```

In dev, "this Mac" needs `aterm daemon` running (any `aterm` launch starts it), and `VITE_ATERM_DAEMON_WS` points the client at another port. A build dials the origin that served it, so a daemon serving `dist/` needs no configuration, and the host tab names that machine. The Demo host is scripted and needs nothing. See [architecture.md](architecture.md).

## License

MIT.

## See also

* [SKILL.md](../SKILL.md) - the rules for changing the client.
* [features.md](features.md) - what ships today.
