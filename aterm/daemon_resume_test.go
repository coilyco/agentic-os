package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const testResumeWindow = daemonResumeWindow

// resumeClock is the resume store's clock, so a window passes without waiting.
type resumeClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *resumeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *resumeClock) advance(by time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(by)
}

// resumeDaemon is a daemon with a seat to type into, a passkey store, and a
// controllable resume window (zero for none).
func resumeDaemon(t *testing.T, window time.Duration) (*daemon, *resumeClock) {
	t.Helper()
	d, _ := wsDaemon(t)
	var err error
	if d.local().passkeys, err = newPasskeyStore(filepath.Join(t.TempDir(), "passkeys.json"), passkeyRPID, []string{passkeyOrigin}); err != nil {
		t.Fatal(err)
	}
	clock := &resumeClock{at: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)}
	d.local().resume.window, d.local().resume.now = window, clock.now
	wsSession(t, d, false)
	return d, clock
}

var remotePhone = peerStanding{web: true, remote: true}

// unlock enrolls key on a new remote connection, which unlocks it, and returns the
// connection with its welcome and the enrollment reply.
func unlock(t *testing.T, d *daemon, key *softKey) (*conn, frame, frame) {
	t.Helper()
	device := pipeClient(t, d, remotePhone)
	welcome := nextFrame(t, device, "welcome")
	begin, err := device.request(frame{Type: "passkey_enroll_begin", EnrollCode: mintCode(t, d)})
	if err != nil {
		t.Fatal(err)
	}
	done, err := device.request(frame{Type: "passkey_enroll_finish", Credential: key.register(begin.Options)})
	if err != nil || done.Type != "passkey_enrolled" {
		t.Fatalf("enroll finish: %+v, %v", done, err)
	}
	if standing := nextFrame(t, device, "typing"); !standing.Typing.Allowed {
		t.Fatalf("enrolling should unlock: %+v", standing.Typing)
	}
	return device, welcome, done
}

// dropAndWait closes the phone's connection and waits until the daemon has seen it,
// since the window runs from that moment.
func dropAndWait(t *testing.T, d *daemon, device *conn) {
	t.Helper()
	_ = device.Close()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		d.local().resume.mu.Lock()
		open := 0
		for _, grant := range d.local().resume.grants {
			if grant.open {
				open++
			}
		}
		d.local().resume.mu.Unlock()
		if open == 0 {
			return
		}
	}
	t.Fatal("the daemon never saw the connection drop")
}

// redial opens a new remote connection and returns it with the standing its welcome gave.
func redial(t *testing.T, d *daemon) (*conn, *typingStanding) {
	t.Helper()
	device := pipeClient(t, d, remotePhone)
	return device, nextFrame(t, device, "welcome").Typing
}

// stillLocked asserts locked and typing frames are refused as passkey_required.
func stillLocked(t *testing.T, device *conn) {
	t.Helper()
	// input checks attachment before the typing gate, and attach is open to a locked device.
	if _, err := device.request(frame{Type: "attach", Session: "scientist-evie", Rows: 24, Cols: 80}); err != nil {
		t.Fatal(err)
	}
	for _, refused := range []frame{
		{Type: "input", Session: "scientist-evie", Data: []byte("x")},
		{Type: "launch", Role: "scientist"},
		{Type: "spawn", Kind: kindTerminal},
	} {
		reply, err := device.request(refused)
		if err == nil || reply.Reason != reasonPasskeyRequired {
			t.Fatalf("%s should be refused as %s: %+v, %v", refused.Type, reasonPasskeyRequired, reply, err)
		}
	}
}

func screenHolds(d *daemon, want string) bool {
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if strings.Contains(strings.Join(d.session("scientist-evie").status(24).Screen, "\n"), want) {
			return true
		}
	}
	return false
}

