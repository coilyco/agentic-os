# Why a device cannot reach the daemon

How the client and daemon name the layer that kept a browser out (COI-2597). The daemon half is in [aterm-daemon.md](../../../../docs/aterm-daemon.md).

**A refused websocket shows a page nothing.** Script gets a failed socket with no status and no body, so the client asks over plain HTTPS first. `GET /_aterm/reach` answers an admitted device `{"layer":"ok"}`. A refusal is 403 with its layer in `X-Aterm-Refusal` (`name`, `ownership` or `origin`) and the reason as the body. Only a page the daemon would give a socket can read either answer, through CORS.

**The client names five outcomes**, in `src/lib/reach.ts`.

* `ownership` - the device's tailnet login or tags are not admitted. A retry cannot help.
* `name` - the page used an address that is not the daemon's tailnet name.
* `origin` - the daemon answered and refuses this page's origin.
* `answered` - an opaque `no-cors` request got a reply, so the name, TLS and route work, but the page may not read it. An older daemon sends no CORS header, so its refusal lands here, with the URL to open in a tab, where it prints its reason.
* `network` - both asks failed. A browser cannot split DNS (Private DNS can bypass MagicDNS), the tailnet route, Chrome's local network block, TLS and a stopped daemon, so the message says so and points at Chrome's own error code.

**The daemon logs each refusal once per peer, layer and reason in five minutes**, as `refused ownership from <peer> (origin "..."): <reason>` in `aterm-daemon.log`. Before this a refusal logged nothing, so a log with no line proved nothing.

**A refusal is remembered for 2 seconds, an admission for a minute.** A page load makes several requests and each uncached one runs `tailscale whois`, so a refusal is kept just long enough for one load. A phone tagged a moment ago is admitted on its next load. With a minute for both, Kai needed about six reloads (COI-2597).

**A tagged node has no owner.** Tailscale lends it a login named after the node, so the owner rule matches no person and only `tag:physical` devices attach. An untagged phone is refused as `ownership` until it carries that tag. The startup line then says "no untagged device" in place of "its own owner".

## The connection check, hop by hop (COI-3256)

**The host panel checks six hops in order and says which broke**, from `src/lib/hops.ts`. It runs by itself for a host that does not answer or a link that stays down, on demand for any daemon host, and from the banner's Check connection. One button copies the report as a message to prod-director, and a page that may not write the clipboard shows it selected instead. Each hop says whether the page **saw** it or **inferred** it.

* 1 name resolves, 2 tailnet route, 3 TLS - **a page cannot tell these apart.** Refused, reset, a bad certificate and a name that did not resolve all reach `fetch` as one `TypeError`, so the panel says "hop 1, 2 or 3". A request that hangs is an `AbortError`, which it reads as the route. Chrome's own error page, in a tab, names the exact failure.
* 4 HTTP - seen, with the status, when the daemon lets the page read the reach answer. Otherwise only that something answered.
* 5 websocket upgrade - a refused upgrade shows close code 1006 and no status or body.
* 6 daemon admits this device - the refusal layer on the reach answer, or the `welcome` frame and its error. A passkey or device-key lock only makes typing read only, so it is not a connection failure.

**Measured, not assumed.** `just aterm-client-drive-hops` points the real client at `scripts/hop_daemon.py`, a mock daemon that breaks one hop per scenario on real sockets. It compares the panel's verdict with the `net::ERR_*` code the browser reports to the test runner, which a page never reads. It disproved an idea: Chromium 147 leaves a Resource Timing record for an `https` request whose name did not resolve, so a missing record cannot stand for DNS. Only Chromium has run (COI-3266 covers WebKit, Firefox and a real phone).

**The daemon serves HTTPS on the tailnet address itself, with a `tailscale cert` certificate, and a Tailscale Serve front on 443 is a second way in** ([tailnet-access.md](tailnet-access.md)). So "nothing is serving HTTPS" means the listener the page dialled is unbound or failing its handshake, which the daemon logs, or no Serve front exists. A daemon whose binary an upgrade replaced resets every handshake while it looks bound, which is the "connection was reset" a page sees.

**Where a daemon would help, not built.** A daemon the page can reach could dial the silent one and report Go's own error per hop (COI-3265).
