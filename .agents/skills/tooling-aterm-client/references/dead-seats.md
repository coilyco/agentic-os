# Dead seats, Reopen, and the window a close ends

A seat ended by `aterm close` left its kitty window open, and a seat whose holder died stayed "running when last seen" in `aterm resume --list` with no way back from Treehouse (COI-3799).

## The window closes on a close

* **`exit` and `exited` carry `ended_by`.** `close` for `aterm close`, `close self` and a terminal's close, `reload` for a reload, `lost` for a holder that went without an exit frame. Absent means the harness exited on its own.
* **A window holds only a failure.** The `_session` stage in the kitty window used to hold any non-zero exit on "Press Enter to close". A forced close ends a harness at SIGTERM's 143, or 1 when it traps TERM, as often as at 0, so whether the window went away depended on the exit code. Now `close` and `reload` end the stage whatever the code, and kitty's window goes with it. A crash or a lost holder still holds, so its last screen is readable.
* **Not retroactive.** A window opened by an older `aterm` runs that binary until it ends, so it still holds. Windows opened after the upgrade close.

## A dead seat is found and marked

* **While the daemon watches.** A holder connection that drops with no exit frame, or a child gone for a second with none, ends the session as `lost`. Its ledger record gets `ended` and `lost: true`.
* **After it was not watching.** On start, after adopting every holder that answers, and every 30 seconds after, the daemon marks dead each record with no `ended` whose name and start time it does not hold. That is a reboot, a holder killed while no daemon ran, or the old "running when last seen" records. A holder that answered but failed adoption stops the sweep, since its seat may still run.
* **`aterm resume --list`** shows such a record as `dead, holder gone, found <time>`.

## Treehouse offers Reopen

* **The `dead` list.** `list`, `subscribe` to `sessions`, and every roster push carry `dead` beside `sessions`: `name`, `ref`, `role`, `identity`, `seat`, `started`, `ended`. Newest first, at most 12, started in the last 7 days, and only seats whose conversation the harness can resume. Absent means none.
* **Leaving the list.** A reopen through the daemon stamps the record `reopened`. A later launch of the same conversation, such as `aterm resume`, counts as reopened too. A live seat with the same `name@ref` is never listed.
* **The client.** The seat overview, on every width, and the desktop sidebar show a Dead group under the other seats, each row with the role, how long ago it was found dead, and Reopen. Reopen sends `reopen` with `name@ref`, which resumes the same conversation headless (`aterm resume`'s path), and the seat comes back under its recorded identity where COI-2819 allows. A dead seat whose pool name a new seat now holds still reopens, by its ref. Reopen shows only with the daemon's `reopen` feature, a typing connection and a live socket.
* **Trying it with no daemon.** `?dead=1` on the dev page gives the Mock host one dead seat. `just aterm-client-drive` checks it at 1280px and 320px, by keyboard.
