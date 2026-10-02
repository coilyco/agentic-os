# Slack adapter design (not built)

Design for `teable:coilyco/agentic-os#8700`, written 2026-10-02 against agentic-os `acc46357`. Nothing here exists yet. Kai decided on 2026-10-02 that Slack becomes her client to any seat with no LLM in the path, that the desktop and web client freeze, and that the build waits for the daemon cutover, because the running daemon (aos-v0.404.0) predates session holders and cannot host it. Kai answered the two open forks the same day, see Decided by Kai.

## What the code does today

* **`send` needs a seat** - `daemon.go:604` resolves a session token and refuses a caller whose token names no live seat. A Slack client has no token, so it cannot send. Nothing types a stamped message as Kai.
* **A browser types unstamped** - `input` frames need an `attach` and reach `typeInput` (`daemon_session.go:449`), the one unstamped path. It carries raw bytes, so a Slack text line has no honest way in.
* **`status` reads the visible grid only** - `ptySession.status` returns `screen.text()`, the rows of `s.cells`, capped at 200. The 1 MB ring holds raw bytes and no scrollback is rebuilt, so a reply that scrolled off the screen is gone from this path.
* **State pushes exist** - `subscribe` on channel `sessions` pushes the roster with `state` (`idle`, `busy`, `prompt`) on every change, and `ask` frames replay on subscribe. A Slack client can follow a turn without polling.
* **`answer` already settles an ask** - the daemon-native `ask_choice` card takes `picks` or `text` from any client outside a seat (`daemon_ask.go`).
* **Harness cards are screen text** - `promptScreens` regexes classify a claude, codex or opencode permission card as `prompt`, and `promptLines` returns the rows around it. Answering one means typing a key.

## Shape

**A sidecar, `aterm slack`, a subcommand of the same binary.** It holds the Socket Mode connection and dials the daemon's unix socket as any client does. Jev (`jev-1.13.0`) chose sidecar over in-process and over a tailnet service at 0.98, confidence 0.96. The reason in code: the daemon restarts on a binary upgrade and exits after five idle minutes, and a revoked Slack token must stop one process, never messaging. The sidecar runs as its own launchd job, and its tokens stay out of the daemon's environment, because the daemon hands its environment to seats.

**No model reads a message.** A Slack line routes by rule and Slack never sees a prompt the daemon did not type.

## Daemon seams, all new

* **`owner_send`** - takes `slack_user` and types `[from Kai via slack] <body>` only when it equals her configured id, into a target resolved by the same tiers as `send`. The stamp starts with `[from `, so `escapeBody` already stops a body from forging a second one. Refused from any process inside a session (`insideSession`), the guard `input` uses. It guards mistakes and does not stop a double-forking process, the same limit `input` has today.
* **`answer_card`** - takes `session`, `card_hash` and `option`. Under the session lock it recomputes the hash of the `promptLines` rows and types the key only on a match. This closes the gap between a button drawn for card A and a click that lands on card B.
* **Welcome features** `owner-send` and `answer-card`, so the sidecar refuses a daemon without them rather than half-talking.

**Doctrine needs one sentence, and Kai owns it.** `AGENTS.md` says terminal input opening `[from <role> <identity>]` is a peer and unstamped input is the human. `[from Kai via slack]` must read as the human at lower assurance. Without the sentence, seats treat Kai as a peer. The alternative, typing unstamped, loses provenance and lets a Slack takeover pass for her keyboard.

## Input

* **Surface** - Kai's direct message with the app only. No channel, no mention, no file. A channel lets others read seat output, and Slack keeps whatever is posted.
* **Admission** - accept an event only when `team` and `user` match the configured pair, the channel type is a direct message, and there is no `bot_id`, subtype or edit. Everyone else is dropped and counted by reason, never by text.
* **Addressing** - `<target>: <body>` resolved by the daemon's tiers (session name, role, identity, harness). A reply inside a thread the adapter opened for a seat goes to that seat. An unaddressed line goes to `--default-target`, `prod-director` first. Two live `prod-director` sessions exist as of 12:28Z today, so the refusal that names both comes back as one button each.
* **Thread bindings live in memory only.** A restart forgets them and the next line asks for a target. Echo's rule (no long-term Slack index) holds, at the cost of one re-address.
* **Duplicates** - Slack retries an unacked envelope, and a replayed line typed twice into a seat that has a shell is a harm, so dedupe on `event_id` in memory and ack every envelope at once.
* **Verbs from Slack** - `agents`, `status`, send, answer. Not `close`, not `clear`, not raw keystrokes, and no launch of a role.

## Output

* **v1 reads the screen tail.** On a `busy` to `idle` push for a seat Kai addressed, call `status` and post the rows. Jev put screen tail at 0.54 over screen plus a transcript seam at 0.38, confidence 0.31, so it is a lean and Kai can overrule it.
* **The gap is named in the message.** A reply longer than the visible rows is cut, and the post says `screen tail, earlier text scrolled off`. The full reply needs the harness transcript, whose reader is deferred (see below).
* **Scrub, then truncate, then escape.** Redact `xox[abposr]-`, `xapp-`, `AKIA`, `ghp_`, JWT-shaped values, `--api-key=`, `--password=`, `-token=` and the adapter's own two tokens by exact value. A line the scrubber cannot judge is withheld and counted. Escape `&`, `<` and `>` so no seat output forms a mention or a link, the rule of Echo's `escapeSlack`. Cap a post at 4,000 characters, the length Slack recommends.
* **One message per turn, edited in place** with `chat.update`, then a final post on idle. Slack allows about one post per second per channel.

