# The aterm host daemon

`aterm daemon` routes every session [`aterm`](aterm.md) opens. The kitty window is one client
attached to it, and `aterm send` types a stamped message from one session into another.

```text
aterm agents
aterm send frontend-eng "ready for review"
aterm send --launch scientist -       # open the role if none answers
aterm attach eng-platform-beetle-ox   # Ctrl-] detaches
aterm status scientist-frog-ox        # state and screen
aterm clear scientist-frog-ox
aterm close scientist-frog-ox
aterm daemon                          # foreground, websocket on 127.0.0.1:7419
aterm ask "Ship it?" yes no           # a choice card
aterm mcp
```

## What the daemon owns

**`_session` hands the harness to the daemon instead of running it.** After the card it sends a `spawn` with the argv, environment, directory and window size, then attaches as one client. The argv reaches the harness untouched, and the window holds on a non-zero exit.

**A session is named `<role>-<identity>`, with `-2`, `-3` only for concurrent instances, and a spawn under a live session's name is refused.** A `claim` frame grants each launch the first free pool name for two minutes, so a relaunch takes the bare one back. Claude also gets it as `--name`.

**The harness starts without agent-compose's Enter gate**, since the window drew its own card (`AGENT_COMPOSE_NO_PAUSE=1`). agent-compose's `ESC ] 7750 ; agent-compose ; degraded=<steps> BEL` becomes the session's `degraded` field.

**`list` reads each session's screen.** The daemon rebuilds each terminal's screen from its output and reports `state`: `starting`, `prompt` (a permission or choice card is up), `busy` (recent output or the interrupt hint), else `idle`. `aterm status` adds the prompt text, quiet seconds, the draft, and the last rows, and never types.

**`aterm clear` types the harness's clear command, unstamped.** Only Kai's client and the `prod-director` role may, never on the caller's own session, a prompt, or a seat with no known command (claude only). Without `--force` it also refuses a busy session, a draft, or queued messages.

**A session outlives its window.** Closing it detaches that client, and the harness runs on until `aterm close` types the harness's exit (`/exit` for claude) at an idle prompt and waits 10 seconds, then sends SIGTERM, then SIGKILL after 3 seconds.

**A holder owns each session's terminal, so the daemon is replaceable.** `aterm hold`, this binary once per session, detached, owns the PTY, the child and a 1 MB scrollback ring, and takes its spawn as one stdin line since the environment holds credentials. It listens in `hold/`, one socket per spawn, and speaks `aterm.hold.v1`. A daemon exiting by signal or crash leaves every session running. The next one adopts each, drops a socket nobody answers. An adopted session has a typing hold, and pending messages are lost.

**`ATERM_SENTRY_DSN` (SSM `/coilysiren/sentry/dsn/aterm`, via `daemon.env`) turns on two Sentry reports.** A cron check-in every 5 minutes says `ok` if the tailnet handshake completes, else `error`. A panic in a daemon goroutine goes out through sentry-go as a fatal event tagged `goroutine`, flushed within 2 seconds, then is raised again, so the crash is unchanged. No log quotes the DSN.

**A missing daemon costs messaging, never the session.** `_session` starts one, else runs the harness directly, and [launchd](aterm-bundles.md) can run it.

**The socket is `/tmp/aterm-<uid>/daemon.sock`, keyed by uid rather than `HOME`**, because a session shadow moves `HOME` and every seat must reach one daemon. `ATERM_DAEMON_SOCKET` overrides it. The directory must be the user's alone, which is all of local client auth.

## `aterm send`

**The sender is stamped by the daemon, never declared.** Each spawn gets a fresh `ATERM_SESSION_TOKEN`, replacing any inherited one. `send` presents it, and the daemon resolves the seat and types `[from <role> <identity>] <body>`.

**A body cannot forge a second envelope.** A body line opening with `[from `, after leading space, gets a `\` in front. Every C0 control byte but tab, DEL, and C1 control shows in caret or `<U+XXXX>` notation, since an escape byte would end a bracketed paste.

**Targets resolve in tiers**: session name, role slug, identity, then harness. The first tier with a match wins, several matches in it refuse and name them, and none exits 3 with the live sessions listed. `--launch` on a role slug opens the role and holds the message up to three minutes. `--new` always opens another instance and names it.

**Delivery serializes with the keyboard.** One lock covers every PTY write, so nothing interleaves. A message is `queued` until the target is ready, `held` while Kai typed in the last 1.5 seconds, has a draft touched in the last minute, or a permission or choice card is up, then `delivered` or `failed`. Enter, Ctrl-C or Ctrl-U clear the draft.

**A program that asked for bracketed paste gets the message as one paste, then Enter 300ms later**, since an Enter inside the paste reads as a newline. Without it, lines join with spaces. codex turns it on at its prompt, so **ready means paste is on, never a quiet screen**. claude and opencode drop a paste before the prompt mounts, so **ready means `Try "` or `Ask anything` showed**, or 20s passed. **`delivered` means claude took Enter**, re-pressed up to 3 times.

**A process inside a session cannot type into one.** The daemon walks the connecting pid's parents. Such a process may send, stamped, but not type, unless it spawned that session. This guards mistakes, not a double-forking process.

## Wire contract

`aterm.daemon.v1` is one JSON object per line over the socket, and one per text message over the websocket. Both sides open with `hello` and `welcome` naming the format, and a mismatch refuses. A request's `id` is echoed on the reply or on an `error` with `code`.

* `spawn`, `attach`, `detach`, `input` and `output` (base64 `data`), `resize`, `exit` with `code`.
* `send` answers `sent` with the message state, waiting 3 seconds, or `wait` up to 120, for delivery unless `launching`. One not yet final earns the sender `[from aterm daemon] message <id> to <session>: <state>`, never the body. `notify_idle` adds `<session> is idle`, `is held at a prompt` or `ended`.
* `status` (with `lines`), `clear` and `close` (with optional `force`), each with a `target`, answer `status`, `cleared` and `closed` with the exit `code`. `claim` with a `session` base answers `claimed` with a free pool name, and `peek` holds none.
* `list` answers `sessions`, each with `state`, `quiet_seconds` and `context` ([the meter](../.agents/skills/tooling-aterm-client/references/context-meter.md)). `subscribe` to channel `sessions` pushes the roster on every change, and `message` events carry each state change, never the body.
* `whoami` resolves a token to its session. `roster` answers `aterm.roster.v1`, the launchable roles `aterm --list --json` prints. `launch` with a `role` and optional `seat` opens it headless, answering `launched`.
* `ask` takes a `question`, `options` (`label`, `description`), `header`, `allow_other`, `multi`. MCP `ask_choice` takes up to four `questions`, one card each. The daemon stamps the asker and pushes `ask` to subscribers, replayed on subscribe. `answer` (`ask_id`, `picks`, `text`) or `cancel_ask` settles it, and `asked` tells every client to drop the card. An asker leaving cancels its asks, and an answer takes the typing guard as Kai's input.

**Browsers get the client from `--client-dir` at `/`, and a websocket there.** Loopback is `127.0.0.1:7419`. The tailnet is HTTPS on this node's tailnet name, port 7419, and the daemon handshakes against it every 30 seconds, rebinding after two failures. `aterm doctor` runs the same handshake. `tailscale whois` admits a peer, never the request: this node owner's untagged device or one tagged `tag:physical`. A websocket opens only from the served page or `https://coilyco.dev`.
