# The Focus, Default and Verbose views

A seat on a host whose welcome lists `transcript` shows three views of it, read from the transcript the daemon takes from the seat's own harness files (claude, codex, opencode). COI-3532. Kai chose the three stops, Focus as the phone default, and one release behind the daemon read. The frame belongs to the daemon, so [transcript.md](transcript.md) is the contract. This page is the client half.

## The rule

**The seat's PTY is the seat.** The views are a read-only lens. They never type for the seat, never answer a dialog, and the screen stays mounted under them so a dialog card still reads from it. A choice of level changes only what Treehouse shows, and never sends `/focus` to the seat.

## The three stops

Named from Claude Code's `viewMode`. **Focus** is each turn's prompt, one line per tool call and the turn's last reply. **Default** keeps every reply and three lines of each result, and says how many more there are. **Verbose** keeps all of it, and a Raw screen button inside it brings up the seat's own screen. The client cuts the output for Focus and Default, and the daemon sends it whole, to 64 KiB.

* **Opens to** Focus on a phone and the raw screen on a desktop, which is today's look. The choice is per seat on this device (`seat-view.ts`).
* **Peer messages** and the replies that answer them never show, at any level. They stay in the Messages panel. A peer item is `kind: peer`, and `for` links an answer to it. An item with no `for` answers nobody. When the peer landed mid-turn the daemon also sets `shared`, since the file cannot tell an answer from the person's own work, and the view keeps those items and says they may answer a message from that seat, with a Show the message button that opens the Messages panel on it (`withoutPeers`, `findPeerMessage`). Kai's rule is that the peer message itself never shows, and replies to her always do, so a shared item stays marked.
* **Think text** - a reply's `<think>` block (opencode writes one into the reply from some models) is left out at Focus and Default, and a reply that was only thought draws nothing. Verbose keeps it (`withoutThinking`).
* **No transcript** (a refusal such as `unreadable_format`) opens the raw screen with one line saying why. Picking a stop asks again. Every refusal but `unreadable_format` and `unsupported_harness` is also asked again every 5 seconds, and at once when a locked device unlocks, so a refusal on a cold start does not strand the seat on its screen (COI-3646, `mayClear`). A host without the feature shows no control at all.
* **It polls** `since` the last cursor every 2 seconds while open, paused while the page is hidden, and refetches the newest page on `bad_cursor`.
* **A bounded window** (COI-3648) - a seat holds at most 400 items. Past that, a seat pinned to the newest line starts over from a fresh read of the newest page, and reading back past 400 drops the newest rows, stops the 2 second poll, and offers Jump to latest, which reads the newest page again. Held work no longer grows with how long a seat has run or been watched. The journey is `transcript-window`.
* **Show earlier** at the top asks for the page `before` the reply's `older` cursor and keeps the oldest row on screen where it was. Text the host cut says so under it. A device that must unlock first (`passkey_required`) gets the screen and the reason.

## Opening on the last page (COI-3534)

A seat opens on the transcript this device last read, then catches up. The measurement behind it (2026-11-10, six live seats on aos-v0.575.0): a terminal replay is one 1 MiB frame, so the cost on a phone is the transfer, and the client never sent the `offset` the daemon's `attach` accepts. This page of work covers the Messages and Focus views. A terminal that resumes from `offset` is a separate change.

* **What is kept** - per host and seat in IndexedDB, the newest 60 items without any tool output, 12 seats at most, 24 hours (`transcript-cache.ts`). Output is left out because it is where a seat's secrets sit. Jev picked this over memory only (0.72, confidence 0.58), so it is a flag-sized change to drop it.
* **When it shows** - only on a device that may type, so a device that must unlock first never paints a kept page.
* **On open** - the kept page paints at once with `Showing what this device read 3 hours ago. Catching up.` The newest page is read in parallel and joined by item id, which brings a tool's output back. A host that does not answer leaves the kept page up and says so.
* **A view switch** lands on the newest line and keeps following, whether Transcript stays mounted or comes back from the raw screen. The marker below is for opening a seat only (COI-3647).
* **The marker** - `New since you were last here` sits above the first item that was not on the kept page, at the first row drawn at this view, and scrolls there. It is set on open only, never for lines that arrive while watching, and clears after 8 seconds with the newest line in view. When the read shares no item with the kept page, the kept page is dropped, since a gap would read as nothing having happened, and everything shown is new.
* **Journey** - `seat-open-instantly` on a phone, with the mock's `holdTranscript` to hold the host's answer.

## A message just sent (COI-3690)

The transcript shows a message only once the harness writes it, and the client polls every 2 seconds, so a message appeared 0.2 to 1.8 seconds after the harness wrote it (12 samples against the mock daemon, 2026-11-17, the poll's phase and nothing else). A real seat's write time and a device's own numbers are not measured.

* **At send** - the Composer notes the words in `pending-sends.svelte.ts` before the first byte is typed, and the view draws a dashed "You" entry marked `Sending.` on the next frame, scrolled into view.
* **Resolving** - a prompt that arrives with the same words (an attached image's path before them counts) removes the entry in the same update that merges it, so the message never shows twice. A prompt resolves one send, so two sends of the same words stay two. A first read trusts a prompt only by its stamp, within 60 seconds before the send, for host clock drift.
* **Taken while closed** - switching seats or to the raw screen closes the view, and a busy seat buries the prompt under the newest page by the time it opens again. The first read pages back by `before`, 200 items a page and 10 pages at most, without showing what it reads, until each waiting send settles or a page starts before the earliest of them. A message absorbed mid-turn has no user line in a Claude file, only a `queued_command` attachment, and the daemon reads it as a prompt too (COI-3690).
* **Queued** - a working seat holds a message and writes it only when it takes it, which ran from 2 seconds to an hour on one live seat. So past 30 seconds the entry reads `Queued.` with no Dismiss while the seat works, and turns amber only once the seat has been quiet 5 seconds without it.
* **Never taken** - after 30 seconds the entry turns amber, says `Not in the transcript yet`, and offers Dismiss. A late prompt still resolves it. A send the lock refused and returned to the field leaves no entry.
* **Pictures only** (COI-3705) - a send with no words shows `An image sent`, or `2 images sent`, and settles on the first new prompt that is only image markers such as `[Image #1]`. That marker is the harness's, and the daemon drops a Claude line with no text at all, so a harness that writes none leaves the entry to turn unsent. Not yet checked against a real seat.
* **Not covered** - a restart: entries live in memory (send history is COI-3691).
* **Journey** - `pending-send`, with the page clock moved 31 seconds for the unsent state.

## Where it lives

`src/lib/transcript.ts` (types, decoder, `rowsFor`), `src/lib/seat-view.ts`, `src/components/SeatViews.svelte` (the radio group, arrow keys, 44px stops), `src/components/Transcript.svelte`, `src/lib/pending-sends.svelte.ts`, `src/lib/transcript-cache.ts`, and the host channel `transcripts` in `daemon-host.ts`. The journey is `seat-views`, and the bed's mock answers `transcript` from `setTranscript`.
