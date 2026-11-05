# Message text

A `message` event is one state change of a peer message, and it carries the body as `text` (COI-3418).

* **Who gets it** - only a subscriber the typing guard would let type: a local client outside every session, or a remote device after its unlock. A session's own process and a locked device get the event with no `text` field, because a body can hold private working context. The check runs per event, so a device that unlocks mid-connection gets the next one.
* **How much** - the first 32 KiB, cut at a rune. `text_cut` is true when more was sent. A send can reach 256 KiB, and the event repeats on every state change, so the whole body could stall a phone's socket and drop it from the roster.
* **What never carries it** - the sender's `sent` reply, the inbox receipt, and `sessions`. A client opened after the send sees no earlier text, since there is no replay on connect.
* **No text, no reason** - an old daemon, a refused client and a body-less event look the same, so the client says only that the host did not send the text.
* **Seam** - `broadcastMessage` in `aterm/daemon.go` builds the per-client frame, and `bodyHead` in `aterm/send_limit.go` owns the cut.
* **One process read** - the typing check walks the process table, and a state change fires on the delivery path under the PTY write lock. `descender` reads the table once per event for all subscribers, so do not call `typingRefusal` per subscriber there.
