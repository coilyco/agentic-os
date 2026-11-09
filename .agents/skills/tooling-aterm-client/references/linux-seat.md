# A seat on a Linux host: what the listeners allow (COI-3498)

Proof for one capped seat user on kai-server, kept host-neutral for the cloud target (COI-3469).

**The only TCP listeners in the aterm package are the loopback websocket and the tailnet one.** The daemon socket and the session holders are unix sockets. An empty `ATERM_DAEMON_WS` and `ATERM_DAEMON_TAILNET_PORT` turn both off, and a non-empty value turns one on, checked on macOS with `lsof` against a running daemon. A daemon an `aterm` launch starts inherits that environment and takes no listener flags, so set both on the seat's unit, and run `aterm daemon --websocket "" --tailnet-port ""` as its start so none is auto-started.

**Any process that can reach a listener could `spawn` any argv.** The loopback websocket treats a peer with no session above it as a browser on this host. The tailnet listener did too when the peer dialed the node's own tailnet name, because a source address that is one of the host's own addresses counted as local, so `remoteLocked` never applied. The tailnet policy now marks every peer remote, so an unauthenticated `spawn` gets `passkey_required` (`TestTailnetPeerOnThisHostCannotSpawnWithoutUnlocking`).

**The loopback websocket is still open to a local process.** Keep it off for a seat user, or put a firewall rule in front.

**What the tailnet change does not cover.** A locked remote peer can still attach and read seat screens. It reaches the host with a tailnet identity and no passkey.

**Probe.** `scripts/aterm_listener_probe.py HOST PORT [--tls] [--insecure] --marker PATH` sends `hello` then a `spawn` that touches the marker. Exit 0 means refused, 1 means the spawn ran, 2 means no verdict. Run it as the seat user and as a different local user, against each listener.

**The Linux seat job runs it on Ubuntu.** `.github/workflows/aterm-linux-seat.yml` runs `scripts/ci/aterm-linux-seat.sh` on dispatch and on the listener code paths. It runs a daemon as an unprivileged user with both listener variables empty, asserts with `ss` that the user holds no TCP listener (a control with the websocket on must show one), runs a stub seat that takes an image, and checks a second uid cannot reach the socket. It cannot prove the tailnet listener, a real agent harness, or kai-server's own limits.
