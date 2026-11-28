# Kai's draft in an aterm seat

How the daemon decides a seat holds Kai's unsent draft, the `kai_drafting` flag on a session row.
The flag holds peer messages back so they do not land on top of her line, and it makes
[`aterm close`](../../../../docs/aterm-daemon.md) and `aterm clear` refuse without `--force`.

**The draft is counted from Kai's keys, not read from the screen.** `trackDraft` in `aterm/daemon_session.go` reads every key a person types through a client. Printable keys add one, Backspace removes one, and Enter, Ctrl+C and Ctrl+U set it to zero. A paste counts all of its characters, newlines included. Escape sequences and a terminal's replies to queries (OSC, DCS, cursor and device reports, mouse reports) are not text.

**Keys arrive in two encodings, and both are read.** Claude Code turns on the kitty keyboard protocol (`CSI > 5 u`), so in a seat Ctrl+C arrives as `CSI 99;5u` and Ctrl+U as `CSI 117;5u`, not as `0x03` and `0x15`. A `CSI code;mods u` key clears, sends or edits the count exactly as its legacy byte does. A release event (`:3`) is ignored after its press, and caps lock and num lock are not chords. Under the report-all-keys flag, text arrives as `CSI code u` and still counts. Shift+Enter (`CSI 13;2u`) is a newline inside the draft, not a send.

**Two Escape presses in a row clear a seat's draft**, because Claude Code clears its input line on Esc Esc. A terminal session keeps its count, since a shell completes on Esc Esc and keeps the line. A legacy Escape is one ESC byte, which is also how every escape sequence starts, so a chunk ending on a lone ESC counts as one press. A key's bytes arrive in one chunk from a client, so an arrow split across chunks is rare, and any other key between two Escapes resets the run.

**What it still cannot see.** A harness that clears its line some other way, a slash command that empties the box, or a click in a client's own composer leaves the count where it was. Before COI-3841 the kitty forms and Esc Esc were in that list, which is how `game-dev-keen-whale` showed "Kai drafting" over an empty line and refused a close.
