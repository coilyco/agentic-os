# Transcript search

**The daemon searches a seat's whole harness transcript, and the client jumps to a match even when it sits in a page the view never loaded** (COI-3797). The view holds the newest 50 items and pages back with `before` ([seat-views.md](seat-views.md)), so a phrase from an hour ago in a long seat is not on the client, and the search cannot run there. Code: `aterm/transcript_search.go`, `aterm-client/src/components/TranscriptSearch.svelte`, `aterm-client/src/lib/transcript-search.ts`.

## The frame

* **Feature** - `transcript_search` in `welcome`. A client without it shows no search, and Find opens the screen's own find as before.
* **Request** - `{type: "transcript_search", id, session, query}`. `query` is 1 to 200 characters, trimmed.
* **Reply** - `{type: "transcript_search", id, session, reader, query, matches, total, more?, updated_at}`. `matches` is oldest first, at most the newest 500. `total` counts all of them. `more` says the file was longer than the 128 MiB a read covers, so its oldest part was not searched.
* **A match** - `{id, kind, at?, field, snippet, before?}`. `field` is `text`, `summary` (a tool's line, the command as typed where it differs) or `output`. `snippet` is the first hit with 60 characters either side, on one line.
* **`before`** - the cursor a `transcript` read sends to page the match in: its page ends 8 items past the match, so what followed shows too. Absent means the newest page holds it. It is the same opaque cursor `older` carries, so a client never builds one.
* **Gate** - the same as `transcript`, since a snippet of tool output can hold a secret. Refusals are the transcript's reasons.

## Every seat at once

* **Feature** - `transcript_search_all` in `welcome`.
* **Request** - `{type: "transcript_search", id, all: true, query}`, with no session.
* **Reply** - `{type: "transcript_search", id, query, seats, updated_at}`. `seats` holds one entry per live seat, by name: `{session, reader?, matches, total, more?, reason?}`, with that seat's newest 50 matches. A seat that cannot be read carries its refusal `reason` and no matches, so one harness without a transcript never hides the rest.
* **The client** - a Search every seat field at the top of the Seats overview. Results group by seat, newest first, and each leads with what it is (You, Reply, Tool, Tool output, Note) and marks the phrase. A tap opens that seat on its transcript, moving a desktop off its raw screen, with the seat's own search bar filled in and the match paged in (`search-jump.svelte.ts`).

## How a search runs, and what it costs

* **Off the read loop** - a search runs on its own goroutine, so the asking connection keeps taking polls and typing. A newer `transcript_search` from the same connection cancels its search still running, which ends with an `error` carrying reason `search_cancelled`. The client drops that reply, since it already asked again. Closing the connection cancels its search too.
* **Two at once** - at most 2 searches run across the daemon. The rest wait for a slot, and a waiting search can still be cancelled.
* **A cache of parsed items per seat** - kept while the seat's file (and opencode's `-wal`) keeps the same size and modification time. A grown file is parsed again. The cache holds at most 256 MiB of item text across every seat, and drops the least recently searched seat first. A seat larger than the whole budget is searched and not kept. A seat's items leave when its session ends.
* **An ASCII phrase** is found in the bytes with no copy. Any other phrase folds rune by rune.
* **Measured** (COI-3797, 2026-11-28, M3 Pro, `ATERM_MEASURE_SEARCH` in `aterm/transcript_search_measure_test.go`):
  * Every seat over 10 synthetic seats of 12.8 MB: 1.23 s before the cache, 80 ms after it. With 3 of the 10 seats growing between searches, 331 ms.
  * One real seat of 12.9 MB, read without the cache: 41 ms.
  * One synthetic seat of 127 MiB, read without the cache: about 1 s, with a 617 MiB peak while it parses.

## What matches

* Case folds rune by rune, so `É` finds `é` and `ß` never becomes `ss`.
* A peer message, and an item that answers only a peer (`for` without `shared`), never match, since neither shows in a seat's view.
* Every harness the read serves (claude, codex, opencode) is searched by the same rule. The file readers parse the file once from its end, and opencode reads every part of the session in one query.

## The client

* **Opening it** - on a desktop, Cmd+F, Ctrl+F, Ctrl+Shift+F, or the Find button in the seat's header, while a transcript view shows. A phone has a search button at the bottom right of the transcript, clear of the screen's Wrap and Aa corner. In a raw view, Find stays the terminal's own find.
* **The bar** - a field, a count such as `3 of 12`, Older and Newer buttons, and Close. It searches 300ms after typing stops. Enter steps to an older match, Shift+Enter to a newer one, and both wrap. Escape closes it and clears the marks.
* **Where it starts** - the newest match, since the view sits at the newest line, so the count reads from there.
* **A jump** - a match not held reads its page by `before`, which stops following the newest line, as reading far back does, until Jump to latest. The match is scrolled to the middle of the view and draws whole, as Verbose would, so a reply Focus leaves out or a line deep in a tool's output still shows.
* **Marks** - every hit in the view is marked, the current match more strongly, with the CSS Custom Highlight API. A browser without it still jumps, with no marks.

## Not covered

* Text a seat wrote as markdown matches its source, so a phrase split by formatting is found by the host and may not be marked on screen.

## Tests

* `go test -run 'TranscriptSearch|Snippet' ./aterm` - every item of each harness's fixture is paged in by its own cursor, peers never match, and a phrase at the start of a 1,401-line file is found and paged in.
* Journey `transcript-search` at every size - no match, one match from item 3 of 1,200, and twelve with Older, Newer and wrapping.
* Journey `seat-search` at every size - two seats' matches grouped, a seat without one left out, and a tap that opens Beetle at item 5 of 606.
* `go test -run TranscriptSearchCoversEveryLiveSeat ./aterm` - three claude seats and a goose seat in name order, the goose one naming `no_transcript`.
