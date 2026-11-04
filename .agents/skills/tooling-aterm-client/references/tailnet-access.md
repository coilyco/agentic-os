# Tailnet access

How [the aterm daemon](../../../../docs/aterm-daemon.md) decides which device on the tailnet may attach. `tailscale whois` names the peer, and nothing the request says counts.

## Who is admitted

**An untagged device attaches when its owner login is one the daemon admits**: this node's own owner, and each login in `--allow-owners` (env `ATERM_DAEMON_ALLOW_OWNERS`, comma-separated, case-insensitive). A tagged node has no owner of its own, so with none configured no untagged device attaches. **A device tagged `tag:physical` attaches** (`--allow-tags`), and a tagged device claiming an owner's login does not. The passkey gate still applies to every one of them.

**A websocket opens only from the page the listener served, `https://coilyco.dev`, or the Android app's origin**, and the origin must be https.

## A Serve front is never loopback

**`tailscale serve --bg http://127.0.0.1:7419` puts HTTPS on the node's name at 443.** Serve proxies to the loopback listener, so the socket the daemon sees is the proxy's. Judged as loopback, every device Serve admits would type with no passkey.

**A request with any proxy header counts as proxied**: `X-Forwarded-*`, `Forwarded`, `X-Real-Ip`, `Tailscale-*`. Serve always sets `X-Forwarded-Host` and replaces a client's `X-Forwarded-For`, and a browser on this host sends neither. A local process that fakes one is held to more, never less.

**A proxied request is judged by whois on the peer `X-Forwarded-For` names**, with the admission above. It is refused when whois fails, the peer is not one non-loopback address, the Host is not this node's name (a port is allowed), `X-Forwarded-Host` differs from the Host, or it is a Funnel request. `Tailscale-User-Login` must equal the login whois names. Serve strips a client's copy and sends none for a tagged peer, so a tagged peer with the header, or an untagged one without it, is refused. A proxied request is a remote device and types only after a passkey.

**Unset `forwardedPolicy`, or a node tailscaled will not describe, refuses every proxied request.**

## The direct listener and a replaced binary

**The tailnet listener completes a TLS handshake against itself every 30 seconds and binds again after two failures.** A daemon whose binary an upgrade removed stops instead, and launchd starts the installed build. macOS drops the flows of a process whose file is gone, so every handshake resets (`socket is not connected`, `EOF`) while the listener looks bound.

**The file checked is the one resolved when the daemon started.** The launch path is a Homebrew symlink an upgrade repoints at the new version, so a stat of it passes while the version the process runs is gone. A daemon older than this check needs one restart onto the installed build.

**Host discovery dials each peer on this port**, so the direct listener stays with a Serve front beside it.
