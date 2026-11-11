package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func publishTo(t *testing.T, primary, followed string, newer time.Time) {
	t.Helper()
	raw, err := os.ReadFile(primary)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	value, err := publishedPasskeys(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(followed, value, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(followed, newer, newer); err != nil {
		t.Fatal(err)
	}
}

func assertOn(t *testing.T, store *passkeyStore, key *softKey) error {
	t.Helper()
	assertion, session, err := store.beginAssert()
	if err != nil {
		return err
	}
	options, _ := json.Marshal(assertion)
	return store.finishAssert(*session, key.assert(options))
}

func TestAReplicaUnlocksWithAPasskeyEnrolledOnThePrimaryAndFollowsItsRevoke(t *testing.T) {
	dir := t.TempDir()
	primaryPath, followed := filepath.Join(dir, "primary", "passkeys.json"), filepath.Join(dir, "followed.json")
	primary, err := newPasskeyStore(primaryPath, passkeyRPID, []string{passkeyOrigin})
	if err != nil {
		t.Fatal(err)
	}
	key := newSoftKey(t)
	creation, session, _ := primary.beginEnroll()
	options, _ := json.Marshal(creation)
	if err := primary.finishEnroll(*session, key.register(options)); err != nil {
		t.Fatal(err)
	}
	if err := primary.addDeviceKey(deviceKey{KeyID: "phone", PublicKey: []byte{1, 2, 3}, Level: "strongbox"}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	publishTo(t, primaryPath, followed, start)

	replica, err := newFollowingStore(followed, passkeyRPID, []string{passkeyOrigin})
	if err != nil {
		t.Fatal(err)
	}
	if !replica.enrolled() || !replica.deviceKeyEnrolled() {
		t.Fatal("the replica should hold what the primary published")
	}
	if _, found := replica.findDeviceKey("phone"); !found {
		t.Fatal("the replica should know the primary's device key")
	}
	before, _ := os.ReadFile(followed)
	if err := assertOn(t, replica, key); err != nil {
		t.Fatalf("the passkey enrolled on the primary should unlock the replica: %v", err)
	}
	if after, _ := os.ReadFile(followed); !bytes.Equal(before, after) {
		t.Fatal("a replica must never write the file it follows")
	}

	if _, _, err := replica.mintCode(); !errors.Is(err, errFollowing) {
		t.Fatalf("mint on a replica: %v", err)
	}
	if err := replica.revoke(); !errors.Is(err, errFollowing) {
		t.Fatalf("revoke on a replica: %v", err)
	}
	if _, _, err := replica.beginEnroll(); !errors.Is(err, errFollowing) {
		t.Fatalf("enroll on a replica: %v", err)
	}
	if err := replica.addDeviceKey(deviceKey{KeyID: "other"}); !errors.Is(err, errFollowing) {
		t.Fatalf("a device key added on a replica: %v", err)
	}

	if err := os.WriteFile(followed, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(followed, start.Add(time.Second), start.Add(time.Second))
	if !replica.enrolled() {
		t.Fatal("a file that will not parse must keep the passkeys already held")
	}

	if err := primary.revoke(); err != nil {
		t.Fatal(err)
	}
	publishTo(t, primaryPath, followed, start.Add(2*time.Second))
	if replica.enrolled() || replica.deviceKeyEnrolled() {
		t.Fatal("a published revoke must reach the replica")
	}
	if err := assertOn(t, replica, key); err == nil {
		t.Fatal("a revoked passkey must not unlock the replica")
	}

	if err := os.Remove(followed); err != nil {
		t.Fatal(err)
	}
	if replica.enrolled() {
		t.Fatal("a file that is gone publishes nothing")
	}
}

func TestPublishedPasskeysDropAttestationAndKeepWhatALoginNeeds(t *testing.T) {
	user := passkeyUser{ID: []byte("user-handle"), Credentials: []webauthn.Credential{{
		ID: []byte("cred"), PublicKey: []byte("pub"),
		Attestation: webauthn.CredentialAttestation{ClientDataJSON: []byte("big"), Object: []byte("bigger")},
	}}, DeviceKeys: []deviceKey{{KeyID: "phone", PublicKey: []byte{9}, Level: "strongbox"}}}
	raw, _ := json.Marshal(&user)
	published, err := publishedPasskeys(raw)
	if err != nil {
		t.Fatal(err)
	}
	var got passkeyUser
	if err := json.Unmarshal(published, &got); err != nil {
		t.Fatal(err)
	}
	credential := got.Credentials[0]
	if len(credential.Attestation.ClientDataJSON) != 0 || len(credential.Attestation.Object) != 0 {
		t.Fatalf("attestation should be dropped: %+v", credential.Attestation)
	}
	if string(credential.ID) != "cred" || string(credential.PublicKey) != "pub" || string(got.ID) != "user-handle" || got.DeviceKeys[0].KeyID != "phone" {
		t.Fatalf("what a login needs must survive: %+v", got)
	}
	if empty, err := publishedPasskeys(nil); err != nil || len(empty) == 0 {
		t.Fatalf("an empty store publishes a revoke: %q %v", empty, err)
	}
}

// COI-3542 at the protocol: enroll on one daemon, then unlock another with the passkey.
func TestAPasskeyEnrolledOnOneDaemonUnlocksAReplicaOverTheProtocol(t *testing.T) {
	dir := t.TempDir()
	primaryPath, followed := filepath.Join(dir, "passkeys.json"), filepath.Join(dir, "followed.json")
	primary, _ := wsDaemon(t)
	primary.local().passkeys, _ = newPasskeyStore(primaryPath, passkeyRPID, []string{passkeyOrigin})
	wsSession(t, primary, false)
	remote := peerStanding{web: true, remote: true}
	key := newSoftKey(t)

	code := mintCode(t, primary)
	enrolling := pipeClient(t, primary, remote)
	nextFrame(t, enrolling, "welcome")
	begin, err := enrolling.request(frame{Type: "passkey_enroll_begin", EnrollCode: code})
	if err != nil {
		t.Fatal(err)
	}
	if done, err := enrolling.request(frame{Type: "passkey_enroll_finish", Credential: key.register(begin.Options)}); err != nil || done.Type != "passkey_enrolled" {
		t.Fatalf("enroll on the primary: %+v, %v", done, err)
	}
	publishTo(t, primaryPath, followed, time.Now())

	replica, _ := wsDaemon(t)
	replica.local().passkeys, _ = newFollowingStore(followed, passkeyRPID, []string{passkeyOrigin})
	wsSession(t, replica, false)

	device := pipeClient(t, replica, remote)
	if welcome := nextFrame(t, device, "welcome"); welcome.Typing.Passkey != "enrolled" || welcome.Typing.Allowed {
		t.Fatalf("the replica should know the passkey and start locked: %+v", welcome.Typing)
	}
	_, _ = device.request(frame{Type: "attach", Session: "scientist-evie", Rows: 24, Cols: 80})
	options, err := device.request(frame{Type: "passkey_assert_begin"})
	if err != nil {
		t.Fatal(err)
	}
	if reply, err := device.request(frame{Type: "passkey_assert_finish", Credential: key.assert(options.Options)}); err != nil || reply.Type != "passkey_asserted" {
		t.Fatalf("the passkey enrolled on the primary should unlock the replica: %+v, %v", reply, err)
	}
	if standing := nextFrame(t, device, "typing"); !standing.Typing.Allowed {
		t.Fatalf("asserting should unlock the replica: %+v", standing.Typing)
	}

	terminal := pipeClient(t, replica, peerStanding{pids: []int{os.Getpid()}})
	nextFrame(t, terminal, "welcome")
	if reply, err := terminal.request(frame{Type: "passkey_mint"}); err == nil || reply.Type == "passkey_minted" {
		t.Fatalf("a replica must not mint a code: %+v, %v", reply, err)
	}
}
