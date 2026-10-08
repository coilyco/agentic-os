# Terminal split and side-tab defaults

The Terminal tab sits beside Messages, Views and Browser in a seat's side panel (COI-2499). Source: `src/lib/side-tab.ts`, `src/lib/terminals.ts`, `src/components/TerminalPane.svelte`.

## Default tab by role

* **Table** - `ROLE_DEFAULTS` in `side-tab.ts` is the only place a role name picks a tab. `sysadmin-*` opens Terminal, `frontend-*` Browser, `prod-director` and every other role Messages.
* **Override** - clicking a tab remembers it per role in `aterm.side-tab.v1`, so every instance of that role opens on it. The role's default never overwrites a stored pick.

## Pane states

The worst state is checked first (`paneState`). Each has words in `paneText`.

* **unsupported** - the host carries no terminal channel. This is what the real daemon shows until COI-2498 lands.
* **waiting** - the host has not sent its first terminal list. The pane never opens a shell before it, or a reload would start a second one.
* **opening, refused** - a spawn in flight, then the host's reason with Try again. An unanswered spawn becomes a refusal after eight seconds.
* **locked** - the typing guard refuses this browser. An existing shell can be watched, and no new one is offered.
* **closed, ready** - none beside this seat. Choosing the tab opens one unless Kai closed it, which `aterm.terminal-closed.v1` remembers.
* **live, exited** - a shell, or a read-only one with its exit code and Open a new terminal.

## Split and keyboard

The split handle is a window splitter. Drag, Left and Right (Shift for larger steps), Home, End, or double-click to reset. The width is remembered in `aterm.side-width.v1`. Below 1000px the panel stacks and the terminal takes a taller sheet. The shell takes Tab, so Ctrl+Alt+Left returns focus to the composer and Ctrl+Alt+Right enters the shell.

## Demo host modes

`?terminals=` picks one: `ok`, `off` (no channel), `refuse`, `silent` (never answers), `mute` (no first list), `locked` (typing refused). They script the frame proposed on COI-2498, which is not yet accepted.

## Left-sidebar terminal

Not built, since Kai is undecided. A host terminal would be an entry with no seat label in `app.terminals`, shown by the same pane and `Terminal.svelte` with an id of its own. Only `terminalFor`, which finds a seat's shell by label, would need a sibling.
