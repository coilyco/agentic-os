package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// goldenSnapshot was written by the real configsource.EncodeSnapshot in treehouse-servers
// (agent-compose, aac5d49), so a reader that passes it agrees with the writer's hash.
const goldenSnapshot = "testdata/treehouse-config-snapshot.v1.json"

// publish writes a snapshot with the real hash spelling, atomically as the writer does.
func publish(t *testing.T, dir string, settings map[string]map[string]bool) {
	t.Helper()
	// The writer hashes its struct's compact spelling. Rebuild that spelling field by field.
	spelled, _ := json.Marshal(struct {
		Revision      int64                      `json:"revision"`
		Creatures     []any                      `json:"creatures"`
		Personalities []any                      `json:"personalities"`
		Roles         []any                      `json:"roles"`
		Settings      map[string]map[string]bool `json:"settings"`
	}{1, []any{}, []any{}, []any{}, settings})
	file, _ := json.MarshalIndent(map[string]any{"format": configSnapshotFormat, "sha256": sha256hex(spelled), "config": json.RawMessage(spelled)}, "", "  ")
	tmp := filepath.Join(dir, ".snapshot.tmp")
	if err := os.WriteFile(tmp, file, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, configSnapshotFile)); err != nil {
		t.Fatal(err)
	}
	// A rewrite inside one clock tick keeps the same size and time, so move the time on.
	later := time.Now().Add(time.Duration(len(file)) * time.Millisecond)
	_ = os.Chtimes(filepath.Join(dir, configSnapshotFile), later, later)
}

func TestTheReaderAgreesWithTheRealWritersGoldenSnapshot(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile(goldenSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, configSnapshotFile), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	r := newSettingsReader(dir)
	for _, tc := range []struct {
		user, key string
		want      bool
	}{
		{"acct_01HZX3K9Q7M2", settingPushQuestionText, true},
		{"acct_OFF00000000A", settingPushQuestionText, false},
		{"acct_ODD&<name>00000", settingPushQuestionText, true},
		{"acct_NOBODYHASSET", settingPushQuestionText, false},
	} {
		if got := r.Bool(tc.user, tc.key, false); got != tc.want {
			t.Fatalf("%s %s is %v, want %v (reader error: %v)", tc.user, tc.key, got, tc.want, r.Err())
		}
	}
	if r.Err() != nil {
		t.Fatalf("the golden snapshot verifies: %v", r.Err())
	}
	// A byte of the config changed, with the hash left alone, must not load.
	tampered := strings.Replace(string(raw), `"revision": 3`, `"revision": 4`, 1)
	bad := t.TempDir()
	_ = os.WriteFile(filepath.Join(bad, configSnapshotFile), []byte(tampered), 0o600)
	rb := newSettingsReader(bad)
	if rb.Bool("acct_01HZX3K9Q7M2", settingPushQuestionText, false) || rb.Err() == nil {
		t.Fatalf("a snapshot whose config no longer matches its hash must not load: %v", rb.Err())
	}
}

func TestASettingResolvesUserThenEveryoneThenTheDefault(t *testing.T) {
	dir := t.TempDir()
	publish(t, dir, map[string]map[string]bool{
		"acct_ME":    {settingPushQuestionText: true},
		"acct_OTHER": {settingPushQuestionText: false},
		"*":          {"terminal.e2e_frames": true},
	})
	r := newSettingsReader(dir)
	if !r.Bool("acct_ME", settingPushQuestionText, false) || r.Bool("acct_OTHER", settingPushQuestionText, true) || r.Bool("acct_NONE", settingPushQuestionText, false) {
		t.Fatalf("user value, then everyone, then default: %v", r.Err())
	}
	if !r.Bool("acct_NONE", "terminal.e2e_frames", false) || !r.Bool("acct_OTHER", "terminal.e2e_frames", false) {
		t.Fatal("an everyone value applies to a user with no value of their own")
	}
	if newSettingsReader(t.TempDir()).Bool("acct_ME", settingPushQuestionText, false) {
		t.Fatal("no snapshot reads as the default")
	}
}

