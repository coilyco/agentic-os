# Delivery to a busy claude seat

`aterm send` into a claude seat that is mid-turn types the message and presses Enter. A busy seat can hold that Enter back for a minute or more, so the daemon used to give up after 55 seconds and report `failed: its prompt took the text but not Enter`, although the seat took the message moments later (COI-2619). The director's failed send at 05:08Z on 2026-10-09 was enqueued by the seat's own harness at 05:09:01.

## What the daemon does now

* **A seat that is working is parked, not failed.** When the Enter retries run out and the box still holds the text, the daemon checks the screen for claude's spinner (`(3m 1s · ↓ 141 tokens)` or `esc to interrupt`). With the spinner up, the message is **parked**: it leaves the send queue, its text stays in the box, and its state is `held`.
* **An idle seat is still `failed`.** No spinner means the seat should have taken the Enter, so the old verdict stands. Recent output does not count as work, because typing the message makes an idle seat echo it.
* **Messages behind it wait.** A later message to the same seat is `held` until the parked one settles, so two never share one box.
* **Settling is read off the screen.** The daemon presses Enter again every 10 seconds while the box holds the text and no card is up. The message is `delivered` once the text has left the box and shows above it on two reads running.
* **It ends.** After 30 minutes, or at once when a person types into the box, the message is `failed` and the reason names the unsent text. A session that exits fails it too.

The senders' `--wait` returns the `held` state when its time runs out, and the receipt typed into the sender's session reports the final state.

## Not explained

Why the seat held the Enter back is not known. Delivery to a busy seat worked in every probe run for this record: 250 bytes, 8 KB and 20 KB messages sent into a long Bash call, a long turn, and a host under load all arrived byte for byte in the seat's own transcript. The parked state makes the verdict correct whatever the cause. It does not find the cause, and it does not explain the cut-off message end Kai described, which has not been reproduced.

## Test it

* **`TestMessageThatABusySeatTakesLateIsDeliveredNotFailed`** and its neighbours use a fake seat that shows a spinner and drops Enter until a deadline.
* **`TestLiveClaudeBusySeat`** runs a real claude seat. Set `ATERM_LIVE_CLAUDE` to the binary, `ATERM_LIVE_CLAUDE_KEEP=CLAUDE_CONFIG_DIR`, and `CLAUDE_CONFIG_DIR`. It sends three messages into a long Bash call and compares the seat's transcript to the bytes sent.
* **`testdata/claude-transcripts/queued-in-tool-call.txt`** is a real screen with three messages queued behind a tool call. Recapture it after a Claude Code upgrade.
