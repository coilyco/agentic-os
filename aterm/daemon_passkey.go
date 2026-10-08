package main

import (
	"encoding/json"
	"errors"

	"github.com/go-webauthn/webauthn/webauthn"
)

// passkeyCeremony is the one challenge a connection has open, spent by the
// first finish it sees, right or wrong.
type passkeyCeremony struct {
	kind string
	data webauthn.SessionData
}

// passkey answers the frames that mint, revoke, enroll and assert a passkey. Mint and
// revoke are Kai's terminal, outside every session. The ceremonies are the browser's.
func (d *daemon) passkey(cl *client, message frame) error {
	switch message.Type {
	case "passkey_mint", "passkey_revoke":
		return d.passkeyAdmin(cl, message)
	}
	if !cl.web {
		return withExit(exitUsage, errors.New("a passkey is enrolled and asserted from a browser"))
	}
	store := d.passkeys
	switch message.Type {
	case "passkey_enroll_begin":
		if err := store.spendCode(message.EnrollCode); err != nil {
			return withExit(exitUsage, err)
		}
		creation, session, err := store.beginEnroll()
		if err != nil {
			return err
		}
		return d.openCeremony(cl, message, "enroll", "passkey_enroll_options", creation, session)
	case "passkey_assert_begin":
		assertion, session, err := store.beginAssert()
		if err != nil {
			return withExit(exitUsage, err)
		}
		return d.openCeremony(cl, message, "assert", "passkey_assert_options", assertion, session)
	}
	ceremony := cl.ceremony
	cl.ceremony = nil
	want, reply := "enroll", "passkey_enrolled"
	if message.Type == "passkey_assert_finish" {
		want, reply = "assert", "passkey_asserted"
	}
	if ceremony == nil || ceremony.kind != want {
		return withExit(exitUsage, errors.New("no "+want+" ceremony is open on this connection"))
	}
	var err error
	if want == "enroll" {
		err = store.finishEnroll(ceremony.data, message.Credential)
	} else {
		err = store.finishAssert(ceremony.data, message.Credential)
	}
	if err != nil {
		return withExit(exitUsage, err)
	}
	// An enrollment carried user verification too, so it needs no second prompt.
	cl.asserted = true
	d.logf("a remote device asserted its passkey (%s)", want)
	if err := cl.c.write(frame{Type: reply, ID: message.ID}); err != nil {
		return err
	}
	return cl.c.write(frame{Type: "typing", Typing: d.typingStanding(cl)})
}

func (d *daemon) openCeremony(cl *client, message frame, kind, reply string, options any, session *webauthn.SessionData) error {
	encoded, err := json.Marshal(options)
	if err != nil {
		return err
	}
	cl.ceremony = &passkeyCeremony{kind: kind, data: *session}
	return cl.c.write(frame{Type: reply, ID: message.ID, Options: encoded})
}

func (d *daemon) passkeyAdmin(cl *client, message frame) error {
	if cl.web {
		return withExit(exitUsage, errors.New("a passkey is minted and revoked from a terminal, not a browser"))
	}
	if err := d.typingRefusal(cl, "a process inside an aterm session cannot mint or revoke a passkey, use a terminal of your own"); err != nil {
		return err
	}
	if message.Type == "passkey_revoke" {
		if err := d.passkeys.revoke(); err != nil {
			return err
		}
		d.logf("every passkey was revoked")
		return cl.c.write(frame{Type: "passkey_revoked", ID: message.ID})
	}
	code, ttl, err := d.passkeys.mintCode()
	if err != nil {
		return err
	}
	d.logf("an enrollment code was issued")
	return cl.c.write(frame{Type: "passkey_minted", ID: message.ID, EnrollCode: code[:4] + "-" + code[4:], ExpiresIn: int(ttl.Seconds())})
}
