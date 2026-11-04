# The Treehouse broker path: a brokered channel and its unlock token

The daemon can dial the Treehouse broker and serve each client the broker attaches. The contract is `docs/contract.md` and `contract/` in coilyco/treehouse-servers, and the code is `aterm/treehouse_token.go`, `treehouse_broker.go` and `treehouse_users.go` (COI-3358). It is off unless all three settings are given, and the local listeners are untouched.

## Settings

`aterm daemon --treehouse-broker <wss address> --treehouse-accounts <https address> --treehouse-dir <dir>`, or `ATERM_TREEHOUSE_BROKER`, `ATERM_TREEHOUSE_ACCOUNTS`, `ATERM_TREEHOUSE_DIR`. The directory holds `daemon.key` (P-256 PEM, mode 0600, refused otherwise) and `daemon.cert` (the accounts-signed certificate). A missing or invalid piece logs why and leaves the brokered path off. The accounts address is the issuer every token must name.

## How a channel unlocks

* **Open** - the broker sends `chan.open` for the daemon's owner account. A channel for another account is turned away, and at most 8 are open.
* **Welcome** - the client's `hello` is answered as on any socket, plus an `unlock` member `{daemon_id, nonce}` and feature `unlock-token`. The channel is a remote web connection, so it starts `passkey_required`.
* **Unlock** - the client gets an accounts token for this daemon and nonce and sends `unlock` with `unlock_token`. The daemon checks the signature (ES256 only, key from the accounts key set), `typ`, `iss`, `use`, `aud`, `sub` against its owner, the nonce, `acr` `uv`, a life of at most 120 seconds, and that the `jti` is unspent. Success sets the same flag a passkey assertion sets and pushes `typing`.
* **Refuse** - every failure answers reason `unlock_refused`, logs the failed check without the token, and five refusals end the channel's nonce. Passkey, device key, resume and push frames are refused on a brokered channel.
* **Fail closed** - with no accounts key set younger than a day, every token is refused.

## Not built yet

Enrollment (`aterm enroll`), certificate renewal, and sending `reg.event` for the broker's push are not in the daemon yet. Nothing ran against the real broker or accounts.