func TestRedialInsideTheResumeWindowArrivesUnlockedAndAfterItLocked(t *testing.T) {
	d, clock := resumeDaemon(t, testResumeWindow)
	key := newSoftKey(t)

	first, welcome, done := unlock(t, d, key)
	if !slices.Contains(welcome.Features, resumeFeature) {
		t.Fatalf("a daemon with a window offers %s: %v", resumeFeature, welcome.Features)
	}
	if done.Resume == "" || done.ExpiresIn != int(testResumeWindow.Seconds()) {
		t.Fatalf("an unlock hands out a proof and its window: %+v", done)
	}
	dropAndWait(t, d, first)
	clock.advance(testResumeWindow - time.Second)

	inside, standing := redial(t, d)
	if standing.Allowed || standing.Reason != reasonPasskeyRequired {
		t.Fatalf("a redial starts locked until it presents a proof: %+v", standing)
	}
	stillLocked(t, inside)
	resumed, err := inside.request(frame{Type: "resume", Resume: done.Resume})
	if err != nil || resumed.Type != "resumed" || resumed.Resume == "" || resumed.Resume == done.Resume {
		t.Fatalf("a live proof resumes and is replaced: %+v, %v", resumed, err)
	}
	if standing := nextFrame(t, inside, "typing").Typing; !standing.Allowed {
		t.Fatalf("resuming should unlock typing: %+v", standing)
	}
	if err := inside.write(frame{Type: "input", Session: "scientist-evie", Data: []byte("typed-after-resume\r")}); err != nil {
		t.Fatal(err)
	}
	if !screenHolds(d, "typed-after-resume") {
		t.Fatal("the resumed connection's input never reached the session")
	}

	// A second drop and a redial past the window, which is measured from that drop.
	dropAndWait(t, d, inside)
	clock.advance(testResumeWindow)
	late, standing := redial(t, d)
	if standing.Allowed {
		t.Fatalf("a redial after the window starts locked: %+v", standing)
	}
	reply, err := late.request(frame{Type: "resume", Resume: resumed.Resume})
	if err == nil || reply.Reason != reasonResumeEnded {
		t.Fatalf("a proof past its window reads as %+v, %v, want %s", reply, err, reasonResumeEnded)
	}
	stillLocked(t, late)
}

func TestAProofIsSingleUseAndEachResumeStartsAFreshWindow(t *testing.T) {
	d, clock := resumeDaemon(t, testResumeWindow)
	first, _, done := unlock(t, d, newSoftKey(t))
	dropAndWait(t, d, first)

	clock.advance(testResumeWindow - time.Second)
	second, _ := redial(t, d)
	resumed, err := second.request(frame{Type: "resume", Resume: done.Resume})
	if err != nil {
		t.Fatalf("first resume: %v", err)
	}
	nextFrame(t, second, "typing")
	dropAndWait(t, d, second)

	spent, _ := redial(t, d)
	if reply, err := spent.request(frame{Type: "resume", Resume: done.Resume}); err == nil || reply.Reason != reasonResumeEnded {
		t.Fatalf("a spent proof must not resume: %+v, %v", reply, err)
	}
	stillLocked(t, spent)

	// Longer than one window since the unlock, shorter than one since the last drop.
	clock.advance(testResumeWindow - time.Second)
	third, _ := redial(t, d)
	if reply, err := third.request(frame{Type: "resume", Resume: resumed.Resume}); err != nil || reply.Type != "resumed" {
		t.Fatalf("a resume renews the window: %+v, %v", reply, err)
	}
}

func TestAnOpenConnectionNeverExpiresItsProofAndItsDropEndsNothingResumed(t *testing.T) {
	d, clock := resumeDaemon(t, testResumeWindow)
	stale, _, done := unlock(t, d, newSoftKey(t))

	// The phone redials before the daemon notices the old socket is dead.
	clock.advance(3 * testResumeWindow)
	fresh, _ := redial(t, d)
	resumed, err := fresh.request(frame{Type: "resume", Resume: done.Resume})
	if err != nil || resumed.Type != "resumed" {
		t.Fatalf("a proof whose connection is still open resumes: %+v, %v", resumed, err)
	}
	nextFrame(t, fresh, "typing")

	// The stale socket closing later must not take the new proof down with it.
	_ = stale.Close()
	time.Sleep(100 * time.Millisecond)
	next, _ := redial(t, d)
	if reply, err := next.request(frame{Type: "resume", Resume: resumed.Resume}); err != nil || reply.Type != "resumed" {
		t.Fatalf("the replacement proof outlives the old connection: %+v, %v", reply, err)
	}
}

