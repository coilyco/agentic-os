# The aterm host daemon

`aterm daemon` routes every session [`aterm`](aterm.md) opens. The kitty window is one client of it.

## What the daemon owns

**`_session` hands the harness to the daemon instead of running it.** It sends a `spawn` with argv, environment, directory and window size, then attaches, the argv untouched.

**A session is named `<role>-<identity>`, and a live name refuses a spawn.** A concurrent instance takes the next pool name (`--identity` picks one), and only a full pool adds `-2`, `-3`.

**`list` reads each session's screen** and reports `state`: `starting`, `prompt` (a card is up), `busy` (recent output or the interrupt hint), else `idle`.

**`aterm clear` types the harness's clear command, unstamped.** Only Kai's client and the `prod-director` role may, never on the caller's own session, a prompt, or a seat with no known command (claude only). Without `--force` it refuses a busy session, a draft, or queued messages.

**A terminal is a login shell, not a seat.** `spawn` with `kind: "terminal"` and optional `cwd` runs `$SHELL -l` under a holder, named `terminal-<hex>`, with no token, and refuses every seat field, `argv` and `env`. It is absent from `sessions`, `aterm agents`, seat counts and every `send`, `status` and `clear` target. `list` and the `sessions` push carry `terminals` (`name`, `pid`, `started`, `clients`, `cwd`, `label`). A spawn `label` (feature `terminal-label`, up to 256 bytes) is opaque and kept by a restart. A seat refuses it. `attach`, `input`, `resize` and `close` take its name, an exiting shell ends it, a restart adopts it. A remote device gets `remote_terminal`, passkey or not.

**A session outlives its window.** `aterm close` types the harness's exit at an idle prompt, then SIGTERM after 10s, SIGKILL 3s on.

**A holder owns each session's terminal, so a daemon crash leaves sessions running.** See [holders, launch, Sentry](aterm-bundles.md#daemon-internals-launch-holders-and-sentry).

**The socket is `/tmp/aterm-<uid>/daemon.sock`, keyed by uid rather than `HOME`**, since a session shadow moves `HOME`. `ATERM_DAEMON_SOCKET` overrides it. The directory must be the user's alone, which is all of local client auth.

## `aterm send`

**The sender is stamped by the daemon, never declared.** Each spawn gets a fresh `ATERM_SESSION_TOKEN`, `send` presents it, and the daemon types `[from <role> <identity>] <body>`.

**A body cannot forge a second envelope**: a line opening with `[from ` gets a leading `\`.

**Targets resolve in tiers**: session name, role slug, identity, then harness. The first tier with a match wins, several in it refuse, named, and none exits 3. `--launch` on a role slug opens the role and holds the message up to three minutes. `--new` opens another instance.

**Delivery serializes with the keyboard**, one lock over every PTY write. A message is `queued` till it is ready, `held` (and those behind) for Kai's typing, a draft or a card, then `delivered` or `failed`.

**A bracketed-paste program gets one paste, then Enter 300ms later**, since an Enter inside reads as a newline. codex and goose turn it on at their prompt, so **ready means paste is on**. claude and opencode drop a paste before the prompt, so **ready means `Try "` or `Ask anything` showed**, or 20s passed. **`delivered` means opencode took Enter and claude's transcript shows the message**. Unseen for 90s is `failed`, and a mid-turn seat that took no Enter is [parked](../.agents/skills/tooling-aterm-client/references/parked.md).

**A process inside a session cannot type into one.** It walks the connecting pid's parents. It may send, stamped, but type only into a session it spawned.

**A websocket on this host gets the same walk**, its source port mapped to the pids holding it, so a browser a session started, or one the daemon cannot name, cannot type, answer, launch or clear. A browser outside every session types, an agent driving Kai's own Chrome included.

