package main

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	resumeFeature   = "resume"
	resumeWindowEnv = "ATERM_RESUME_WINDOW"
	// reasonResumeEnded is every way a proof can fail (expired, spent, unknown, a
	// restart, a revoke), so a probing client learns nothing from which one.
	reasonResumeEnded = "resume_ended"
)

// resumeUntilCloseWord is the --resume-window default, which never expires a proof.
const resumeUntilCloseWord = "app"

// parseResumeWindow reads --resume-window: app, 0 or a positive duration.
func parseResumeWindow(value string) (window time.Duration, untilClose bool, err error) {
	if strings.EqualFold(strings.TrimSpace(value), resumeUntilCloseWord) {
		return 0, true, nil
	}
	if window, err = time.ParseDuration(strings.TrimSpace(value)); err != nil {
		return 0, false, fmt.Errorf("--resume-window is %s, 0 or a duration: %w", resumeUntilCloseWord, err)
	}
	if window < 0 {
		return 0, false, errors.New("--resume-window cannot be negative")
	}
	return window, false, nil
}

// resumeMaxGrants bounds the proofs kept when none expires, oldest closed first.
const resumeMaxGrants = 64

// resumeStore carries one unlock across a reconnect. No window and no untilClose is off.
// In memory only. Design: references/resume.md in tooling-aterm-client.
type resumeStore struct {
	mu     sync.Mutex
	window time.Duration
	// untilClose keeps a proof until it is spent, revoked or the daemon restarts, since the
	// client holds it in page memory and closing the app drops it (Kai, COI-3305).
	untilClose bool
	now        func() time.Time
	grants     map[[sha256.Size]byte]*resumeGrant
}

// resumeGrant is held by hash, so the daemon never keeps a usable token.
type resumeGrant struct {
	open    bool
	dropped time.Time
}

func newResumeStore() *resumeStore {
	return &resumeStore{now: time.Now, grants: map[[sha256.Size]byte]*resumeGrant{}}
}

func (r *resumeStore) enabled() bool { return r.untilClose || r.window > 0 }

// expiresIn is the window a reply reports, zero (omitted) when proofs do not expire.
func (r *resumeStore) expiresIn() int {
	if r.untilClose {
		return 0
	}
	return int(r.window.Seconds())
}

// pruneLocked forgets grants whose window has ended. The caller holds mu.
func (r *resumeStore) pruneLocked() {
	now := r.now()
	for key, grant := range r.grants {
		if !r.untilClose && !grant.open && !now.Before(grant.dropped.Add(r.window)) {
			delete(r.grants, key)
		}
	}
	for len(r.grants) > resumeMaxGrants {
		var oldest [sha256.Size]byte
		found := false
		for key, grant := range r.grants {
			if !grant.open && (!found || grant.dropped.Before(r.grants[oldest].dropped)) {
				oldest, found = key, true
			}
		}
		if !found {
			return
		}
		delete(r.grants, oldest)
	}
}

// issue gives cl a fresh token, ending any it held, and returns it. Empty when the
// daemon runs with no window.
func (r *resumeStore) issue(cl *client) (string, error) {
	if !r.enabled() {
		return "", nil
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := rawURL.EncodeToString(raw)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	if cl.resumeHeld {
		delete(r.grants, cl.resumeKey)
	}
	cl.resumeKey, cl.resumeHeld = sha256.Sum256([]byte(token)), true
	r.grants[cl.resumeKey] = &resumeGrant{open: true}
	return token, nil
}

// redeem spends token for cl and returns the replacement. It fails for a token that
// is unknown, spent, or past its window.
func (r *resumeStore) redeem(cl *client, token string) (string, error) {
	if !r.enabled() {
		return "", errors.New("resume is not on")
	}
	key := sha256.Sum256([]byte(token))
	r.mu.Lock()
	r.pruneLocked()
	_, live := r.grants[key]
	delete(r.grants, key)
	r.mu.Unlock()
	if !live {
		return "", errors.New("no live resume proof")
	}
	return r.issue(cl)
}

// drop starts cl's window when its connection ends.
func (r *resumeStore) drop(cl *client) {
	if !cl.resumeHeld {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if grant, ok := r.grants[cl.resumeKey]; ok {
		grant.open, grant.dropped = false, r.now()
	}
}

// clear ends every grant, which a passkey revoke does along with the keys.
func (r *resumeStore) clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.grants)
}

// issueResume stamps the token onto an unlock reply, which carries it only when
// the daemon runs with a window.
func (u *localUsers) issueResume(cl *client, reply frame) (frame, error) {
	token, err := u.resume.issue(cl)
	if err != nil {
		return reply, err
	}
	if token != "" {
		reply.Resume, reply.ExpiresIn = token, u.resume.expiresIn()
	}
	return reply, nil
}

// resumeFrame answers a redialed phone's proof. A refusal leaves the connection
// as locked as it arrived, so the typing and locked-frame gate is untouched.
func (u *localUsers) resumeFrame(cl *client, message frame) error {
	if !cl.web || !cl.remote {
		return withExit(exitUsage, errors.New("a resume proof belongs to a remote device"))
	}
	if cl.asserted {
		return withExit(exitUsage, errors.New("this connection is already unlocked"))
	}
	token, err := u.resume.redeem(cl, message.Resume)
	if err != nil {
		return withReason(reasonResumeEnded, errors.New("the unlock window ended, unlock this device again"))
	}
	cl.asserted = true
	u.host.logf("a remote device resumed its unlock")
	if err := cl.c.write(frame{Type: "resumed", ID: message.ID, Resume: token, ExpiresIn: u.resume.expiresIn()}); err != nil {
		return err
	}
	return cl.c.write(frame{Type: "typing", Typing: u.host.typingStanding(cl)})
}