func TestResumeIsOffWithoutAWindow(t *testing.T) {
	d, _ := resumeDaemon(t, 0)
	first, welcome, done := unlock(t, d, newSoftKey(t))
	if slices.Contains(welcome.Features, resumeFeature) || done.Resume != "" || done.ExpiresIn != 0 {
		t.Fatalf("no window means no feature and no proof: %v %+v", welcome.Features, done)
	}
	dropAndWait(t, d, first)
	second, standing := redial(t, d)
	if standing.Allowed {
		t.Fatalf("an unlock stays on its connection: %+v", standing)
	}
	if reply, err := second.request(frame{Type: "resume", Resume: "anything"}); err == nil || reply.Reason != reasonResumeEnded {
		t.Fatalf("resume without a window: %+v, %v", reply, err)
	}
	stillLocked(t, second)
}

func TestGuessedProofsAndPasskeyRevokeEndEveryResume(t *testing.T) {
	d, _ := resumeDaemon(t, testResumeWindow)
	first, _, done := unlock(t, d, newSoftKey(t))
	dropAndWait(t, d, first)

	guess, _ := redial(t, d)
	for _, token := range []string{"", "not-a-proof", strings.Repeat("A", 43)} {
		if reply, err := guess.request(frame{Type: "resume", Resume: token}); err == nil || reply.Reason != reasonResumeEnded {
			t.Fatalf("proof %q read as %+v, %v", token, reply, err)
		}
	}
	stillLocked(t, guess)

	terminal := pipeClient(t, d, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(t, terminal, "welcome")
	if reply, err := terminal.request(frame{Type: "passkey_revoke"}); err != nil || reply.Type != "passkey_revoked" {
		t.Fatalf("revoke: %+v, %v", reply, err)
	}
	if reply, err := guess.request(frame{Type: "resume", Resume: done.Resume}); err == nil || reply.Reason != reasonResumeEnded {
		t.Fatalf("a revoke must end the proof: %+v, %v", reply, err)
	}
	stillLocked(t, guess)
}

func TestOnlyARemoteDeviceCanResume(t *testing.T) {
	d, _ := resumeDaemon(t, testResumeWindow)
	_, _, done := unlock(t, d, newSoftKey(t))
	local := pipeClient(t, d, peerStanding{web: true})
	nextFrame(t, local, "welcome")
	if _, err := local.request(frame{Type: "resume", Resume: done.Resume}); err == nil {
		t.Fatal("a device that is not remote has no unlock to resume")
	}
}

func TestDeviceKeyAssertionIssuesAProofToo(t *testing.T) {
	d, _ := deviceDaemon(t, newPhone(t, attest{}))
	d.local().resume.window = testResumeWindow
	device := pipeClient(t, d, remotePhone)
	nextFrame(t, device, "welcome")
	p, keyID := enroll(t, d, device, attest{})
	nextFrame(t, device, "typing")
	begin, err := device.request(frame{Type: "device_assert_begin", KeyID: keyID})
	if err != nil {
		t.Fatal(err)
	}
	challenge, _ := rawURL.DecodeString(begin.Challenge)
	asserted, err := device.request(frame{Type: "device_assert_finish", KeyID: keyID, Signature: p.signAssertion(t, challenge)})
	if err != nil || asserted.Resume == "" || asserted.ExpiresIn != int(testResumeWindow.Seconds()) {
		t.Fatalf("a device key assertion hands out the proof: %+v, %v", asserted, err)
	}
	nextFrame(t, device, "typing")
	dropAndWait(t, d, device)

	back, standing := redial(t, d)
	if standing.Allowed {
		t.Fatalf("a redial starts locked: %+v", standing)
	}
	if reply, err := back.request(frame{Type: "resume", Resume: asserted.Resume}); err != nil || reply.Type != "resumed" {
		t.Fatalf("the device key's proof resumes: %+v, %v", reply, err)
	}
}
