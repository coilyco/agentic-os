# Web Push for a waiting seat

**The daemon sends a Web Push when a seat starts waiting, so a closed app or a sleeping phone still notifies.** An open page needs none of it, because the [client](../../../../aterm-client/README.md) cues itself. The service worker half is the client's, and this page is the contract it builds against. The [daemon](../../../../docs/aterm-daemon.md) owns keys, subscriptions and sending.

## Turning it on

* **Make a key** - `aterm vapid --out FILE` writes a P-256 key at mode 0600, refuses an existing file, and prints only the public half.
* **Keep it in SSM** - the file is the secret, stashed at `/coilysiren/aterm/vapid-key` and never tracked.
* **Feed the daemon** - `ATERM_VAPID_KEY` (or `--vapid-key`) in `daemon.env`, the way the Sentry DSN arrives. Rolling it out to a host is ansible's.
* **Without a key** - push is off, `web-push` is absent from the welcome features, and the `push_*` frames answer an error.
* **Subscriptions** live in `push-subscriptions.json` beside the passkeys at mode 0600, capped at 32.

## What fires

* **done** - a seat went busy then idle and stayed idle for 4 seconds.
* **prompt** - a permission or choice card holds the seat for 4 seconds.
* **ask** - an `ask` frame arrives, at once, with the question as the body.
* **Baseline** - a seat already waiting when the daemon first sees it never fires. A second done or prompt about one seat inside 30 seconds is held.

**The daemon sends whether or not a page is attached.** A phone that sleeps leaves a websocket the daemon cannot tell from a live one, so counting attachment would suppress the push this exists for. What to show while a window is open is the service worker's call. Chrome shows a default notice for a push that shows none, and no documented exemption for a visible page turned up, so the worker should show one and let the `tag` replace repeats.

## Frames

* `push_key` answers `push_key` with `key`, the public key to pass as `applicationServerKey`.
* `push_subscribe` takes `push`, the browser's `PushSubscription.toJSON()` as it is, and answers `push_subscribed`. `push_unsubscribe` takes `push.endpoint` and answers `push_unsubscribed`.
* **A remote device needs its passkey** for both, as for typing. The relying party is `coilyco.dev`, so only the page served there can subscribe.
* **An endpoint must be https on port 443** at `fcm.googleapis.com` or under `.push.services.mozilla.com`, `.push.apple.com` or `.notify.windows.com`. Anything else is refused, since the endpoint is a URL the daemon posts to.
* **The payload** is JSON, `kind` (`done`, `prompt`, `ask`), `session`, `role`, `identity`, `title`, `body`, and `tag` (`aterm-<session>`, so a second push replaces the first). It is encrypted to the browser under RFC 8291, so the push service sees ciphertext. TTL is 600 seconds, urgency high.
* **A 404 or 410** from the push service means the browser dropped the subscription, and the daemon forgets it.

## Built on the standard library

Encryption is RFC 8291 and the token RFC 8292 on `crypto/ecdh`, `crypto/hkdf` and `crypto/ecdsa`, with no dependency. `webpush-go` was read first. It has one maintainer, its last tag is January 2025, and later fixes sit untagged on its main branch. One test replays the RFC 8291 example byte for byte, and another has a stand-in browser open a sent message.

## Not yet verified

* **A real device.** No phone has received one. The open question from COI-2529 stands: whether a tailnet-only phone gets pushes. The browser reaches its push service over the internet and the daemon host needs outbound https to it, but only a phone test settles that.
* **The client.** The service worker `push` handler, the opt-in control, and the subscribe call are frontend work against these frames.
