# The aterm host daemon

`aterm daemon` routes every session [`aterm`](aterm.md) opens. The kitty window is one client
attached to it.

## What the daemon owns

**`_session` hands the harness to the daemon instead of running it.** After the card it sends a `spawn` with the argv, environment, directory and window size, then attaches. The argv reaches the harness untouched.

**A session is named `<role>-<identity>`, with `-2`, `-3` only for concurrent instances, and a live name refuses a spawn.** A `claim` frame grants each launch the first free pool name for two minutes.

**`list` reads each session's screen.** The daemon rebuilds each screen from its output and reports `state`: `starting`, `prompt` (a card is up), `busy` (recent output or the interrupt hint), else `idle`.

**`aterm clear` types the harness's clear command, unstamped.** Only Kai's client and the `prod-director` role may, never on the caller's own session, a prompt, or a seat with no known command (claude only). Without `--force` it also refuses a busy session, a draft, or queued messages.

**A terminal is a login shell, not a seat.** `spawn` with `kind: "terminal"` and optional `cwd` runs `$SHELL -l` under a holder, named `terminal-<hex>`, with no token, and refuses every seat field, `argv` and `env`. It is absent from `sessions`, `aterm agents`, seat counts and every `send`, `status` and `clear` target. `list` and the `sessions` push carry `terminals` (`name`, `pid`, `started`, `clients`, `cwd`, `label`), feature `terminals`. A spawn `label` (feature `terminal-label`, text up to 256 bytes) is opaque, echoed unchanged in `spawned` and on the entry, kept by a restart. A seat spawn refuses it. `attach`, `input`, `resize` and `close` take its name, an exiting shell ends it, a restart adopts it. A remote device gets `remote_terminal`, passkey or not.

**A session outlives its window.** `aterm close` types the harness's exit at an idle prompt, waits 10 seconds, then SIGTERM, then SIGKILL after 3.

**A holder owns each session's terminal, so a daemon crash leaves sessions running.** See [holders, launch, Sentry](aterm-bundles.md#daemon-internals-launch-holders-and-sentry).

**The socket is `/tmp/aterm-<uid>/daemon.sock`, keyed by uid rather than `HOME`**, because a session shadow moves `HOME` and every seat must reach one daemon. `ATERM_DAEMON_SOCKET` overrides it. The directory must be the user's alone, which is all of local client auth.

## `aterm send`

**The sender is stamped by the daemon, never declared.** Each spawn gets a fresh `ATERM_SESSION_TOKEN`. `send` presents it, and the daemon resolves the seat and types `[from <role> <identity>] <body>`.

**A body cannot forge a second envelope.** A body line opening with `[from ` gets a leading `\`. Control bytes but tab show as caret or `<U+XXXX>`, since an escape would end a paste.

**Targets resolve in tiers**: session name, role slug, identity, then harness. The first tier with a match wins, several in it refuse and name them, and none exits 3 listing the live sessions. `--launch` on a role slug opens the role and holds the message up to three minutes. `--new` opens another instance.

**Delivery serializes with the keyboard.** One lock covers every PTY write, so nothing interleaves. A message is `queued` until the target is ready, `held` while Kai typed in the last 1.5 seconds, has a draft touched in the last minute, or a card is up, then `delivered` or `failed`.

**A program that asked for bracketed paste gets the message as one paste, then Enter 300ms later**, since an Enter inside the paste reads as a newline. codex turns it on at its prompt, so **ready means paste is on, never a quiet screen**. claude and opencode drop a paste before the prompt mounts, so **ready means `Try "` or `Ask anything` showed**, or 20s passed. **`delivered` means claude took Enter**, re-pressed up to 3 times.

**A process inside a session cannot type into one.** The daemon walks the connecting pid's parents. Such a process may send, stamped, but not type, unless it spawned that session.

**A websocket on this host gets the same walk**, its source port mapped to the pids holding it, so a browser a session started, or one the daemon cannot name, cannot type, answer, launch or clear. A browser outside every session types, an agent driving Kai's own Chrome included.

**A device on the tailnet types only after a passkey.** Its `input`, `answer`, `launch`, `clear`, `spawn`, `close`, `send`, `claim`, `ask`, `cancel_ask`, `gateway_add`, `view_call` and `view_close` refuse until an assertion with user verification succeeds on that connection, then `typing` pushes `allowed`. The relying party is `coilyco.dev`, the only asserting origin `https://coilyco.dev`, so the daemon-served page cannot assert. `aterm passkey enroll` prints a one-time code and `revoke` forgets every passkey, neither from a session.

## Wire contract

`aterm.daemon.v1` is one JSON object per line over the socket, and one per text message over the websocket. Both sides open with `hello` and `welcome`, and a mismatched format refuses. A request's `id` is echoed on its reply or `error`.

* `spawn`, `attach`, `detach`, `input` and `output` (base64 `data`), `resize`, `exit` with `code`.
* `send` answers `sent` with the message state, waiting 3 seconds, or `wait` up to 120, for delivery unless `launching`. One not yet final earns the sender `[from aterm daemon] message <id> to <session>: <state>`, never the body. `notify_idle` adds `<session> is idle`, `is held at a prompt` or `ended`.
* `status` (with `lines`), `clear` and `close` (with optional `force`), each with a `target`, answer `status`, `cleared` and `closed` with the exit `code`. `claim` with a `session` base answers `claimed` with a free pool name, and `peek` holds none.
* `list` answers `sessions`, each with `state`, `quiet_seconds` and `context` ([the meter](../.agents/skills/tooling-aterm-client/references/context-meter.md)). `subscribe` to channel `sessions` pushes the roster on every change, and `message` events carry each state change, never the body. A stalled subscriber is dropped after 5 seconds.
* `welcome` to a websocket adds `typing-guard`, `passkey` and `typing` `{allowed, reason, passkey}`, `passkey` being `enrolled` or `unenrolled`. A refusal's `error` carries `reason`: `session_descendant`, `peer_unread` or `passkey_required`.
* Passkey: `passkey_enroll_begin` (`enroll_code`) answers `passkey_enroll_options`, and `passkey_enroll_finish` (`credential`) answers `passkey_enrolled`. `passkey_assert_begin` answers `passkey_assert_options`, and `passkey_assert_finish` answers `passkey_asserted`. `options` and `credential` are WebAuthn JSON.
* `hosts` lists tailnet daemons that answered ([discovery](../.agents/skills/tooling-aterm-client/references/discovery.md)).
* `whoami` resolves a token to its session. `roster` answers `aterm.roster.v1`, the launchable roles `aterm --list --json` prints. `launch` with a `role` and optional `seat` opens it headless, answering `launched`.
* `ask` takes a `question`, `options` (`label`, `description`), `header`, `allow_other`, `multi`. MCP `ask_choice` takes up to four `questions`, one card each. The daemon stamps the asker and pushes `ask` to subscribers, replayed on subscribe. `answer` (`ask_id`, `picks`, `text`) or `cancel_ask` settles it, and `asked` tells every client to drop the card. An asker leaving cancels its asks, and an answer takes the typing guard as Kai's input.

**Browsers get the client from `--client-dir` at `/`, and a websocket there.** Loopback is `127.0.0.1:7419`. The tailnet is HTTPS on this node's tailnet name, port 7419, probed every 30 seconds and rebound after two failures. `tailscale whois` admits a peer, never the request: this node owner's untagged device or one tagged `tag:physical`. A websocket opens only from the served page or `https://coilyco.dev`.

**Views** come from the [MCP Apps gateway](../.agents/skills/tooling-aterm-client/references/mcp-apps-gateway.md).
