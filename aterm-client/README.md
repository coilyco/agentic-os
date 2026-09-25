# aterm-client

The window onto aterm agent sessions, for the web, Android, and the Mac. Sessions run on a host under the aterm daemon. This client attaches to them.

## What it does

* Hosts and seats sit as tabs in a sidebar. Pick a host to attach, then a seat to open its terminal or launch it.
* Each session's terminal renders in xterm.js, with messages from other seats marked in the sender's colour.
* A messages panel lists what each seat sent and received, and whether it was typed in, queued, held, or bounced.

MCP Apps views and the shared browser are designed but not built yet. See [docs/FEATURES.md](docs/FEATURES.md).

## Running it

```sh
just install
just dev        # http://localhost:5173, against the mock host
just gate       # check, test, build
```

The client talks to a mock host until the daemon lands, so every session and message on screen is scripted. See [docs/architecture.md](docs/architecture.md).

## License

MIT.

## See also

* [AGENTS.md](AGENTS.md) - agent operating rules for this repo.
* [docs/FEATURES.md](docs/FEATURES.md) - what ships today.
