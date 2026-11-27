# Host kind

Every host carries a kind, `personal`, `ephemeral` or `durable` (COI-3767). The shape is `host_kind` in treehouse-servers' `contract/treehouse.defs.v1.json`, and the frame is `contract/treehouse.frame.host-kind.v1.json`.

* **personal** - a desktop or laptop a person uses and closes. It may sleep, so keep-awake runs here (COI-3768).
* **ephemeral** - a batch machine that ends when its work is done.
* **durable** - an always-on server, the only kind that hosts native Treehouse Artifacts (COI-3756).

## What the daemon does

**A host never told its kind is `personal`.** The daemon keeps the kind in `host.json` beside the session ledger (`~/.local/state/aterm/host.json`, or beside `ATERM_STATE_DIR`). A missing, unreadable or unknown file reads as `personal` and logs why. No host name is special. A server is durable only because someone set it.

**Setting it.** `aterm host kind` prints the kind, and `aterm host kind durable` sets it, over the local socket. A client sends `host_kind` with a `host_kind` member to set it and without one to read it, and gets a `host_kind` reply with the kind now held. A change from a websocket is judged as typing is, so a locked tailnet or brokered device gets `passkey_required`, and a browser a seat started gets `session_descendant`. The file is written before the change is reported, so a kind that did not persist is never shown.

**Reporting it.** Every `welcome` carries `host_kind` and the feature `host-kind`. Each change pushes a `host_kind` frame with no `id` to every `sessions` subscriber. Each `hosts` entry carries the peer's `host_kind` from its welcome, absent from a daemon that predates it.

**The broker hears it** through `treehouse.reg.host.v1`, sent after `reg.ready` and on each change, only when ready lists the `host-kind` feature. An older broker ends a link on any message it does not know, so the daemon never sends it one. The broker lists the kind as `kind` in `treehouse.daemon.list.v1`.

## What the client does

A host row in the sidebar leads with the kind, as `Durable // 2 running`. The attached host's kind comes from its welcome and its `host_kind` pushes. A saved host's comes from its probe's welcome, a found host's from `hosts`, and a brokered machine's from the broker list. A host that never named one shows no kind. On a phone the host rows are chips that drop every detail, so the kind shows on the host page and in Settings.

The select (Personal, Ephemeral, Durable) is the **Host kind** group in Settings, for the attached host only, since only that connection can send the change. A locked device reaches the host page and not the seat overview, and it sees the same select there, disabled, with the reason. A refused change says the guard's words and puts the select back. A daemon without `host-kind` gets no select. The client code is `src/lib/machine-kind.ts` and `src/components/MachineKindPicker.svelte`. It says machine kind, because `Host.kind` already means daemon or mock.

## For daemon code

`d.hostKind()` is the kind, from any goroutine. `d.onHostKind(fn)` calls `fn` with the current kind at once and again after each change, from the goroutine that made it, with no daemon lock held. `fn` must not block. The code is `aterm/host_kind.go`.
