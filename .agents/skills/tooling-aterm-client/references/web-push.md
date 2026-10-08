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

**The daemon sends whether or not a page is attached.** A phone that sleeps leaves a websocket the daemon cannot tell from a live one, so counting attachment would suppress the push this exists for. What to show while a window is open is the service worker's call, and it always shows one: Chrome adds its own notice for a push that shows none, and no documented exemption for a visible page turned up. A visible, focused window already cues itself, so that notification is `silent` and the `tag` replaces repeats. Jev chose this over always-audible and over showing nothing (0.93, confidence 0.9).

## Frames

* `push_key` answers `push_key` with `key`, the public key to pass as `applicationServerKey`.
* `push_subscribe` takes `push`, the browser's `PushSubscription.toJSON()` as it is, and answers `push_subscribed`. `push_unsubscribe` takes `push.endpoint` and answers `push_unsubscribed`.
* **A remote device needs its passkey** for both, as for typing. The relying party is `coilyco.dev`, so only the page served there can subscribe.
* **An endpoint must be https on port 443** at `fcm.googleapis.com` or under `.push.services.mozilla.com`, `.push.apple.com` or `.notify.windows.com`. Anything else is refused, since the endpoint is a URL the daemon posts to.
* **The payload** is JSON, `kind` (`done`, `prompt`, `ask`), `session`, `role`, `identity`, `title`, `body`, and `tag` (`aterm-<session>`, so a second push replaces the first). It is encrypted to the browser under RFC 8291, so the push service sees ciphertext. TTL is 600 seconds, urgency high.
* **A 404 or 410** from the push service means the browser dropped the subscription, and the daemon forgets it.

## Built on the standard library

Encryption is RFC 8291 and the token RFC 8292 on `crypto/ecdh`, `crypto/hkdf` and `crypto/ecdsa`, with no dependency. `webpush-go` was read first. It has one maintainer, its last tag is January 2025, and later fixes sit untagged on its main branch. One test replays the RFC 8291 example byte for byte, and another has a stand-in browser open a sent message.

## The client

* **The switch** - Notify when closed, in the Alerts group beside Sound and Strong visual, off by default and shown only when the welcome lists `web-push`. A tap asks for notification permission first, since some browsers only prompt inside the tap. It then sends `push_key`, calls `pushManager.subscribe`, and sends `push_subscribe`. Off sends `push_unsubscribe`, then drops the browser's subscription even when the daemon could not be told, since a dead endpoint answers 410.
* **Its states** - off, working, on, and a refusal that leaves it off and says why: a blocked site, a dismissed prompt, a push service the browser could not reach, a daemon refusal, and a page that cannot receive push at all (insecure, no worker, a dev build, an iPhone not yet on the Home Screen). A locked remote device is told before any prompt, and the hosted client says to unlock while a daemon-served page says passkeys only work at coilyco.dev.
* **On means** - this browser holds a subscription and `aterm.push.v1` in local storage lists the host it subscribed through. A subscription is bound to one key, so a host with another key replaces it.
* **The worker** - `pwa/sw.js` shows the payload's `title` and `body` under its `tag`, and a payload that is not that still shows a generic notice. A tap focuses an open window and messages it the seat, or opens the app at `?seat=<session>`.
* **Opening the seat** - the page attaches the host this browser subscribed through, waits 15 seconds from attach for it to list the seat, then selects it. Opening a seat closes its notification.

## Verified and not

* **Against a private daemon** with a test key and its own state directory, in Chromium: the frames, the switch off to on to off with the daemon's store and the browser's subscription agreeing, a blocked site at 320px, the worker's notification with a window focused (silent) and with none open (audible), a click opening the seat, and a cold launch at `?seat=` attaching the host and opening it.
* **Not verified: a delivered push.** The daemon's own `ask` push, sent through the push service to that browser, did not arrive within 30 seconds. The worker was driven with the same payload through the DevTools protocol instead. Whether the daemon's send or the push service to an automation browser fell short is not settled.
* **Not verified: a real device**, a real tap on an OS notification, and a tailnet-only phone. The open question from COI-2529 stands.
* **Not built:** a host hint in the payload, so a browser subscribed through two daemons cannot tell which a tap belongs to, and a worker `pushsubscriptionchange` handler.
