# How long a seat has been in its state, its last reply, and what it says it is doing

Daemon fields the seat rows and the pinned reply read (COI-3289, COI-3303, COI-3339). All ride the session view in `list` and in every pushed `sessions` frame.

## State age

* `state_seconds` is how long `state` has lasted, as the daemon saw it. Absent from a daemon without feature `state-age`, and `0` is a reading.
* Counting starts when the daemon first sees the state. After a daemon restart it counts from the restart, and a seat still starting counts from its start.
* The daemon looks at each seat five times a second and pushes a `sessions` frame the moment `state` flips, so a client need not poll.
* `prompt` holds across a repaint. A card that is gone for under 1.5 seconds still reads `prompt`, and a card that closes reads `prompt` 1.5 seconds longer, so "waiting 4m" does not reset on a flicker.
* Flips are rare. `busy` to `idle` needs three quiet seconds, and a working claude shows its interrupt hint, which keeps it `busy`.

## Last reply

* `last_reply` is the first line, as before, and `last_reply_at` (RFC 3339) is when that reply ended. Claude's transcript stamps it. For other harnesses it is the transcript file's modification time when the reply was first read. Feature `last-reply`.
* The whole text is fetched, not pushed, so the `sessions` frames stay small. Send `{type: "last_reply", target}` and read `{type: "last_reply", reply: {session, text, truncated, at}}`. Fetch when `last_reply_at` changes.
* `text` is cut to 8 KiB on a rune boundary, and `truncated` says so. A seat with no finished reply yet answers empty text and no `at`.
* **Drawn as markdown (COI-3543).** `src/lib/markdown.ts` parses a reply to a tree and `Markdown.svelte` and `InlineText.svelte` draw it node by node, so no HTML string is built and raw tags stay visible text. Only `http`, `https` and `mailto` addresses become links. The pinned line and the sidebar preview use `parseLine`, which treats a cut as expected so no half mark shows.
* **What counts.** The prose a seat wrote since its last tool result, in its last finished turn. Narration before a tool call is dropped. A turn that ends in tool calls with no closing prose keeps the earlier reply. Messages a seat sent to other seats are tool calls, not replies.

## Background work

* `background` is how many background tasks the seat's footer counts (shells, monitors, read off the `⏵⏵` line of a claude seat, 2026-11-09). A seat waiting on CI reads `idle` and `background: 1`. Feature `background`.
* **Absent means unknown, never zero.** Absent from an old daemon, from another harness, from a footer cut by the terminal width, and while a card or startup screen hides the footer. Offer one-tap close on `background: 0` alone.
* A `close` of a seat whose checkouts hold commits on no remote or changed files is refused with `reason: "unpushed_work"` (feature `close-unpushed`). Show the error text, and send the close again with `force` only after a confirm. Names a checkout under the seat's directory, at most two levels down.
* `{type: "reopen", target: "name@ref"}` resumes a closed seat headless, whatever its harness, answered `reopened` with `role` and `seat`. It claims a fresh name, so find the seat in the next `sessions` push. Refused with no conversation on record, or while the name is live. Feature `reopen`.

## Notify

* A coordinator seat pings Kai with the `notify_kai` tool, `{type: "notify", token, title, body}` on the wire (feature `notify`). The daemon sends clients on the `sessions` channel `{type: "notify", notify: {id, session, role, identity, title, body, at}}`: `at` is RFC 3339, `id` is unique per notify so a client plays it once, and title and body are cut to 60 and 140 characters with control characters dropped. It is **not replayed** on subscribe.
* Web Push goes out too, whether or not a page is attached: `kind: "notify"`, the same `title` and `body`, `tag: "aterm-<session>"`, so the worker shows it as it is and a tap opens the seat.
* Only a role the roster marks `pinned_by_default` may send one, looked up by the token's role slug. The seat's refusal carries `reason`: `not_coordinator`, `too_soon` (one per seat per 10 seconds), `bad_text`, or `roster_unavailable`, which fails closed. Until an agent-compose release carries the attribute no seat may.
* The tool's bar: a real decision or a result that needs Kai, never routine status. A seat's own waiting sounds or notifies nothing.

## Roster

* A role in `aterm.roster.v1` may carry `pinned_by_default: true`, agent-compose's role attribute, set for the coordinator. Absent means false, so an older roster pins nothing. A person's own pin or unpin wins over it.

## Launch origin

* `origin` is `window` or `headless`, how the seat was launched. Feature `origin`. Absent means unknown: an older daemon, an older launcher, or a seat adopted from a holder an older daemon started. Show no icon then.
* `window` is a seat the `aterm` launcher started in a terminal window and attached to. `headless` is a seat started with no window: a `launch` frame from the app, `aterm send --launch`, `aterm resume --headless`, `aterm bypass`. The daemon cannot tell the app from those other headless launches, so label it "no window", not "app".
* It is a fact of the launch. Attaching or detaching a client never moves it, and it survives a daemon restart, since the holder keeps it.

## Activity line

* `activity` is one line the seat wrote about what it is doing, and `activity_at` (RFC 3339) is when it set it. Both are absent until the seat sets one. Feature `activity`.
* A seat sets it with `aterm activity "<text>"` and empties it with `aterm activity --clear`. On the wire that is `{type: "activity", token, text}`, answered by `{type: "activity", session, text}` where `text` is what was stored. Empty `text` clears.
* Only the session's own token names the session. There is no target, and the daemon never reads the line off the screen, so a seat's output cannot set it.
* The daemon cuts the text to **57 characters** (runes, not bytes) and drops control and format characters. A line break or tab reads as one space. The cut is plain, with no ellipsis added.
* Setting pushes a `sessions` frame at once. Setting the same text again moves `activity_at` and pushes, which is how a seat says the line is still true. The age is `now - activity_at`, and a client decides what counts as stale.
* The line ends with the session, and a daemon restart drops it too, since it is held in memory. An adopted seat shows none until it sets one again.
