package main

// outboxDepth is how many broadcast frames a subscriber may lag before it is
// dropped. A sessions frame carries every seat, so a few fill a socket buffer.
const outboxDepth = 256

// offer queues a broadcast frame for the subscriber's writer goroutine and never
// waits, so a stalled reader cannot block the daemon. False means it is full.
func (c *conn) offer(message frame) bool {
	c.outMu.Lock()
	if c.outbox == nil {
		c.outbox, c.gone = make(chan frame, outboxDepth), make(chan struct{})
		go c.drainOutbox(c.outbox, c.gone)
	}
	outbox := c.outbox
	c.outMu.Unlock()
	select {
	case outbox <- message:
		return true
	default:
		return false
	}
}

// drainOutbox writes queued frames in order. A failed write closes the
// connection, which ends the subscription through the reader loop.
func (c *conn) drainOutbox(outbox <-chan frame, gone <-chan struct{}) {
	for {
		select {
		case <-gone:
			return
		case message := <-outbox:
			if err := c.write(message); err != nil {
				_ = c.Close()
				return
			}
		}
	}
}