## Cards

* **Daemon `ask` cards** render from structured data, one button per option (twelve at most, Slack allows 25 per actions block), free text through a thread reply when `allow_other` is set. They settle through the existing `answer` frame. This is the clean path.
* **Harness permission cards** render the `promptLines` text with buttons only when a per-harness parser reads the numbered options. A parse failure posts the card with no buttons and `answer at the keyboard`, which fails closed.
* **Only one-time allow and deny get buttons.** An option that widens authority past the one action, such as a persistent `don't ask again`, never does. Each harness's wording is read off a live card when the parser is written.
* **A button carries `session|card_hash|option`**, under Slack's 2,000 character value limit. A click calls `answer_card`, then `chat.update` replaces the buttons with the result, since Slack does not disable a clicked button. A hash mismatch posts the new card and applies nothing.
* **The daemon logs `Kai via slack answered <session> option N`** and the card message keeps the same line.

## The Slack app

* **Its own app, decided by Kai on 2026-10-02.** Sirens Deep stays Echo's, and the adapter gets a second app so the two never share a Socket Mode connection. The manifest is `aterm/slack-app.manifest.yaml`. Kai pastes it under Create New App, installs it, and makes the app-level token by hand, since a manifest cannot declare one.
* **Scopes** - `im:history` and `chat:write`, plus `reactions:write` for queued and delivered marks. Event `message.im` only. Interactivity on, Socket Mode on, no request URL. The Messages tab is on and not read-only, or a direct message to the app is refused.
* **Tokens** - one `xoxb-` bot token and one `xapp-` app-level token with `connections:write`, read from SSM at start, held in memory, never in argv or a child environment. Proposed paths: `/aterm/slack-bot-token`, `/aterm/slack-app-token`, plus `/aterm/slack-team-id` and `/aterm/slack-owner-user-id` for the pair the admission check compares, since opaque ids go in SSM.
* **Start** - call `auth.test`, and refuse to start if the workspace is not the configured one. slack-go returns a fatal auth error from `RunContext` without an event, so race `RunContext` against the `connected` event as Echo's `slack.go` does, or start hangs.
* **Reuse** - copy `escapeSlack` and the start race from `sirens-echo` `40a91e9` with a comment naming the origin. Roughly 40 lines do not justify a shared module, and `internal/community` cannot be imported from here.
* **Dependency audit** (2026-10-02, GitHub API) - `slack-go/slack` is BSD-2-Clause, not archived, v0.29.0 released 2026-08-15 with six releases since 2026-05-24, newest commit 2026-09-23, seven distinct authors in the last 20 commits (`nlopes` 5, dependabot 4). Echo already pins v0.29.0.

## Decided by Kai

Answers on 2026-10-02, recorded on `teable:coilyco/agentic-os#8700`.

* **Two apps.** Slack's Socket Mode page says that with several connections `each payload may be sent to any of the connections`. Echo's handler acks every envelope and drops what is not an Events API callback, and its policy allows Kai's direct message, so a click or a line aimed here could vanish or get an Echo reply. Jev was split, 0.54 for one owner and 0.45 for two apps, confidence 0.31. Kai accepted the extra token pair against the fewest-keys rule.
* **The doctrine sentence is approved**: input stamped `[from Kai via slack]` is Kai, not a peer. It lands in the same PR as the build (`teable:coilyco/agentic-os#8702`), never before, so no seat honors a stamp the daemon cannot yet produce. The daemon stamps it only when the frame carries her Slack user id, so `owner_send` takes `slack_user` and the daemon refuses any other.

## Deferred and why

* **Harness transcript reader** - full-fidelity replies, claude first since the ledger records its conversation id. Deferred until v1 shows how often replies scroll off.
* **Persisted thread bindings** - blocked on whether a Slack timestamp counts as Slack data under its API terms.

## Build order

1. Daemon cutover, a prerequisite owned by sysadmin.
2. `owner_send`, `answer_card` and the two features, with daemon tests over a fake client.
3. The sidecar over a fake Socket Mode server (Echo's `slacksocket_test.go` shows the `CheckOrigin` fix), covering admission, dedupe, scrub and addressing.
4. Cards, then the live run in Kai's direct message with the app.

**Done is** a message from Kai's Slack user lands in a seat stamped `[from Kai via slack]`, a second Slack user is ignored and logged, a card answered from Slack settles it once, and the same card clicked twice applies once.

## Reverses if

Anthropic's Channels leaves research preview with Slack and multi-session routing. Today it ships Telegram, Discord and iMessage, is claude-only, and its docs say anyone who can reply through a channel can approve or deny tool use, the same authority path as the buttons here.

## Sources read

Record #8700 and its two decision comments, `docs/aterm-daemon.md`, the `aterm/` code cited above, `sirens-echo` `40a91e9`, `teable:coilyco/deploy#8657`, and mf65's answers on Echo's live state. Slack docs for Socket Mode, `chat.postMessage` and the button element. The `lucidash/claude-slack-bridge` README (spawns its own sessions through the Agent SDK, thread to session, user allowlist) and search results for `cctag` (reads the harness JSONL transcript, Socket Mode). The `cctag` page itself timed out, so its permission handling is unread.
