# Host discovery

A served client lists the other tailnet nodes running an aterm daemon, with no typed name (COI-2494). The `hosts` frame is part of [the daemon's wire contract](../../../../docs/aterm-daemon.md).

## What the daemon answers

**`hosts` lists a peer only if a browser on this node's page could attach to it.** The daemon reads its own `tailscale status --json`, keeps the online macOS and Linux peers, dials each on the tailnet port with the websocket this node's served page opens, `Origin` included, and reads the `welcome`. Each answer carries `name`, `host`, `port` and `version`. Absent, never an error frame: a peer with no daemon, one that refuses this node by `tailscale whois`, one that refuses this node's page, and a different service.

**Discovery grants nothing.** The client dials each listed host itself, so that peer's whois and origin check judge the browser again. A peer lists this node only when its `ATERM_DAEMON_ALLOW_ORIGINS` names this node's page, `https://<node>.<tailnet>.ts.net:7419` with the port, since the origin policy admits another node's page no other way.

**Cost is bounded.** An answer is reused 30 seconds, at most 8 dials run at once, and each gives up after 4 seconds. An empty `--tailnet-port` turns discovery off, and an unreadable tailnet answers `error`, which leaves the connection open. The code is `aterm/daemon_hosts.go`.

## What the client does

`src/lib/discovery.ts` asks each online daemon host it lists, once its probe answers, and `newFoundHosts` merges the reply. A found host has id `found:<address>`, shows `found on tailnet` in the sidebar, and is never stored on the device. A host already listed by address, whether local, saved or found, is not listed twice. The client then probes each found host like any other, so one this browser cannot reach shows as not answering. Found hosts do not ask in turn. A daemon that predates the frame answers `error`, and the client adds nothing.
