# Native choices

How the client turns a harness's own select menu into buttons. The surrounding design is in [architecture.md](architecture.md).

## Reading the menu

A harness draws its own select menu, so the client reads the visible rows for one: a `❯` cursor row, sibling options at its indent, deeper lines as descriptions, and an `Enter to` or `Esc to` footer within three lines. The footer is required, since shells and Claude Code's own input prompt use the same glyph. Picking an option sends arrows from the cursor to it, then any typed text, then Enter, 150ms apart, because a free-text row only takes typing once the cursor is on it. The card overlays the terminal rather than shrinking it, since a resize makes the harness redraw a scrolled menu, which reads as a different menu and loops. A menu missing for one frame is a redraw in progress, so the card hides only after 400ms without one.

## The durable path

Screen reading is per harness and breaks when a harness redraws its menu differently. The durable path is a structured ask over the daemon's MCP server that any harness can call, answered by the same card.
