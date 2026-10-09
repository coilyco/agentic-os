# How a session's PTY is sized

A PTY has one size, and the harness draws for it with absolute cursor moves. Several clients of different sizes attach to one session, so the daemon picks one size (COI-2609). The client half is `src/components/Terminal.svelte`.

**The policy is the largest client.** The daemon keeps the box each attached client reported and sets the PTY to the maximum columns and the maximum rows. A client attaching, resizing smaller or leaving re-chooses it. Before this, every `attach` and `resize` set the size directly, so the last client to send one won. A phone mounting its terminal squeezed every other client, and nothing restored the size when it left.

**A client's size is its capacity, not an order.** `attach` and `resize` carry `rows` and `cols` from `fit.proposeDimensions()` of the visible box. A client that pans a larger PTY says `scales: true` on `attach`. One that omits it is raw, like the kitty window, and would mis-wrap a wider PTY. The size is held to the smallest raw client's columns and rows. With only web clients it is plain largest.

**The daemon sends `{type:"size", session, rows, cols}`** to a client right after its attach, even when it is the only one and even when unchanged, and to every attached client whenever the size changes, which includes the survivors when the largest client leaves. The client sets xterm to exactly that size. The feature `pty-size` in `welcome` says the daemon does this. Without it a client keeps fitting and sending.

**A phone pans, anchored bottom-left**, showing the bottom rows when the PTY is taller than its box, since the input and the newest text are there. Scaling 200 columns into a phone is about 2px a column. This is COI-2608's layout.

**Ship the client first.** A client that predates `size` draws a wider PTY mis-wrapped, and as a raw client it also caps everyone at its own size. A client on an older daemon gets no `size` frame and keeps today's behavior.

A client that sends no box (a watcher) neither caps nor grows the size. A session with nobody attached keeps its last size.
