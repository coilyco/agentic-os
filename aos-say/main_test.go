package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseRequest(t *testing.T) {
	var got request
	if err := json.Unmarshal([]byte(`{"text":"build done","voice":"Samantha","rate":190,"notification":true}`), &got); err != nil {
		t.Fatal(err)
	}
	if got.Text != "build done" || got.Voice != "Samantha" || got.Rate != 190 || !got.Notification {
		t.Fatalf("unexpected request: %#v", got)
	}
}

func TestBuildSayCommandPreservesShellMetacharacters(t *testing.T) {
	req := request{
		Text:  `done; $(rm -rf /) "quoted"`,
		Voice: "Samantha",
		Rate:  180,
	}
	got := buildSayCommand(req)
	want := []string{sayBinary, "-v", "Samantha", "-r", "180", `done; $(rm -rf /) "quoted"`}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected argv:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestNotificationCommandEscapesAppleScriptString(t *testing.T) {
	got := buildNotificationCommand(`done; $(rm -rf /) "quoted" \ slash`)
	if len(got) != 3 {
		t.Fatalf("unexpected command len: %#v", got)
	}
	if got[0] != "/usr/bin/osascript" {
		t.Fatalf("unexpected binary: %#v", got[0])
	}
	if !strings.Contains(got[2], `done; $(rm -rf /) \"quoted\" \\ slash`) {
		t.Fatalf("notification command did not escape text: %#v", got[2])
	}
}

func TestRunRemoteMissingRelay(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run([]string{"hello"}, "linux", bytes.NewReader(nil), &stdout, &stderr)
	if got != 1 {
		t.Fatalf("unexpected exit code: %d", got)
	}
	if !strings.Contains(stderr.String(), relayEnv) {
		t.Fatalf("missing relay diagnostics: %q", stderr.String())
	}
}

func TestRunLocalDryRunPrintsCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run([]string{"--dry-run", "--voice", "Samantha", "--rate", "190", "done"}, "darwin", bytes.NewReader(nil), &stdout, &stderr)
	if got != 0 {
		t.Fatalf("unexpected exit code: %d", got)
	}
	out := stdout.String()
	if !strings.Contains(out, sayBinary) || !strings.Contains(out, "done") {
		t.Fatalf("unexpected dry-run output: %q", out)
	}
}

func TestRunRelayReadsOneRequest(t *testing.T) {
	req := request{Text: `done; $(rm -rf /)`}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	oldRunner := runCommand
	defer func() { runCommand = oldRunner }()
	var gotCommands [][]string
	runCommand = func(command []string) error {
		gotCommands = append(gotCommands, append([]string(nil), command...))
		return nil
	}
	var stdout, stderr bytes.Buffer
	got := run([]string{"relay"}, "linux", bytes.NewReader(raw), &stdout, &stderr)
	if got != 0 {
		t.Fatalf("unexpected exit code: %d", got)
	}
	if len(gotCommands) != 1 {
		t.Fatalf("unexpected command count: %#v", gotCommands)
	}
	if gotCommands[0][0] != sayBinary || gotCommands[0][1] != req.Text {
		t.Fatalf("unexpected relay argv: %#v", gotCommands[0])
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("unexpected output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestEscapeAppleScriptString(t *testing.T) {
	got := escapeAppleScriptString(`a\ b "c"`)
	if got != `a\\ b \"c\"` {
		t.Fatalf("unexpected escape: %q", got)
	}
}

func TestRelayTarget(t *testing.T) {
	cases := []struct {
		in      string
		network string
		target  string
	}{
		{"unix:///tmp/aos-say.sock", "unix", "/tmp/aos-say.sock"},
		{"unix:/tmp/aos-say.sock", "unix", "/tmp/aos-say.sock"},
		{"tcp://127.0.0.1:1234", "tcp", "127.0.0.1:1234"},
		{"/tmp/aos-say.sock", "unix", "/tmp/aos-say.sock"},
		{"127.0.0.1:1234", "tcp", "127.0.0.1:1234"},
	}
	for _, tc := range cases {
		network, target := relayTarget(tc.in)
		if network != tc.network || target != tc.target {
			t.Fatalf("relayTarget(%q) = %q, %q", tc.in, network, target)
		}
	}
}