func TestABadPublishKeepsWhatWasLoadedAndLastGoodServesAFirstLoad(t *testing.T) {
	dir := t.TempDir()
	publish(t, dir, map[string]map[string]bool{"acct_ME": {settingPushQuestionText: true}})
	r := newSettingsReader(dir)
	if !r.Bool("acct_ME", settingPushQuestionText, false) {
		t.Fatalf("the first publish loads: %v", r.Err())
	}
	good, _ := os.ReadFile(filepath.Join(dir, configSnapshotFile))
	if err := os.WriteFile(filepath.Join(dir, configSnapshotFile), good[:len(good)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	_ = os.Chtimes(filepath.Join(dir, configSnapshotFile), later, later)
	if !r.Bool("acct_ME", settingPushQuestionText, false) || r.Err() == nil {
		t.Fatalf("a truncated publish keeps the last values and says why: %v", r.Err())
	}
	// A fresh reader meeting the same bad file falls back to the last-good snapshot.
	if err := os.WriteFile(filepath.Join(dir, configLastGoodFile), good, 0o600); err != nil {
		t.Fatal(err)
	}
	if !newSettingsReader(dir).Bool("acct_ME", settingPushQuestionText, false) {
		t.Fatal("a first load falls back to the last good snapshot")
	}
	if err := os.WriteFile(filepath.Join(dir, configSnapshotFile), []byte(`{"format":"other","sha256":"","config":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(dir, configLastGoodFile))
	if newSettingsReader(dir).Bool("acct_ME", settingPushQuestionText, false) {
		t.Fatal("a wrong format with nothing good behind it reads as the default")
	}
}

func TestFirstCharsNeverSplitsACharacter(t *testing.T) {
	long := strings.Repeat("é", 200)
	if got := firstChars(long, 140); got != strings.Repeat("é", 140) {
		t.Fatalf("140 whole characters, got %d bytes", len(got))
	}
	if got := firstChars("short", 140); got != "short" {
		t.Fatal("a short question is kept whole")
	}
	if got := firstChars(strings.Repeat("a", 140), 140); len(got) != 140 {
		t.Fatal("exactly 140 characters is kept whole")
	}
}

// askEvent asks on the rig and returns the exact bytes of the reg.event the broker got.
func askEvent(t *testing.T, r *brokeredRig, question string) []byte {
	t.Helper()
	notifyRoster(t, "scientist")
	r.b.attach(t, "c1", testOwner)
	deadline := time.After(10 * time.Second)
	for {
		r.d.users.NotifyAsk(choiceAsk{Question: question, Session: "scientist-evie", From: "scientist Evie"})
		select {
		case raw := <-r.b.rawEvents:
			// Drain duplicates from the retry so the next call starts clean.
			time.Sleep(50 * time.Millisecond)
			for len(r.b.rawEvents) > 0 {
				<-r.b.rawEvents
			}
			return raw
		case <-time.After(100 * time.Millisecond):
		case <-deadline:
			t.Fatal("the broker never heard of the ask")
		}
	}
}

func payload(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestWithTheSettingOffTheQuestionNeverLeavesTheDaemon(t *testing.T) {
	question := strings.Repeat("Which branch should I merge? ", 10)
	for name, settings := range map[string]map[string]map[string]bool{
		"no snapshot":    nil,
		"set to false":   {testOwner: {settingPushQuestionText: false}},
		"everyone false": {settingAllUsersKey: {settingPushQuestionText: false}},
		"another user":   {"acct_SOMEONEELSE": {settingPushQuestionText: true}},
	} {
		t.Run(name, func(t *testing.T) {
			r := prepareBrokeredRig(t, time.Hour)
			if settings != nil {
				publish(t, r.configDir, settings)
			}
			r.start()
			raw := askEvent(t, r, question)
			want := map[string]any{"type": "treehouse.reg.event.v1", "kind": "ask", "session": "scientist-evie", "identity": "Evie"}
			if got := payload(t, raw); !reflect.DeepEqual(got, want) || strings.Contains(string(raw), "Which branch") {
				t.Fatalf("an off setting sends exactly %v, got %s", want, raw)
			}
		})
	}
}

const settingAllUsersKey = settingAllUsers

func TestWithTheSettingOnTheFirst140CharactersRideTheEvent(t *testing.T) {
	question := strings.Repeat("é", 100) + strings.Repeat("x", 100)
	r := prepareBrokeredRig(t, time.Hour)
	publish(t, r.configDir, map[string]map[string]bool{testOwner: {settingPushQuestionText: true}})
	r.start()
	raw := askEvent(t, r, question)
	want := map[string]any{
		"type": "treehouse.reg.event.v1", "kind": "ask", "session": "scientist-evie", "identity": "Evie",
		"question": strings.Repeat("é", 100) + strings.Repeat("x", 40),
	}
	if got := payload(t, raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("an on setting sends exactly %v, got %s", want, raw)
	}
	if n := len([]rune(payload(t, raw)["question"].(string))); n != questionChars {
		t.Fatalf("the question is %d characters, want %d", n, questionChars)
	}
}

func TestAnEveryoneSettingTurnsTheQuestionOnForTheOwner(t *testing.T) {
	r := prepareBrokeredRig(t, time.Hour)
	publish(t, r.configDir, map[string]map[string]bool{settingAllUsers: {settingPushQuestionText: true}})
	r.start()
	if got := payload(t, askEvent(t, r, "Merge it?"))["question"]; got != "Merge it?" {
		t.Fatalf("an everyone value reaches the owner: %v", got)
	}
}

func TestOnlyAnAskCarriesAQuestionAndASettingChangeAppliesToTheNextEvent(t *testing.T) {
	dir := t.TempDir()
	var sent []brokerMsg
	d := newDaemon(func(string, ...any) {})
	link := &brokerLink{d: d, identity: daemonIdentity{ID: testDaemonID, Owner: testOwner}, settings: newSettingsReader(dir),
		sendEvent: func(m brokerMsg) { sent = append(sent, m) }}
	ask := waitEvent{Kind: "ask", Session: "scientist-evie", Identity: "Evie", Question: "Merge it?"}

	link.emit(ask) // no snapshot yet
	publish(t, dir, map[string]map[string]bool{testOwner: {settingPushQuestionText: true}})
	link.emit(ask)
	link.emit(waitEvent{Kind: "done", Session: "scientist-evie", Identity: "Evie", Question: "leaked?"})
	link.emit(waitEvent{Kind: "prompt", Session: "scientist-evie", Identity: "Evie", Question: "leaked?"})
	publish(t, dir, map[string]map[string]bool{testOwner: {settingPushQuestionText: false}})
	link.emit(ask)

	var got []string
	for _, m := range sent {
		got = append(got, m.Kind+":"+m.Question)
	}
	want := []string{"ask:", "ask:Merge it?", "done:", "prompt:", "ask:"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events carried %v, want %v", got, want)
	}
}

func TestStartTreehouseUsesTheConfigDirFlagThenTheEnvThenHome(t *testing.T) {
	t.Setenv(configDirEnv, "/env/dir")
	if configDir("/flag/dir") != "/flag/dir" || configDir("") != "/env/dir" {
		t.Fatal("the flag wins over the environment")
	}
	t.Setenv(configDirEnv, "off")
	if configDir("") != "" {
		t.Fatal("off reads no snapshot")
	}
	t.Setenv(configDirEnv, "")
	if dir := configDir(""); !strings.HasSuffix(dir, filepath.Join(".agent-compose", "config")) {
		t.Fatalf("the default is under the home directory: %q", dir)
	}
	_ = context.Background
}

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
