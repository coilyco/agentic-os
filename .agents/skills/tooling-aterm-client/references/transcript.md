# The transcript read

**The daemon reads a seat's conversation from the harness's own files, so the client shows prompts, replies and tool steps with full output.** It serves Claude, Codex and opencode seats. The seat's PTY stays the seat, and this is a read-only lens (COI-3532). Code: `aterm/transcript*.go`. Fixtures and the replies a client's mock replays: `aterm/testdata/transcript/`.

## The frame

* **Feature** - `transcript` in `welcome`, so an older daemon is detected.
* **Request** - `{type: "transcript", id, session, limit?, before?, since?}`. `limit` is 50 by default and 200 at most. `before` and `since` are opaque cursors, and never both.
* **Reply** - `{type: "transcript", id, session, reader, items, cursor, older?, more?, updated_at}`. `reader` is `claude`, `codex` or `opencode`.
* **Order** - items are oldest first in every reply. A request with neither cursor returns the newest page. `before` (the reply's `older`) returns the page before it. `since` (the reply's `cursor`) returns what was written after.
* **Gate** - the same as `send`: a remote device needs its passkey, since tool output can hold secrets.

## An item

`{id, kind, text?, at?, model?, complete?, from?, for?, shared?, tool?}` with `text_bytes` and `truncated` when the text was cut.

* **kind** - `prompt` (Kai's), `reply`, `tool`, `note`, `peer`.
* **id** - the harness's own, stable. A running tool keeps its id, and the finished one replaces it in place.
* **text** - cut at 64 KiB on a rune boundary. A reply carries at most 1 MiB of text in all, oldest items dropped first.
* **complete** - on a reply: the turn ended.
* **model** - Claude: on each assistant line. Codex: the turn's model. opencode: the assistant message's.
* **tool** - `{name, class, summary, ok?, output?, output_bytes?, truncated?, diff?}`.
  * `class` is `shell`, `edit`, `read`, `search`, `mcp` or `other`.
  * `summary` is built here (`Ran just gate`, `Edited reply.go +12 -3`), so a client needs no harness vocabulary.
  * `ok` is true or false, and absent while the tool runs or when the harness cannot say.
  * `diff` is `{files, added, removed}`, counts only. The patch text is not sent. An edit carries no `output`.
  * `output` is cut at 64 KiB with `truncated` and the full size in `output_bytes`.
* **Left out** - reasoning and thinking, attachments, subagent lines, and the harness's own bookkeeping.

## Peer messages

Anything another seat sent is `kind: "peer"` with `from: {role, identity}`, and never belongs in a seat's main view. **Unstamped input is Kai's** and stays a `prompt`.

* **What marks one** - the stamp `[from <role> <identity>]` that the daemon builds in `envelope()` for every line it types into a seat (aterm send, MCP `send_message`, daemon notices and relays, feedback), and that a body cannot forge. Claude's own cross-session message carries a structured `origin.kind: "peer"`, resolved to a role through the ledger, with the bare name as `identity` when the ledger does not know it. A message queued while a Claude turn ran is a `queued_command` attachment and is read the same way.
* **`for`** - on a reply, tool or note that descends from a peer message: that peer item's id. Absent means the item answers nobody.
* **`shared`** - set only with `for`, when the peer landed inside a running turn. The files cannot separate what answers the peer from what continues Kai's earlier task, so the items from that peer to the next prompt carry `shared: true`. The client decides what to do with them.
* **How `for` is derived** - Claude follows `parentUuid`, opencode follows the assistant message's `parentID`, Codex follows `turn_id`. All three give `for` exactly when a peer starts its own turn, and `shared` when it does not.

## How each harness records a tool step

* **Claude** - a `tool_use` block in an assistant line, and its `tool_result` block (with `is_error`) in a later user line. That line also holds a richer `toolUseResult`: edit hunks for the diffstat, `stdout` and `stderr` for Bash. One content block is one line, so a reply is several lines sharing `message.id`, merged into one item.
* **Codex** - the typed `item_completed` events, not the raw `response_item` calls: `CommandExecution` (`status`, `exit_code`, `aggregated_output`), `FileChange` (`unified_diff`), `McpToolCall`. There is no error flag, so `ok` comes from `status` and `exit_code`.
* **opencode** - a `part` row with `type: "tool"` and `state: {status, input, output, metadata}`. `status` is `completed`, `error` or `running`, and a bash `metadata.exit` other than 0 is not ok.

All three files are the harness's own and unversioned, so a field not named in the readers is never read.

## Cursors and polling

* **Cursor** - opaque. A byte offset for Claude and Codex, a creation time and part id for opencode. A client never builds one.
* **Poll `since`** every 2 seconds while a view is open. An unchanged file answers without reading. There is no push yet.
* **A running tool** holds the `cursor` back to before it, so the next `since` returns the tool again once it ends.
* **Large files** are read from the end in a growing window, never whole.

## Refusals

An `error` frame with a `reason`, and the client falls back to the screen on any of them.

* `no_transcript` - no file for this seat.
* `ambiguous` - two seats share one folder, so which session is this seat's cannot be told.
* `unreadable_format` - the file is not a format the reader knows.
* `unsupported_harness` - a harness with no transcript the daemon reads.
* `bad_cursor` - a cursor this daemon did not issue, or one past a replaced file. Fetch the newest page again.
* `passkey_required` - a locked remote device.

## Changing it

Run `UPDATE_GOLDEN=1 go test -run Transcript ./aterm` to rewrite the goldens, and read the diff. The fixtures are real file structure with text, paths and ids replaced, because this repository is public.
