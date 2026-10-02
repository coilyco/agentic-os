package main

import (
	"strings"
	"testing"
	"time"
)

// sendWaiting is a send that holds its reply for the final state, as --wait asks.
func sendWaiting(t *testing.T, token, target, body string, wait int) peerMessage {
	t.Helper()
	c, err := dialDaemon(false)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	reply, err := c.request(frame{Type: "send", Token: token, Target: target, Body: body, Wait: wait})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	return *reply.Message
}

// drain reads a client's frames for a while, keeping its output.
func (tc *testClient) drain(window time.Duration) {
	deadline := time.Now().Add(window)
	for {
		_ = tc.c.raw.SetReadDeadline(deadline)
		message, err := tc.c.read()
		if err != nil {
			return
		}
		if message.Type == "output" {
			tc.output.Write(message.Data)
		}
	}
}

// spawnPair starts a sender that echoes its token and takes pastes, and a target
// that takes pastes and echoes them.
func spawnPair(t *testing.T, targetScript string) (token string, sender, target *testClient) {
	t.Helper()
	testDaemon(t)
	sender = dialTest(t)
	sender.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox",
		`echo TOKEN=$ATERM_SESSION_TOKEN; printf 'READY\033[?2004h\n'; cat`)
	token = sender.token()
	target = dialTest(t)
	target.spawn("scientist-evie", "scientist", "Evie", targetScript)
	target.until("READY")
	return token, sender, target
}

const pasteCat = `printf 'READY\033[?2004h\n'; cat`

func kaiTypes(t *testing.T, target *testClient, text string) {
	t.Helper()
	if err := target.c.write(frame{Type: "input", Session: "scientist-evie", Data: []byte(text)}); err != nil {
		t.Fatalf("type: %v", err)
	}
}

func TestSendWaitHoldsTheAnswerForTheFinalState(t *testing.T) {
	token, _, target := spawnPair(t, pasteCat)
	time.Sleep(1600 * time.Millisecond)
	kaiTypes(t, target, "half a thou")
	target.until("half a thou")
	go func() {
		// Past the default 3s window once the 1.5s typing hold after Enter runs out.
		time.Sleep(2500 * time.Millisecond)
		_ = target.c.write(frame{Type: "input", Session: "scientist-evie", Data: []byte("\r")})
	}()
	started := time.Now()
	state := sendWaiting(t, token, "scientist", "ping", 10)
	if state.State != "delivered" {
		t.Fatalf("state = %+v, want the final state, not the held one it began in", state)
	}
	if took := time.Since(started); took < 3100*time.Millisecond {
		t.Fatalf("the answer came in %v, before Kai's draft cleared, so it was not held for the final state", took)
	}
}

func TestAHeldMessageEarnsTheSenderAReceiptWhenItLands(t *testing.T) {
	token, sender, target := spawnPair(t, pasteCat)
	time.Sleep(1600 * time.Millisecond)
	kaiTypes(t, target, "half a thou")
	target.until("half a thou")
	state := sendAs(t, token, "scientist", "ping the secret body")
	if state.State != "held" {
		t.Fatalf("state = %+v, want held behind Kai's draft", state)
	}
	kaiTypes(t, target, "\r")
	// The receipt is typed into the sender, stamped by the daemon.
	sender.until("[from aterm daemon] message " + state.ID + " to scientist-evie: delivered")
	if strings.Contains(sender.output.String(), "secret body") {
		t.Fatalf("a receipt must never carry the body:\n%q", sender.output.String())
	}
}

func TestAMessageDeliveredAtOnceEarnsNoReceipt(t *testing.T) {
	token, sender, target := spawnPair(t, pasteCat)
	time.Sleep(1800 * time.Millisecond)
	if state := sendAs(t, token, "scientist", "ping"); state.State != "delivered" {
		t.Fatalf("state = %+v, want delivered at once", state)
	}
	target.until("[from eng-platform Beetle-Ox] ping")
	sender.drain(2500 * time.Millisecond)
	if strings.Contains(sender.output.String(), "[from aterm daemon]") {
		t.Fatalf("a message already final when answered owes no receipt:\n%q", sender.output.String())
	}
}

func TestATargetThatEndsBeforeDeliveryEarnsAFailedReceipt(t *testing.T) {
	token, sender, target := spawnPair(t, `printf 'READY\033[?2004h\n'; read -r _`)
	kaiTypes(t, target, "x")
	state := sendAs(t, token, "scientist", "ping")
	if finalState(state.State) {
		t.Fatalf("state = %+v, want it still waiting when answered", state)
	}
	kaiTypes(t, target, "\r") // ends the target's read, and so the target
	sender.until("[from aterm daemon] message " + state.ID + " to scientist-evie: failed: scientist-evie ended before it was delivered")
}

func TestReceiptTextNamesTheMessageAndNeverItsBody(t *testing.T) {
	got := receiptText(peerMessage{ID: "ab12", Target: "scientist", Session: "scientist-evie", State: "failed", Reason: "scientist-evie ended before it was delivered"})
	if want := "message ab12 to scientist-evie: failed: scientist-evie ended before it was delivered"; got != want {
		t.Fatalf("receipt = %q, want %q", got, want)
	}
	if got := receiptText(peerMessage{ID: "cd34", Target: "scientist", State: "delivered"}); got != "message cd34 to scientist: delivered" {
		t.Fatalf("receipt = %q", got)
	}
}
