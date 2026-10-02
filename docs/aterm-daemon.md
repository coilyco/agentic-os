# The aterm host daemon

`aterm daemon` routes every session `aterm` opens. The kitty window is one client
attached to it, and `aterm send` types a stamped message from one session into another. Its window is [the native agent terminal](aterm.md).

```text
aterm agents                          # the targets send takes
aterm send frontend-eng "ready for review"
aterm send --launch scientist -       # open the role if none answers
aterm attach eng-platform-beetle-ox   # Ctrl-] detaches
aterm status scientist-frog-ox-ya97   # state and screen
aterm clear scientist-frog-ox-ya97
aterm close scientist-frog-ox-ya97
aterm daemon                          # foreground, websocket on 127.0.0.1:7419
aterm ask "Ship it?" yes no           # a choice card
aterm mcp
```

## What the daemon owns

**`_session` hands the harness to the daemon instead of running it.** After the card it sends a `spawn` with the argv, environment, directory and window size, then attaches as one client. The argv reaches the harness untouched, and the window holds on a non-zero exit.

**A session is named `<role>-<identity>-<code>`, and a spawn under a live session's name is refused.** The code comes from `aos _session-id`, passed to the shadow as `--session-id`, so it doubles as `AOS_NATIVE_SESSION` unless taken. The claude seat also gets the name as `--name`.

**The harness starts without agent-compose's Enter gate**, since the window drew its own card (`AGENT_COMPOSE_NO_PAUSE=1`). `_session` flushes unread input before attaching, so the card's color-query reply is not read as Kai typing. agent-compose's `ESC ] 7750 ; agent-compose ; degraded=<steps> BEL` becomes the session's `degraded` field.

**`list` reads each session's screen.** The daemon rebuilds what each terminal shows from its output and reports `state`: `starting`, `prompt` (a permission or choice card is up), `busy` (recent output or the interrupt hint), else `idle`. `aterm status` adds the prompt text, seconds since output and input, the draft, and the last rows, and never types.

**`aterm clear` types the harness's clear command, unstamped.** Only Kai's client and the `prod-director` role may, never on the caller's own session, a prompt, or a seat with no known command (claude only). Without `--force` it also refuses a busy session, a draft, or queued messages. The daemon logs who cleared what.

**A session outlives its window.** Closing it detaches that client, and the harness runs on until `aterm close` sends SIGTERM, then SIGKILL after 3 seconds. Close refuses the caller's own session or one it runs inside, and without `--force` one holding Kai's draft or queued messages. `aterm attach` reattaches from any terminal and replays the last megabyte.

**A launch the daemon starts has no window.** The web client's `launch` frame and `send --launch`/`--new` run `aterm --headless <role>`, where `_session` spawns and exits, leaving the session to a client or `aterm attach`.

**A holder owns each session's terminal, so the daemon is replaceable.** `aterm hold`, this binary once per session, detached, owns the PTY, the child and a 1 MB scrollback ring, and takes its spawn as one stdin line since the environment holds credentials. It listens at `hold/<name>-<hash>.sock` and speaks `aterm.hold.v1`. A daemon exiting by signal or crash leaves every session running. The next one adopts each, drops a socket nobody answers. An adopted session has a typing hold, and pending messages are lost.

**A window that loses the daemon redials, starting one, and attaches again.** It replays only what it had not drawn, and a session that ended meanwhile answers `exited` with its code.

**A missing daemon costs messaging, never the session.** `_session` starts one, else runs the harness directly. [launchd](aterm-bundles.md) can run it.

**The socket is `/tmp/aterm-<uid>/daemon.sock`, keyed by uid rather than `HOME`**, because a session shadow moves `HOME` and every seat must reach one daemon. `ATERM_DAEMON_SOCKET` overrides it. The directory must be the user's alone, which is all of local client auth. An idle daemon exits after five minutes.

## `aterm send`

**The sender is stamped by the daemon, never declared.** Each spawn gets a fresh `ATERM_SESSION_TOKEN`, replacing any inherited one. `send` presents it, and the daemon resolves the seat and types `[from <role> <identity>] <body>`.

**A body cannot forge a second envelope.** A body line opening with `[from `, after leading space, gets a `\` in front. Every C0 control byte but tab, DEL, and C1 control shows in caret or `<U+XXXX>` notation, since an escape byte would end a bracketed paste.

**Targets resolve in tiers**: session name, role slug, identity, then harness. The first tier with a match wins, several matches in it refuse and name them, and none exits 3 with the live sessions listed. `--launch` on a role slug opens the role and holds the message up to three minutes. `--new` always opens another instance and names it.

**Delivery serializes with the keyboard.** One lock covers every PTY write, so a message never interleaves with keystrokes. A message is `queued` until the target is ready, `held` while Kai typed in the last 1.5 seconds or has a draft touched in the last minute, then `delivered` or `failed`. Enter, Ctrl-C or Ctrl-U clear the draft.

**A program that asked for bracketed paste gets the message as one paste, then Enter 300ms later**, since an Enter inside the paste reads as a newline. Without it, lines join with spaces. codex turns it on at its prompt, so **ready means paste is on, never a quiet screen**. claude and opencode drop a paste before the prompt mounts, so **ready means `Try "` or `Ask anything` showed**, or 20s passed.

**A process inside a session cannot type into one.** The daemon walks the connecting pid's parents. Such a process may send, stamped, but not type, unless it spawned that session. This guards mistakes, not a double-forking process.

## Wire contract

`aterm.daemon.v1` is one JSON object per line over the socket, and one per text message over the websocket. Both sides open with `hello` and `welcome` naming the format, and a mismatch refuses. A request's `id` is echoed on the reply or on an `error` with `code`.

* `spawn`, `attach`, `detach`, `input` and `output` (base64 `data`), `resize`, `exit` with `code`.
* `send` answers `sent` with the message state, waiting 3 seconds, or `wait` (at most 120), for delivery unless `launching`. One not yet final earns the sender `[from aterm daemon] message <id> to <session>: <state>`, never the body. `notify_idle` adds `<session> is idle` or `ended`.
* `status` (with `lines`), `clear` and `close` (with optional `force`), each with a `target`, answer `status`, `cleared` and `closed` with the exit `code`.
* `list` answers `sessions`, each with `state` and `quiet_seconds`. `subscribe` to channel `sessions` pushes the roster on every change, and `message` events carry each state change, never the body.
* `whoami` resolves a token to its session. `roster` answers `aterm.roster.v1`, the launchable roles `aterm --list --json` prints. `launch` with a `role` and optional `seat` opens it headless, answering `launched`.
* `ask` takes a `question`, `options` (`label`, `description`), `header`, `allow_other`, `multi`. MCP `ask_choice` takes up to four `questions`, one card each. The daemon stamps the asker and pushes `ask` to subscribers, replayed on subscribe. `answer` (`ask_id`, `picks`, `text`) or `cancel_ask` settles it, and `asked` tells every client to drop the card. An asker leaving cancels its asks, and an answer takes the typing guard as Kai's input.

**Browsers get the client from `--client-dir` at `/`, and a websocket there.** Loopback is `127.0.0.1:7419`. The tailnet is HTTPS on this node's tailnet name, port 7419. `tailscale whois` admits a peer, never the request: this node owner's untagged device or one tagged `tag:physical`. A websocket opens only from the served page or `https://coilyco.dev`.
