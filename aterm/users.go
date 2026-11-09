package main

import (
	"errors"
	"time"
)

// userManagement is the one seam to everything that says who a person is (COI-3358).
// The typing gate stays in the core and judges only Unlocked, so a swap cannot widen it.
type userManagement interface {
	// ProxiedPolicy admits a request a Serve front proxied to the loopback
	// listener. The zero accessPolicy refuses every one.
	ProxiedPolicy() accessPolicy
	// TailnetPolicy admits a peer of the direct tailnet listener on node.
	TailnetPolicy(node tailnetNode) accessPolicy
	// Admits names who TailnetPolicy lets in, for the listener's startup log line.
	Admits(node tailnetNode) string

	// Unlocked is true when cl has proved it is the owner on this connection.
	Unlocked(cl *client) bool
	// Standing is whether a passkey and a device key are enrolled, for the
	// welcome and the typing frame.
	Standing(cl *client) (passkey, deviceKey string)
	// Challenge is the unlock offer a brokered channel's welcome carries, nil for any
	// other connection.
	Challenge(cl *client) *unlockOffer
	// Handles is true for the frames this seam answers, and Frame answers one.
	Handles(frameType string) bool
	Frame(cl *client, message frame) error
	// Disconnected is told when cl's connection ends.
	Disconnected(cl *client)

	// ResumeEnabled and PushEnabled are the welcome's optional features.
	ResumeEnabled() bool
	PushEnabled() bool
	// NotifyAsk wakes subscribed devices for a seat's ask. WatchWaiting does the
	// same for a seat that starts waiting, until done closes.
	NotifyAsk(ask choiceAsk)
	// NotifyPush wakes subscribed devices with a coordinator's notify.
	NotifyPush(payload pushPayload)
	WatchWaiting(done <-chan struct{}, every time.Duration)
}

// userHost is what user management needs back from the daemon, as functions
// because tests replace the daemon's logf after it is built.
type userHost struct {
	logf           func(string, ...any)
	guard          func(where string)
	views          func() []sessionView
	typingRefusal  func(cl *client, message string) error
	typingStanding func(cl *client) *typingStanding
	// mayPing says whether a role is a coordinator, the only kind that reaches Kai by push.
	mayPing func(role string) bool
}

func (d *daemon) userHost() userHost {
	return userHost{
		logf:           func(format string, args ...any) { d.logf(format, args...) },
		guard:          d.guard,
		views:          d.views,
		typingRefusal:  d.typingRefusal,
		typingStanding: d.typingStanding,
		mayPing:        d.mayPing,
	}
}

// admissionConfig is who the tailnet listener and a Serve front let in.
type admissionConfig struct {
	tags, owners, origins []string
	whois                 func(string) (tailnetPeer, error)
}

// localUsers is user management as the daemon does it today: stores on this
// host's disk, admission by tailscale whois.
type localUsers struct {
	host      userHost
	passkeys  *passkeyStore
	resume    *resumeStore
	push      pushState
	admission admissionConfig
	proxied   accessPolicy
	// brokered judges unlock tokens on channels the Treehouse broker opens, nil when off.
	brokered *brokeredGate
	// brokerEvent tells the broker a seat is waiting, nil when the brokered path is off.
	brokerEvent func(waitEvent)
}

// newLocalUsers is the unconfigured state a daemon starts in: no stored
// passkeys, no resume window, no push, and a Serve front refused.
func newLocalUsers(host userHost) *localUsers {
	return &localUsers{host: host, passkeys: defaultPasskeys(), resume: newResumeStore()}
}

