# Native choices

How the client turns a harness's own select menu into buttons. The surrounding design is in [architecture.md](architecture.md).

## Reading the menu

A harness draws its own select menu, so the client reads the visible rows for one: a `❯` cursor row, sibling options at its indent, deeper lines as descriptions, and an `Enter to` or `Esc to` footer within five rows, since a narrow terminal wraps it. The footer is required, since shells and Claude Code's own input prompt use the same glyph. Picking an option sends arrows from the cursor to it, then any typed text, then Enter, 150ms apart, because a free-text row only takes typing once the cursor is on it. The card overlays the terminal rather than shrinking it, since a resize makes the harness redraw a scrolled menu, which reads as a different menu and loops. A menu missing for one frame is a redraw in progress, so the card hides only after 400ms without one. The rows come from the live screen, never the scrolled viewport, so reading scrollback does not close the card. Options scroll inside the card while its buttons stay pinned, and a menu's own `Next` or `Submit` row moves into that footer, so no control needs a scroll to reach.

## Multi-question dialogs

Read from Claude Code v2.1.318, captured at 134, 72, 44 and 34 columns. A dialog of several questions opens with a tab bar (`←  ☐ Accent  ☒ Routes  ✔ Submit  →`) that names every question and not the current one, so the card drops it. A multi-select draws `[ ]` or `[✔]` before each label and ends its list with an unnumbered `Next` row, or `Submit` on the last question. That row can hold the cursor, so it is a row like any other. Enter toggles a checkbox and leaves the cursor there, and typing into `Type something` checks its row, which Enter would uncheck, so that row gets text only. The card toggles what differs, then presses Next. The review screen that ends the dialog (`1. Submit answers`, `2. Cancel`) draws no footer, so it is read by its Submit row. A card is held down for the question it just answered while its own keys redraw it.

A terminal shorter than the dialog scrolls the top options out of view, and then the card drops them or does not show.

## The durable path

Screen reading is per harness and breaks when a harness redraws its menu differently. Kai chose `ask_choice`: a tool on the daemon's MCP server that any seat can call. The daemon broadcasts an `ask` frame, the client answers with the picked indexes and any typed text, and an `asked` frame closes the card everywhere. An ask outranks a screen menu in the same seat, and because it is a daemon event it can flag a seat you are not viewing. The frame shape in `daemon-host.ts` is the one proposed to eng-platform, and the daemon's version wins.
