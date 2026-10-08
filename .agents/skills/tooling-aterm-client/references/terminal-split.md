# Terminal split and side-tab defaults

The Terminal tab sits beside Messages, Views and Browser in a seat's side panel (COI-2499). Source: `src/lib/side-tab.ts`, `src/lib/terminals.ts`, `src/components/TerminalPane.svelte`.

## Default tab by role

* **Table** - `ROLE_DEFAULTS` in `side-tab.ts` is the only place a role name picks a tab. `sysadmin-*` opens Terminal, `frontend-*` Browser, `prod-director` and every other role Messages.
* **Override** - clicking a tab remembers it per role in `aterm.side-tab.v1`, so every instance of that role opens on it. The role's default never overwrites a stored pick.

## Pane states

The worst state is checked first (`paneState`). Each has words in `paneText`.

* **unsupported** - the welcome's `features` lacks `terminals`, a daemon older than COI-2498.
* **waiting** - the welcome or the first terminal list has not arrived. The pane opens no shell before then, or a reload would start a second one.
* **opening, refused** - a spawn in flight, then the host's reason with Try again. An unanswered spawn becomes a refusal after eight seconds.
* **locked** - the typing guard refuses this browser. An existing shell can be watched, and no new one is offered.
* **closed, ready** - none beside this seat. Choosing the tab opens one unless Kai closed it, which `aterm.terminal-closed.v1` remembers.
* **live, ended** - a shell, or a read-only one with Open a new terminal. A shell that drops out of the list is shown as ended at once, and its exit code fills in when the `exit` frame comes, since the daemon fixes no order between the two.

## The frame

Terminals arrive in the `sessions` frame's own `terminals` list, never in `sessions`. The daemon never says which seat a shell sits beside, so the client keeps terminal id to seat in `aterm.terminal-seats.v1` from the `spawned` reply. A second device sees the shells and none of that association. A remote device is refused with `remote_terminal` until COI-2488.

## Split and keyboard

The split handle is a window splitter. Drag, Left and Right (Shift for larger steps), Home, End, or double-click to reset. The width is remembered in `aterm.side-width.v1`. Below 1000px the handle goes and the panel is a bottom sheet instead. The shell takes Tab, so Ctrl+Alt+Left returns focus to the composer and Ctrl+Alt+Right enters the shell.

## Demo host modes

`?terminals=` picks one: `ok`, `early` (reply before the list), `off` (no channel), `refuse`, `silent` (never answers), `mute` (no first list), `locked` (typing refused). They script COI-2498's frame, with the list ahead of the `spawned` reply and the exit.

## Left-sidebar terminal

Not built, since Kai is undecided. A host terminal would be an entry with no seat label in `app.terminals`, shown by the same pane and `Terminal.svelte` with an id of its own. Only `terminalFor`, which finds a seat's shell by label, would need a sibling.