**A device on the tailnet types only after a passkey.** Its `input`, `answer`, `launch`, `clear`, `spawn`, `close`, `send`, `claim`, `ask`, `cancel_ask`, `gateway_add`, `view_call` and `view_close` refuse until an assertion with user verification succeeds on that connection, then `typing` pushes `allowed`. Only `https://coilyco.dev` may assert. A chain on Google's status list, or an unreadable list, refuses (`attestation_revoked`).

## Wire contract

`aterm.daemon.v1` is one JSON object per line over the socket, and one per text message over the websocket. Both sides open with `hello` and `welcome`, and a format mismatch refuses. A request's `id` is echoed on its reply or `error`.

* `spawn`, `attach`, `detach`, `input` and `output` (base64 `data`), `resize`, `exit` with `code`.
* `send` answers `sent` with the message state, waiting 3 seconds, or `wait` up to 120, for delivery unless `launching`. One not yet final earns the sender an inbox receipt `message <id> to <session>: <state>` with no body. `notify_idle` types `<session> is idle`, `is held at a prompt` or `ended`.
* `status` (with `lines`), `clear` and `close` (with optional `force`), each with a `target`, answer `status`, `cleared` and `closed` with the exit `code`. `claim` with a `session` base answers `claimed` with a free pool name held two minutes, and `peek` holds none.
* Seat-to-client frames: `image_upload` [saves an image](../.agents/skills/tooling-aterm-client/references/image-upload.md), `open_url` [opens a page](../.agents/skills/tooling-aterm-client/references/open-url.md), `artifact_add` [shows a source file](../.agents/skills/tooling-aterm-client/references/artifacts.md).
* `inbox` (feature `inbox`) returns the caller's own messages by `token`, marked read on return, `all` adding the read. Capped at 200, lost on restart.
* `list` answers `sessions`, each with `state`, `quiet_seconds`, `last_reply` and `context` ([the meter](../.agents/skills/tooling-aterm-client/references/context-meter.md)). `subscribe` to `sessions` pushes the roster on every change, and `message` events carry each state change and, to a typing client, [the body](../.agents/skills/tooling-aterm-client/references/message-text.md). A stalled subscriber drops after 5s.
* `welcome` to a websocket adds `typing-guard`, `passkey` and `typing` `{allowed, reason, passkey}`, `passkey` `enrolled` or `unenrolled`. A refusal's `error` carries `reason`: `session_descendant`, `peer_unread` or `passkey_required`.
* Passkey: `passkey_enroll_begin` (`enroll_code`) answers `passkey_enroll_options`, and `passkey_enroll_finish` (`credential`) answers `passkey_enrolled`. `passkey_assert_begin` answers `passkey_assert_options`, and `passkey_assert_finish` answers `passkey_asserted`.* `resume` (feature of the same name) carries an unlock across a redial: [frames and window](../.agents/skills/tooling-aterm-client/references/resume.md).
* `push_*` serves [Web Push](../.agents/skills/tooling-aterm-client/references/web-push.md).
* `hosts` lists tailnet daemons that answered ([discovery](../.agents/skills/tooling-aterm-client/references/discovery.md)).
* `roster` answers `aterm.roster.v1`, the launchable roles `aterm --list --json` prints. `launch` with a `role` and `seat` opens it headless.
* `ask` takes a `question`, `options`, `header`, `allow_other`, `multi`, `relay`. MCP `ask_choice` takes up to four `questions`, one card each. The daemon stamps the asker and pushes `ask`. `answer` (`ask_id`, `picks`, `text`) or `cancel_ask` settles it, and `asked` tells clients to drop it. An answer takes the typing guard as Kai's input.

**Browsers get the embedded client at `/`, and a websocket there.** Loopback is `127.0.0.1:7419`, the tailnet HTTPS on this node's name, port 7419. `tailscale whois` admits a peer, never the request. [Tailnet access](../.agents/skills/tooling-aterm-client/references/tailnet-access.md) says who, and which pages open a socket.

**Views** come from the [MCP Apps gateway](../.agents/skills/tooling-aterm-client/references/mcp-apps-gateway.md).
