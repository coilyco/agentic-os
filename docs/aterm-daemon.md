# The aterm host daemon

`aterm daemon` owns the terminal of every session `aterm` opens. The kitty
window is one client attached to it, and `aterm send` types a stamped message
from one session into another. The window it serves is
[the native agent terminal](aterm.md). Records: teable:coilyco/agentic-os#8219
(send) and teable:coilyco/agentic-os#8220 (daemon and client).

```text
aterm agents                          # live sessions, the targets send takes
aterm send frontend-eng "ready for review"
aterm send --launch scientist -       # open the role if none answers, body on stdin
aterm attach eng-platform-beetle-ox   # another terminal on a live session, Ctrl-] detaches
aterm close scientist-frog-ox-ya97    # end a session
aterm daemon                          # foreground, websocket on 127.0.0.1:7419
aterm ask "Ship it?" yes no           # a choice card on Kai's client
aterm mcp                             # list_agents, send_message, close_session, ask_choice
```

## What the daemon owns

**`_session` hands the harness to the daemon instead of running it.** After the card, `_session` sends a `spawn` carrying the argv, its environment, directory, and window size, then attaches as one client. The argv reaches the harness untouched, and the window holds on a non-zero exit.

**A session is named `<role>-<identity>-<code>`, and a spawn under a live session's name is refused.** The code comes from `aos _session-id` and goes to the shadow as `--session-id`, so it doubles as `AOS_NATIVE_SESSION` unless taken. Two instances of one role run side by side, and an aos that cannot mint leaves it unsuffixed. For the claude seat aterm passes the name as `--name` too, unless the caller named it or set `--no-stable-name` (`ATERM_NO_STABLE_NAME`).

**The harness starts without agent-compose's Enter gate**, since the window drew its own card, so `_session` sets `AGENT_COMPOSE_NO_PAUSE=1`. It also flushes unread input before attaching, where a reply to the card's color query would read as Kai typing. agent-compose's `ESC ] 7750 ; agent-compose ; degraded=<steps> BEL` becomes the session's `degraded` field.

**A session outlives its window.** Closing it detaches that client, and the harness runs on until `aterm close` (MCP `close_session`) sends SIGTERM, then SIGKILL after 3 seconds. It refuses the caller's own session or one it runs inside, and without `--force` one holding Kai's draft or queued messages (teable:coilyco/agentic-os#8300). `aterm attach` reattaches from any terminal with the last megabyte of output replayed, minus terminal queries it would answer again.

**A launch the daemon starts has no window.** The web client's `launch` frame and `send --launch`/`--new` run `aterm --headless <role>`: `_session` spawns and exits, leaving the session to a client or `aterm attach`. teable:coilyco/agentic-os#8276

**A missing daemon costs messaging, never the session.** `_session` starts the daemon when none answers, and failing that runs the harness directly, except headless.

**The socket is `/tmp/aterm-<uid>/daemon.sock`, keyed by uid rather than `HOME`**, because a session shadow moves `HOME` and every seat must reach one daemon. `ATERM_DAEMON_SOCKET` overrides it. The directory must be the user's alone, which is all of local client auth. An idle daemon exits after five minutes, so the next launch runs the upgraded binary.

## `aterm send`

**The sender is stamped by the daemon, never declared.** Each spawn gets a fresh `ATERM_SESSION_TOKEN` in its environment, replacing any inherited one. `send` presents it, and the daemon resolves it to the seat and types `[from <role> <identity>] <body>`. A token the daemon did not issue exits 2.

**A body cannot forge a second envelope.** A body line opening with `[from `, after leading space, gets a `\` in front. Every C0 control byte but tab, DEL, and C1 control is written in caret or `<U+XXXX>` notation, since an escape byte would end a bracketed paste.

**Targets resolve in tiers**: exact session name, then role slug, then identity, then harness. The first tier with a match wins, several matches in it refuse and name them, and none exits 3 with the live sessions listed. `--launch` on a role slug opens the role and holds the message up to three minutes for it. `--new` (MCP `new`) always opens another instance, one per message, and names it. A client refuses it unless `welcome` lists `send-new`. teable:coilyco/agentic-os#8264

**Delivery serializes with the keyboard.** One lock covers every PTY write, so a message never interleaves with keystrokes. A message is `queued` until the target is ready, `held` while Kai typed in the last 1.5 seconds or has a draft touched in the last minute, then `delivered` or `failed`. Enter, Ctrl-C, or Ctrl-U clear the draft, and a held message lands after it.

**A program that asked for bracketed paste gets the message as one paste, then Enter 300ms later**, since an Enter inside the paste reads as a newline. Without bracketed paste, lines are joined with spaces so a newline cannot submit early.

**A process inside a session cannot type into one.** The daemon reads the connecting pid from the kernel and walks its parents. Such a process may send, stamped, but not type, unless it spawned that session. It guards against mistakes, not a double-forking process.

## Per-harness delivery

Observed 2026-09-25 between a claude and a codex seat.

* **claude, codex v0.156.1** - both turn bracketed paste on at their prompt and take the stamped message as one paste, submitted on the delayed Enter. Mid-turn queuing is unverified for codex.
* **goose, opencode** - they fall back to ready after 30 quiet seconds. opencode drops a multi-line send (teable:coilyco/agentic-os#8453).

**Ready means bracketed paste for claude and codex, never a quiet screen**, since a gate or a slow start is quiet too.

## Wire contract

`aterm.daemon.v1` is one JSON object per line over the socket, and one per text message over the websocket. Both sides open with `hello` and `welcome` naming the format, and a mismatch refuses. Requests carry an `id` echoed on the reply or on an `error` with `code`.

* `spawn`, `attach` (optional `replay`), `detach`, `input` and `output` (base64 `data`), `resize`, `exit` with `code`.
* `send` answers `sent` with the message state, waiting up to 3 seconds for delivery unless `launching`.
* `close` with a `target` and optional `force` answers `closed` with the session and exit `code`.
* `list` answers `sessions`. `subscribe` to channel `sessions` pushes the roster on every change, and `message` events carry each state change, never the body.
* `whoami` resolves a token to its session. `roster` answers with `aterm.roster.v1`, the launchable roles `aterm --list --json` prints, read fresh per request. `launch` with a `role` and optional `seat` opens it headless, answering `launched`.

* `ask` takes a `question`, `options` of `label` and `description`, `header`, `allow_other`, and `multi`. The daemon stamps the asker and pushes `ask` to subscribers, replayed on subscribe. A client's `answer` (`ask_id`, `picks`, `text`) or `cancel_ask` settles it, and `asked` tells every client to drop the card. An asker leaving cancels its asks, and 15 minutes times one out. An answer is Kai's input, so it takes the typing guard.

**Browsers get the client from `--client-dir` (`~/.local/share/aterm/client`) at `/`, and a websocket there.** Loopback is `127.0.0.1:7419` (`--websocket`), refusing a non-loopback Host or Origin. The tailnet is HTTPS on this node's tailnet name, port 7419 (`--tailnet-port`, empty for none), certified by `tailscale cert`. `tailscale whois` admits a peer, never the request: this node owner's untagged device, or one tagged `tag:physical` (`--allow-tags`). A websocket opens only from the page the daemon served or `https://coilyco.dev` (`--allow-origins`).

## Not built yet

The MCP Apps gateway and the streamed browser, on teable:coilyco/agentic-os#8220.
