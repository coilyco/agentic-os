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
