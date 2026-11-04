package main

import (
	"context"
	"crypto/rand"
	"errors"
)

// unlockTokenFeature tells a client a channel unlocks with an accounts token, which
// the welcome's unlock member offers a nonce for.
const unlockTokenFeature = "unlock-token"

// unlockFailLimit is how many refused tokens a channel may present before its nonce ends.
const unlockFailLimit = 5

// unlockOffer is the welcome member of a brokered channel. See the contract's
// treehouse.frame.welcome-unlock.v1.
type unlockOffer struct {
	DaemonID string `json:"daemon_id"`
	Nonce    string `json:"nonce"`
}

// brokeredGate is the typing gate for brokered channels. An accounts token takes the
// place of a passkey assertion, and a success sets client.asserted as that did.
type brokeredGate struct {
	verifier *unlockVerifier
	host     userHost
}

// challenge gives a brokered channel a fresh nonce that only a token carrying it spends.
func (b *brokeredGate) challenge(cl *client) *unlockOffer {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return nil
	}
	cl.unlockNonce = rawURL.EncodeToString(raw)
	return &unlockOffer{DaemonID: b.verifier.identity.ID, Nonce: cl.unlockNonce}
}

// unlockFrame answers an unlock frame. The token never reaches a log or a reply.
func (b *brokeredGate) unlockFrame(cl *client, message frame) error {
	if b == nil || !cl.brokered {
		return withExit(exitUsage, errors.New("unlock belongs to a channel the Treehouse broker opened"))
	}
	if cl.asserted {
		return withExit(exitUsage, errors.New("this channel is already unlocked"))
	}
	refused := withReason(reasonUnlockRefused, errors.New("this unlock was refused, reconnect to try again"))
	if cl.unlockNonce == "" {
		return refused
	}
	if err := b.verifier.check(context.Background(), message.UnlockToken, cl.unlockNonce); err != nil {
		b.host.logf("a brokered unlock was refused: %v", err)
		if cl.unlockFails++; cl.unlockFails >= unlockFailLimit {
			cl.unlockNonce = ""
		}
		return refused
	}
	cl.asserted, cl.unlockNonce = true, ""
	b.host.logf("a brokered channel unlocked with an accounts token")
	return cl.c.write(frame{Type: "typing", ID: message.ID, Typing: b.host.typingStanding(cl)})
}
