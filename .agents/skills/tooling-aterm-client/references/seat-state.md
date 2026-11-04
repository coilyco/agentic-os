# How long a seat has been in its state, and its last reply

Two daemon fields the seat rows and the pinned reply read (COI-3289, COI-3303). Both ride the session view in `list` and in every pushed `sessions` frame.

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
* **What counts.** The prose a seat wrote since its last tool result, in its last finished turn. Narration before a tool call is dropped. A turn that ends in tool calls with no closing prose keeps the earlier reply. Messages a seat sent to other seats are tool calls, not replies.
