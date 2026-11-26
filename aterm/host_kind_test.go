package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHostKindReadsPersonalUnlessTheFileNamesAKind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, hostKindFile)
	for name, content := range map[string]string{
		"missing":      "",
		"not json":     "durable",
		"wrong format": `{"format":"aterm.host.v0","kind":"durable"}`,
		"unknown kind": `{"format":"aterm.host.v1","kind":"server"}`,
	} {
		_ = os.Remove(path)
		if content != "" {
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		var store hostKindStore
		err := store.load(path)
		if store.get() != hostPersonal {
			t.Errorf("%s: read as %s, want personal", name, store.get())
		}
		if (err == nil) != (name == "missing") {
			t.Errorf("%s: load error %v", name, err)
		}
	}
	var store hostKindStore
	if store.get() != hostPersonal {
		t.Fatalf("a store never loaded is %s", store.get())
	}
}

// onHostKind is the accessor keep-awake (COI-3768) builds on: once at registration, again
// on each change, never on a set that changes nothing, and with no daemon lock held.
func TestOnHostKindReportsTheKindNowAndOnEachChange(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	d.host.path = filepath.Join(t.TempDir(), hostKindFile)
	var seen []hostKind
	d.onHostKind(func(kind hostKind) {
		// Taking d.mu here would deadlock if setHostKind held it.
		d.mu.Lock()
		d.mu.Unlock()
		seen = append(seen, kind)
	})
	for _, kind := range []hostKind{hostPersonal, hostDurable, hostDurable, hostEphemeral} {
		if err := d.setHostKind(kind); err != nil {
			t.Fatal(err)
		}
	}
	if want := []hostKind{hostPersonal, hostDurable, hostEphemeral}; !slicesEqual(seen, want) {
		t.Fatalf("watcher saw %v, want %v", seen, want)
	}
	var reloaded hostKindStore
	if err := reloaded.load(d.host.path); err != nil || reloaded.get() != hostEphemeral {
		t.Fatalf("the file holds %s, %v", reloaded.get(), err)
	}
}

func slicesEqual(a, b []hostKind) bool {
	return len(a) == len(b) && strings.Join(kindStrings(a), ",") == strings.Join(kindStrings(b), ",")
}

func kindStrings(kinds []hostKind) []string {
	out := make([]string, len(kinds))
	for i, kind := range kinds {
		out[i] = string(kind)
	}
	return out
}

func runHostKind(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	command := newHostCommand()
	command.Writer = &out
	err := command.Run(context.Background(), append([]string{"host", "kind"}, args...))
	return strings.TrimSpace(out.String()), err
}

// Acceptance (COI-3767): `aterm host kind durable` sets the kind, the welcome reports it,
// and it survives a daemon restart.
func TestAtermHostKindSetsTheKindAndItSurvivesARestart(t *testing.T) {
	t.Setenv(stateDirEnv, filepath.Join(t.TempDir(), "sessions"))
	t.Run("set", func(t *testing.T) {
		testDaemon(t)
		if got, err := runHostKind(t); err != nil || got != "personal" {
			t.Fatalf("a fresh host should be personal: %q, %v", got, err)
		}
		watcher := dialTest(t).c
		if _, err := watcher.request(frame{Type: "subscribe", Channel: "sessions"}); err != nil {
			t.Fatal(err)
		}
		if got, err := runHostKind(t, "durable"); err != nil || got != "durable" {
			t.Fatalf("set: %q, %v", got, err)
		}
		if pushed := nextFrame(t, watcher, "host_kind"); pushed.HostKind != "durable" || pushed.ID != "" {
			t.Fatalf("a subscriber should hear the change: %+v", pushed)
		}
		if _, err := runHostKind(t, "server"); err == nil || !strings.Contains(err.Error(), "personal, ephemeral or durable") {
			t.Fatalf("an unknown kind should be refused: %v", err)
		}
	})
	t.Run("after a restart", func(t *testing.T) {
		testDaemon(t)
		c, err := dialDaemon(false)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		if !strings.Contains(strings.Join(c.features, ","), hostKindFeature) {
			t.Fatalf("the welcome should name %s: %v", hostKindFeature, c.features)
		}
		if got, err := runHostKind(t); err != nil || got != "durable" {
			t.Fatalf("the kind should outlive the daemon: %q, %v", got, err)
		}
	})
}

// A locked brokered channel reads the kind from its welcome and cannot change it, an
// unlocked one can, and the broker hears each change because its ready listed host-kind.
func TestABrokeredClientChangesTheKindOnlyOnceUnlockedAndTheBrokerHearsIt(t *testing.T) {
	r := prepareBrokeredRig(t, time.Hour)
	r.b.features = []string{"host-kind"}
	r.start()
	if got := nextHost(t, r.b); got.Kind != "personal" {
		t.Fatalf("the broker should hear the kind after ready: %+v", got)
	}
	c, welcome := r.openLocked(t, "c1")
	if welcome.HostKind != "personal" {
		t.Fatalf("the welcome carries host_kind: %+v", welcome)
	}
	if reply, err := c.request(frame{Type: "host_kind", HostKind: "durable"}); err == nil || reply.Reason != reasonPasskeyRequired {
		t.Fatalf("a locked device should be refused: %+v, %v", reply, err)
	}
	if r.d.hostKind() != hostPersonal {
		t.Fatalf("a refused change landed: %s", r.d.hostKind())
	}
	token := r.a.mint(t, unlockTokenType, r.a.unlockClaims(testDaemonID, testOwner, welcome.Unlock.Nonce))
	if _, err := c.request(frame{Type: "unlock", UnlockToken: token}); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if reply, err := c.request(frame{Type: "host_kind", HostKind: "durable"}); err != nil || reply.HostKind != "durable" {
		t.Fatalf("an unlocked device sets the kind: %+v, %v", reply, err)
	}
	if got := nextHost(t, r.b); got.Kind != "durable" {
		t.Fatalf("the broker should hear the change: %+v", got)
	}
}

// A broker whose ready does not list host-kind would end the link on reg.host.
func TestAnOlderBrokerIsNeverSentTheHostKind(t *testing.T) {
	r := newBrokeredRig(t)
	r.openLocked(t, "c1")
	if err := r.d.setHostKind(hostDurable); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-r.b.hosts:
		t.Fatalf("an older broker got %+v", got)
	case <-time.After(300 * time.Millisecond):
	}
}

func nextHost(t *testing.T, b *fakeBroker) brokerMsg {
	t.Helper()
	select {
	case got := <-b.hosts:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("no reg.host reached the broker")
		return brokerMsg{}
	}
}
