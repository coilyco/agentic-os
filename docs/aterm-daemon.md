# The aterm host daemon

`aterm daemon` owns the terminal of every session `aterm` opens. The kitty
window is one client attached to it, and `aterm send` types a stamped message
from one session into another. It replaced VibeTunnel. The window it serves is
[the native agent terminal](aterm.md). Records: teable:coilyco/agentic-os#8219
(send) and teable:coilyco/agentic-os#8220 (daemon and client).

```text
aterm agents                          # live sessions, the targets send takes
aterm send frontend-eng "ready for review"
aterm send --launch scientist -       # open the role if none answers, body on stdin
aterm attach eng-platform-beetle-ox   # another terminal on a live session, Ctrl-] detaches
aterm daemon                          # foreground, for a service manager
aterm mcp                             # list_agents and send_message over MCP stdio
```

## What the daemon owns

**`_session` hands the harness to the daemon instead of running it.** After the card, `_session` sends a `spawn` carrying the argv, its environment, directory, and window size, then attaches, relaying keys and output and following the window size. The argv reaches the harness untouched, the shadow child's own `--` included, and the window still holds on a non-zero exit.

**A session is named `<role>-<identity>` from its card, and a spawn under a name in use ends the session holding it**: SIGTERM to its process group, 3 seconds, then SIGKILL. The daemon owns the process, so no pid start-time check is needed. For the claude seat aterm also passes `--name <role>-<identity>` ahead of the caller's arguments and stops running `claude` processes of that name the daemon did not start. A caller's own `--name` is kept, and `--no-stable-name` (or `ATERM_NO_STABLE_NAME`) stops nothing.

**A session outlives its window.** Closing the window detaches that client, and the harness keeps running until it exits or the next launch of the role replaces it. `aterm attach` reattaches from any terminal with the last megabyte of output replayed.

**A missing daemon costs messaging, never the session.** `_session` starts the daemon when none answers. If it still cannot connect, it prints one line and runs the harness directly. `aterm doctor` reports a `daemon` row, which is a warning only when the socket directory would be refused.

**The socket is `/tmp/aterm-<uid>/daemon.sock`, keyed by uid rather than `HOME`**, because a session shadow moves `HOME` and every seat must reach one daemon. `ATERM_DAEMON_SOCKET` overrides it. The directory must be owned by the user and closed to others, and that permission is all of local client auth. A daemon with no session and no client for five minutes exits, so the next launch runs the upgraded binary. A second daemon loses a lock race and exits.

## `aterm send`

**The sender is stamped by the daemon, never declared.** Each spawn gets a fresh `ATERM_SESSION_TOKEN` in its environment, replacing any inherited one. `send` presents it, and the daemon resolves it to the seat and types `[from <role> <identity>] <body>`. A token the daemon did not issue exits 2. Text a person types carries no envelope.

**A body cannot forge a second envelope.** A body line opening with `[from `, after leading space, gets a `\` in front. Every C0 control byte but tab, DEL, and C1 control is written in caret or `<U+XXXX>` notation, since an escape byte would end a bracketed paste and send the rest as keys. The client marks stamped rows by matching the envelope, so this escaping is required.

**Targets resolve in tiers**: exact session name, then role slug, then identity, then harness. The first tier with a match wins, several matches in it refuse and name them, and none exits 3 with the live sessions listed. `--launch` on a role slug opens the role through `aterm <role>` and holds the message up to three minutes for it.

**Delivery serializes with the keyboard.** One lock covers every PTY write, so a message never interleaves with keystrokes. A message is `queued` until the target is ready, `held` while Kai typed in the last 1.5 seconds or has an unsent draft touched in the last minute, then `delivered` or `failed`. The draft count follows printable keys, backspace, and pastes, and Enter, Ctrl-C, or Ctrl-U clear it. A held message lands after her draft is sent.

**A program that asked for bracketed paste gets the message as one paste, then Enter 300ms later**, since a TUI reading a paste as a burst takes an Enter that arrives with it as a newline. Without bracketed paste, lines are joined with spaces so a newline cannot submit early. The daemon reads the mode from the program's own output.

**A process inside a session cannot type into one.** The daemon reads the connecting pid from the kernel and walks its parents. A process under any session may send, which is stamped, but its raw input is refused, unless it spawned that session itself. This guards against mistakes, not a hostile same-user process, which could escape the parent chain by double-forking or reach the window some other way.

## Per-harness delivery

Observed on 2026-09-25 with claude and codex seats launched through `agent-compose` under the daemon.

* **claude** - turns bracketed paste on at its prompt. `aterm send` from a claude seat reported `delivered` in about 6 seconds. Mid-turn queuing is recorded on teable:coilyco/agentic-os#8219 and was not re-observed.
* **codex v0.156.1** - turns bracketed paste on at its prompt, took the stamped message as one paste, submitted it on the delayed Enter, and acted on it. Its shell tool rebuilds `PATH` from a login shell, so it runs the Homebrew `aterm`. Mid-turn queuing is unverified.
* **goose, opencode** - unverified. They fall back to ready after 30 quiet seconds.

**Ready means bracketed paste for claude and codex, never a quiet screen.** `agent-compose launch` stops at `Press Enter to continue` before the harness starts, which is quiet too, and a message typed there is lost.

## Wire contract

`aterm.daemon.v1` is one JSON object per line over the socket, the same objects a websocket will carry. Both sides open with `hello` and `welcome` naming the format, and a mismatch refuses. Requests carry an `id` echoed on the reply or on an `error` with `code`.

* `spawn`, `attach` (optional `replay`), `detach`, `input` and `output` (base64 `data`), `resize`, `exit` with `code`.
* `send` answers `sent` with the message state, waiting up to 3 seconds for delivery.
* `list` answers `sessions`. `subscribe` to channel `sessions` pushes the roster on every change, and `message` events carry each state change.
* `whoami` resolves a token to its session.

## Not built yet

Tailnet reach, meaning a websocket listener, client auth, and daemon discovery, is teable:coilyco/agentic-os#8220's next milestone, along with the MCP Apps gateway and the streamed browser. `aterm mcp` ships, and projecting it into each harness registry is a separate change.
