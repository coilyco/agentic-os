# A seat on a Linux host: what the listeners allow (COI-3498)

Proof for one capped seat user on kai-server, kept host-neutral for the cloud target (COI-3469).

**The only TCP listeners in the aterm package are the loopback websocket and the tailnet one.** The daemon socket and the session holders are unix sockets. An empty `ATERM_DAEMON_WS` and `ATERM_DAEMON_TAILNET_PORT` turn both off, and a non-empty value turns one on, checked on macOS with `lsof` against a running daemon. A daemon an `aterm` launch starts inherits that environment and takes no listener flags, so set both on the seat's unit, and run `aterm daemon --websocket "" --tailnet-port ""` as its start so none is auto-started.

**Any process that can reach a listener could `spawn` any argv.** The loopback websocket treats a peer with no session above it as a browser on this host. The tailnet listener did too when the peer dialed the node's own tailnet name, because a source address that is one of the host's own addresses counted as local, so `remoteLocked` never applied. The tailnet policy now marks every peer remote, so an unauthenticated `spawn` gets `passkey_required` (`TestTailnetPeerOnThisHostCannotSpawnWithoutUnlocking`).

**The loopback websocket is still open to a local process.** Keep it off for a seat user, or put a firewall rule in front.

**What the tailnet change does not cover.** A locked remote peer can still attach and read seat screens. It reaches the host with a tailnet identity and no passkey.

**Probe.** `scripts/aterm_listener_probe.py HOST PORT [--tls] [--insecure] --marker PATH` sends `hello` then a `spawn` that touches the marker. Exit 0 means refused, 1 means the spawn ran, 2 means no verdict. Run it as the seat user and as a different local user, against each listener.

## The kai-server pilot (COI-3498)

**The daemon runs as `kai` with the loopback websocket off and the tailnet listener on**, so Kai's client reaches it over the tailnet and a passkey gates typing. Other local users share the host, which is why loopback stays off.

* **Install** `aterm-linux-amd64` from a release at or after `aos-v0.557.0`, the first with the tailnet fix (0b61d321), and check it against that release's `SHA256SUMS`.
* **Unit.** A user service running `aterm daemon --websocket "" --tailnet-port 7419 --idle 0` with `ATERM_DAEMON_WS=` set empty in the unit as well. It carries no cgroup properties, since a user scope cannot cap CPU there. The operator caps `user-1000.slice` for the pilot day.
* **Check** with `ss -ltnp`. The only aterm listener is the tailnet address on 7419, and nothing on 127.0.0.1.
* **Acceptance.** `scripts/aterm_listener_probe.py NAME 7419 --tls --marker PATH`, run on the host, exits 0. Exit 1 means stop the daemon and enroll nobody. Exit 2 means no verdict, usually admission or the certificate.
* **agent-compose** is a private release, and the ansible role still installs it from the old brew formula. Fetch `agent-compose-v2.220.0` from coilyco/treehouse-servers with `gh release download` and run `scripts/ci/install-agent-compose.sh` with `AGENT_COMPOSE_BASE` and `AGENT_COMPOSE_DEST` set, which checks the pinned SHA-256s. Point aterm at it with `AGENT_COMPOSE_BIN`, and call `aterm` by absolute path.
* **The pilot seat is `prod-manager goose`.** The launch profile pins provider `openai` and model `chat/default`, served by Agent Proxy, which answers `/v1/models` with no key. Install goose from its GitHub release for the fleet's version, checked against the asset's published digest, with `OPENAI_HOST` and `OPENAI_BASE_PATH` in `~/.config/goose/config.yaml` as the goose-config role sets them, and `GOOSE_DISABLE_KEYRING=1` on a headless host.
* **The seat** starts with `aterm prod-manager goose --headless --daemon --working-directory /srv/data/pilot-seat`. Whether composing needs the agent-compose config and agentic-os checkouts that the ansible role renders is not yet known: `agent-compose launch --spec-out FILE prod-manager goose` checks it without starting a seat.
