# Session names, refs, and closing the right seat

A session's name is reused, so a name from an old listing can belong to a different seat now. Closing it by that name ended the wrong seat (COI-3403).

* **`ref`.** Every session in `list` carries `ref`, a stable 8-hex id from its name and start time. It changes when the name is reused. `aterm agents` prints it as the last word of each line.
* **`name@ref`.** `close` and `clear` take it as the target: `aterm close beetle-ox-15@1a2b3c4d`. It is exact. A wrong ref refuses.
* **A seat must have seen what it closes.** The daemon remembers, per calling seat, what it last listed (and what a send told it about). A `close` or `clear` by bare name refuses when the seat never listed that name, or listed a different session under it. The refusal gives the current ref. Kai's client and any caller outside a session are not held to this.
* **Names are retired for a day.** A name that ended within 24 hours is skipped when a launch is handed a name, and the daemon reads the ledger at start so a restart does not forget. A resume relaunches under the ledger's name on purpose and is not affected.
* **Older clients.** An `aterm` that sends `list` without its token is attributed to the seat it runs under by process tree.
* **Not covered.** `send` by name can still reach a reassigned name. It misdirects a message and does not end a seat.