// openLocalUsers reads the stored passkeys and push subscriptions, and applies
// the options. A store that will not load leaves its feature off, as before.
func openLocalUsers(host userHost, options daemonOptions) *localUsers {
	u := newLocalUsers(host)
	var err error
	if u.passkeys, err = newPasskeyStore(passkeyStatePath(), passkeyRPID, []string{passkeyOrigin}); err != nil {
		host.logf("no stored passkeys, so no remote device can type: %v", err)
		u.passkeys = defaultPasskeys()
	}
	u.resume.window, u.resume.untilClose = options.ResumeWindow, options.ResumeUntilClose
	switch {
	case u.resume.untilClose:
		host.logf("a remote device's unlock survives a reconnect until its app closes")
	case u.resume.enabled():
		host.logf("a remote device's unlock survives a reconnect for %s", u.resume.window)
	}
	if store, err := loadPushStore(pushStatePath()); err != nil {
		host.logf("no stored push subscriptions, so no browser is notified while closed: %v", err)
	} else if err := u.setupPush(options.VAPIDKey, store); err != nil {
		host.logf("no Web Push: %v", err)
	}
	u.admission = admissionConfig{tags: options.AllowTags, owners: options.AllowOwners, origins: options.AllowOrigins, whois: tailscaleWhois}
	u.proxied = forwardedPolicy(&nodeReader{read: readTailnetNode}, u.admission.tags, u.admission.owners, u.admission.origins, u.admission.whois)
	return u
}

func (u *localUsers) ProxiedPolicy() accessPolicy { return u.proxied }

func (u *localUsers) TailnetPolicy(node tailnetNode) accessPolicy {
	a := u.admission
	return tailnetPolicy(node, a.tags, a.owners, a.origins, a.whois)
}

func (u *localUsers) Admits(node tailnetNode) string {
	return admitsSummary(node, u.admission.tags, u.admission.owners)
}

func (u *localUsers) Unlocked(cl *client) bool { return cl.asserted }

func (u *localUsers) Standing(cl *client) (passkey, deviceKey string) {
	if cl.brokered {
		return "", ""
	}
	passkey, deviceKey = "unenrolled", "unenrolled"
	if u.passkeys.enrolled() {
		passkey = "enrolled"
	}
	if u.passkeys.deviceKeyEnrolled() {
		deviceKey = "enrolled"
	}
	return passkey, deviceKey
}

func (u *localUsers) Challenge(cl *client) *unlockOffer {
	if !cl.brokered || u.brokered == nil {
		return nil
	}
	return u.brokered.challenge(cl)
}

func (u *localUsers) Handles(frameType string) bool {
	if frameType == "unlock" {
		return true
	}
	switch frameType {
	case "passkey_mint", "passkey_revoke", "passkey_enroll_begin", "passkey_enroll_finish", "passkey_assert_begin", "passkey_assert_finish",
		"device_enroll_begin", "device_enroll_finish", "device_assert_begin", "device_assert_finish",
		"resume", "push_key", "push_subscribe", "push_unsubscribe":
		return true
	}
	return false
}

func (u *localUsers) Frame(cl *client, message frame) error {
	if message.Type == "unlock" {
		return u.brokered.unlockFrame(cl, message)
	}
	if cl.brokered {
		return withExit(exitUsage, errors.New("a brokered channel unlocks with an accounts token, not "+message.Type))
	}
	switch message.Type {
	case "device_enroll_begin", "device_enroll_finish", "device_assert_begin", "device_assert_finish":
		return u.deviceKeyFrames(cl, message)
	case "resume":
		return u.resumeFrame(cl, message)
	case "push_key", "push_subscribe", "push_unsubscribe":
		return u.pushFrame(cl, message)
	}
	return u.passkey(cl, message)
}

func (u *localUsers) Disconnected(cl *client) { u.resume.drop(cl) }

func (u *localUsers) ResumeEnabled() bool { return u.resume.enabled() }

func (u *localUsers) PushEnabled() bool { return u.push.enabled() }

func (u *localUsers) NotifyAsk(ask choiceAsk) {
	if !u.mayPing(askerRole(ask)) {
		return
	}
	if u.brokerEvent != nil {
		u.brokerEvent(waitEvent{Kind: "ask", Session: ask.Session, Identity: askerName(ask), Question: ask.Question})
	}
	u.pushAsk(ask)
}

func (u *localUsers) NotifyPush(payload pushPayload) { u.pushWaiting(payload) }

func (u *localUsers) WatchWaiting(done <-chan struct{}, every time.Duration) {
	u.watchWaiting(done, every)
}
