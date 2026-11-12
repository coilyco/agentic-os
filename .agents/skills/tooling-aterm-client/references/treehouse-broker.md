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

## Enrolling, renewing, and telling the broker

* **Enroll** - `aterm treehouse enroll <code> --treehouse-accounts <https> --treehouse-dir <dir>`. The code comes from a signed-in, user-verified session at accounts. The key is made on this host, and nothing is written until the certificate verifies against the accounts key set and binds that key (`cnf.jkt`). A directory that already holds an identity needs `--replace`.
* **Provisioned claim** - a target Treehouse starts itself, such as kai-server's pod, passes `--treehouse-provision-secret-file` (`ATERM_TREEHOUSE_PROVISION_SECRET_FILE`), plus `--treehouse-name` and `--treehouse-substrate`. With no `daemon.cert` the daemon makes its key first, then claims at accounts `/v1/daemons/provision` and retries with backoff from 5 seconds to 5 minutes until accounts accepts, so it can start before its owner signs up or before the secret file arrives. A retry reuses the key. Nothing is typed on the target, the secret is never logged, and a daemon that already holds a certificate never claims. Flow: treehouse-servers `docs/contract-flows.md`, "Enroll a provisioned target".
* **Renew** - the daemon renews 10 minutes before the certificate expires, replaces `daemon.cert` atomically, and sends `reg.cert` on the link. A certificate already lapsed at start is renewed before the first dial, since accounts takes a lapsed one when the proof holds. A failed renewal is logged and retried every 30 seconds.
* **Seat events** - a seat that finishes a turn or waits on a prompt, and each `ask`, goes to the broker as `reg.event` with the seat's name, role and identity. The ask's question is sent only when the owner's `push.question_text` setting is on, and then as its first 140 characters. See the next section. With no link up an event is dropped, and the broker's reachable push covers the return. The daemon's own Web Push for the local path is unchanged.

## The question-text setting

* **Where it comes from** - the snapshot `agent-compose` publishes (`snapshot.json` in `--treehouse-config-dir`, else `AGENT_COMPOSE_CONFIG_DIR`, else `~/.agent-compose/config`). The daemon reads the file itself, since the writer lives in a private repository. It checks the format and the sha256 of the config as compact JSON, and a snapshot that fails keeps what was loaded, else the last good one, else off.
* **Who** - the setting for the daemon's owner account id, else the one for `*`, else off. Turn it on with `agent-compose config-source set <acct id> push.question_text true`.
* **When** - the file is statted at each event, so a change applies to the next event with no restart. Off, or no snapshot, `question` never leaves the daemon. `done` and `prompt` events never carry one.
* **Held to the writer** - `aterm/testdata/treehouse-config-snapshot.v1.json` was written by the real `EncodeSnapshot`, and a test reads it. If the writer's hash changes, regenerate that file.

## The client

The page side is in `aterm-client/` (COI-3392), replayed from coilyco/treehouse-servers, which keeps the server code and the testbed. It is off unless a build sets both `VITE_TREEHOUSE_ACCOUNTS_URL` and `VITE_TREEHOUSE_BROKER_URL`, and the direct path is unchanged.

* **What a person sees** - a card on the host screen: "Sign in to see your machines" signed out, "Your machines" signed in with each machine online or offline, a Check again button, and Sign out. An offline machine has its own panel. A refused sign-in or listing says why in words.
* **How it connects** - `broker.ts` lists the account's daemons (`GET /v1/daemons`) and asks for a one-use ticket (`POST /v1/connect`). `daemon-host.ts` dials the ticket URL and, on a welcome with `unlock` and the `unlock-token` feature, sends `{type: "unlock", unlock_token}` with a token from accounts. Three refusals redial for a new nonce. Every dial takes a new ticket.
* **Unlock** - `session` first, then the passkey method only on a 403 `forbidden`. The passkey method cannot run in an app, whose origin is outside the RP ID.
* **The session** is held in memory only. A sign-in that accounts ends sends the page back to the card.
* **Sign in from an app or the web** - the system-browser hop, in [system-browser.md](system-browser.md).
* **Not replayed here** - `accounts.test.ts`, `broker.test.ts` and `handoff.test.ts` hold the client to the contract schemas in treehouse-servers, so they run there, and so does the testbed against the real accounts and broker (`just testbed`). The cutover (COI-2924) brings them back.

## Not built yet

Nothing ran against the real broker or accounts. Device-key unlock is not offered by accounts yet, so a brokered channel unlocks by passkey or session.
