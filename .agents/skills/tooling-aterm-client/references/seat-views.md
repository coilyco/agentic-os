# The Focus, Default and Verbose views

A seat on a host whose welcome lists `transcript` shows three views of it, read from the transcript the daemon takes from the seat's own harness files (claude, codex, opencode). COI-3532. Kai chose the three stops, Focus as the phone default, and one release behind the daemon read. The frame belongs to the daemon, so [transcript.md](transcript.md) is the contract. This page is the client half.

## The rule

**The seat's PTY is the seat.** The views are a read-only lens. They never type for the seat, never answer a dialog, and the screen stays mounted under them so a dialog card still reads from it. A choice of level changes only what Treehouse shows, and never sends `/focus` to the seat.

## The three stops

Named from Claude Code's `viewMode`. **Focus** is each turn's prompt, one line per tool call and the turn's last reply. **Default** keeps every reply and three lines of each result, and says how many more there are. **Verbose** keeps all of it, and a Raw screen button inside it brings up the seat's own screen. The client cuts the output for Focus and Default, and the daemon sends it whole, to 64 KiB.

* **Opens to** Focus on a phone and the raw screen on a desktop, which is today's look. The choice is per seat on this device (`seat-view.ts`).
* **Peer messages** and the replies that answer them never show, at any level. They stay in the Messages panel. A peer item is `kind: peer`, and `for` links an answer to it. An item with no `for` answers nobody. When the peer landed mid-turn the daemon also sets `shared`, since the file cannot tell an answer from the person's own work, and the view keeps those items and says they may answer a message from that seat, with a Show the message button that opens the Messages panel on it (`withoutPeers`, `findPeerMessage`). Kai's rule is that the peer message itself never shows, and replies to her always do, so a shared item stays marked.
* **Think text** - a reply's `<think>` block (opencode writes one into the reply from some models) is left out at Focus and Default, and a reply that was only thought draws nothing. Verbose keeps it (`withoutThinking`).
* **No transcript** (a refusal such as `unreadable_format`) opens the raw screen with one line saying why. Picking a stop asks again. A host without the feature shows no control at all.
* **It polls** `since` the last cursor every 2 seconds while open, paused while the page is hidden, and refetches the newest page on `bad_cursor`.
* **Show earlier** at the top asks for the page `before` the reply's `older` cursor and keeps the view where it was. Text the host cut says so under it. A device that must unlock first (`passkey_required`) gets the screen and the reason.

## Where it lives

`src/lib/transcript.ts` (types, decoder, `rowsFor`), `src/lib/seat-view.ts`, `src/components/SeatViews.svelte` (the radio group, arrow keys, 44px stops), `src/components/Transcript.svelte`, and the host channel `transcripts` in `daemon-host.ts`. The journey is `seat-views`, and the bed's mock answers `transcript` from `setTranscript`.
