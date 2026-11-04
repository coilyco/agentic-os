# Native choices

How the client turns a harness's own select menu into buttons. The surrounding design is in [architecture.md](architecture.md).

## Reading the menu

A harness draws its own select menu, so the client reads the visible rows for one: a `❯` cursor row, sibling options at its indent, deeper lines as descriptions, and an `Enter to` or `Esc to` footer within five rows, since a narrow terminal wraps it. The footer is required, since shells and Claude Code's own input prompt use the same glyph. Picking an option sends arrows from the cursor to it, then any typed text, then Enter, 150ms apart, because a free-text row only takes typing once the cursor is on it. The card overlays the terminal rather than shrinking it, since a resize makes the harness redraw a scrolled menu, which reads as a different menu and loops. A menu missing for one frame is a redraw in progress, so the card hides only after 400ms without one. The rows come from the live screen, never the scrolled viewport, so reading scrollback does not close the card. Options scroll inside the card while its buttons stay pinned, and a menu's own `Next` or `Submit` row moves into that footer, so no control needs a scroll to reach.

## Multi-question dialogs

Read from Claude Code v2.1.318, captured at 134, 72, 44 and 34 columns. A dialog of several questions opens with a tab bar (`←  ☐ Accent  ☒ Routes  ✔ Submit  →`) that names every question and not the current one, so the card drops it. A multi-select draws `[ ]` or `[✔]` before each label and ends its list with an unnumbered `Next` row, or `Submit` on the last question. That row can hold the cursor, so it is a row like any other. Enter toggles a checkbox and leaves the cursor there, and typing into `Type something` checks its row, which Enter would uncheck, so that row gets text only. The card toggles what differs, then presses Next. The review screen that ends the dialog (`1. Submit answers`, `2. Cancel`) draws no footer, so it is read by its Submit row. A card is held down for the question it just answered while its own keys redraw it.

A terminal shorter than the dialog scrolls the top options out of view, and then the card drops them or does not show.

A card is the same card while only digits in the dialog's text change (an away-timer, a counter), and its question updates in place. Its text box, draft and ticked rows live in `card-memo.svelte.ts`, keyed by seat and options, so a card rebuilt for any reason comes back with them and the keyboard. They are dropped when the card is answered or cancelled, or 5 seconds after the dialog stays gone (COI-3314).

## The durable path

Screen reading is per harness and breaks when a harness redraws its menu differently. Kai chose `ask_choice`: a tool on the daemon's MCP server that any seat can call. The daemon broadcasts an `ask` frame, the client answers with the picked indexes and any typed text, and an `asked` frame closes the card everywhere. An ask outranks a screen menu in the same seat, and because it is a daemon event it can flag a seat you are not viewing. The frame shape in `daemon-host.ts` is the one proposed to eng-platform, and the daemon's version wins.

## A worker's question on the director's card

A worker that needs Kai calls `send_user_question`, shaped like `ask_choice`, instead of messaging the director (COI-3241). The daemon sends the same `ask` frame with `relay` and `card_seat` added. `session` and `from` still name the asking worker, so its row stays marked as asking. `card_seat` names the session whose pane shows the card. The daemon stamps it, and a caller cannot set it.

* **Where it lands.** The live `prod-director` session other than the asker. With several, the one a client has open, then the one typed into last, then the oldest. With none up, `card_seat` is empty and the card shows on the asker, as an `ask_choice` card does.
* **Busy or closed.** A card is a client frame, so a busy director shows it at once. If the director ends while the question is pending, the daemon sends the same `ask` again with the next director, or an empty `card_seat`.
* **The answer.** `answered` goes back to the asking worker with `card_seat` on it (MCP `shown_on`), and the director gets one `[from aterm daemon]` line with the answer. A cancelled or timed-out question reaches the director the same way.
* **Not built.** The director cannot edit or add context before the card reaches Kai.
* **The client.** Shows the card on `card_seat` when that seat is live, else on the asker, and names the asker on it. Both rows are marked until `asked`: the asker's says "asking you, on Griffin-Goose" and the director's "Frog-Ox asks you". The glow, sound, toast and title point at the pane to open, once. The asker's own pane says where its question is, with a button that goes there, from `RelayNotice.svelte`, which `no-jump.test.ts` lists as click-only. A repeated `ask` with a known id replaces the first, which is a move. A daemon without feature `ask-relay` never sets the fields, and the tool refuses to run against one.
