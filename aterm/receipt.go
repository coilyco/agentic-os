package main

import (
	"fmt"
	"time"
)

// maxSendWait caps how long a send holds its reply for the final state.
const maxSendWait = 2 * time.Minute

// finalState is whether a message has nothing left to report.
func finalState(state string) bool { return state == "delivered" || state == "failed" }

// armReceipt asks for a receipt typed into the sender once this message is final,
// for one that was not when the sender was answered. See docs/aterm-daemon.md.
func (p *pendingSend) armReceipt() {
	p.mu.Lock()
	p.armed = true
	final := finalState(p.msg.State)
	p.mu.Unlock()
	if final {
		p.sendReceipt()
	}
}

// sendReceipt types the receipt once, and only when one was armed.
func (p *pendingSend) sendReceipt() {
	p.mu.Lock()
	if !p.armed || p.receiptSent || p.sender == nil {
		p.mu.Unlock()
		return
	}
	p.receiptSent = true
	snapshot, sender := p.msg, p.sender
	p.mu.Unlock()
	deliverReceipt(sender, snapshot)
}

// deliverReceipt queues a daemon-stamped line in the sender's session, behind Kai's
// draft like any message. Its own state is neither reported nor receipted.
func deliverReceipt(sender *ptySession, message peerMessage) {
	select {
	case <-sender.done:
		return
	default:
	}
	notice := &pendingSend{
		msg:  peerMessage{ID: randomID(6), From: "aterm daemon", Target: sender.name, Accepted: time.Now().UTC()},
		text: envelope("aterm", "daemon", receiptText(message)),
		done: make(chan struct{}),
	}
	sender.enqueue(notice)
}

// receiptText names the message and where it landed. It never carries the body.
func receiptText(message peerMessage) string {
	where := message.Session
	if where == "" {
		where = message.Target
	}
	line := fmt.Sprintf("message %s to %s: %s", message.ID, where, message.State)
	if message.Reason != "" {
		line += ": " + message.Reason
	}
	return line
}
