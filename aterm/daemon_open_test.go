package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// openRig is a real daemon with one seat whose token and launch environment the
// test can read, and a way to ask as that seat.
type openRig struct {
	t      *testing.T
	socket string
	seat   *testClient
	token  string
}

func newOpenRig(t *testing.T) *openRig {
	t.Helper()
	socket := testDaemon(t)
	seat := dialTest(t)
	seat.spawn("scientist-evie", "scientist", "Evie",
		`echo TOKEN=$ATERM_SESSION_TOKEN; echo BROWSER=$BROWSER; echo PATH=$PATH; sleep 30`)
	return &openRig{t: t, socket: socket, seat: seat, token: seat.token()}
}

func (r *openRig) ask(token, address string) (frame, error) {
	r.t.Helper()
	c := dialTest(r.t)
	return c.c.request(frame{Type: "open_url", Token: token, URL: address})
}

// watcher is a client subscribed to open_url, as the web client does.
func (r *openRig) watcher() *testClient {
	r.t.Helper()
	w := dialTest(r.t)
	if _, err := w.c.request(frame{Type: "subscribe", Channel: "open_url"}); err != nil {
		r.t.Fatalf("subscribe: %v", err)
	}
	return w
}

func TestOpenURLReachesASubscribedClientLabeledWithTheSeat(t *testing.T) {
	r := newOpenRig(t)
	w := r.watcher()
	reply, err := r.ask(r.token, "https://example.com/a?b=c")
	if err != nil || reply.Type != "opened" || reply.Delivered != 1 || reply.Session != "scientist-evie" {
		t.Fatalf("one client took it: %+v, %v", reply, err)
	}
	pushed := nextFrame(t, w.c, "open_url")
	if pushed.Session != "scientist-evie" || pushed.URL != "https://example.com/a?b=c" || pushed.Token != "" {
		t.Fatalf("the push names the seat and the address and never the token: %+v", pushed)
	}
}

func TestOpenURLWithNoClientDeliversToNone(t *testing.T) {
	r := newOpenRig(t)
	reply, err := r.ask(r.token, "https://example.com")
	if err != nil || reply.Delivered != 0 {
		t.Fatalf("no subscriber means nobody took it: %+v, %v", reply, err)
	}
	// Subscribing to sessions is not a promise to show a page.
	c := dialTest(t)
	if _, err := c.c.request(frame{Type: "subscribe", Channel: "sessions"}); err != nil {
		t.Fatal(err)
	}
	if reply, _ := r.ask(r.token, "https://example.com"); reply.Delivered != 0 {
		t.Fatalf("a sessions subscriber is not an opener: %+v", reply)
	}
}

func TestOpenURLStopsCountingAClientThatLeft(t *testing.T) {
	r := newOpenRig(t)
	w := r.watcher()
	_ = w.c.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		reply, err := r.ask(r.token, "https://example.com")
		if err == nil && reply.Delivered == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("a closed client should drop out: %+v, %v", reply, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestOpenURLRefusesWhatIsNotAWebAddress(t *testing.T) {
	r := newOpenRig(t)
	for name, address := range map[string]string{
		"file":       "file:///etc/passwd",
		"javascript": "javascript:alert(1)",
		"ftp":        "ftp://example.com/x",
		"no host":    "https://",
		"bare":       "example.com",
		"empty":      "",
		"control":    "https://example.com/\x00",
	} {
		reply, err := r.ask(r.token, address)
		if err == nil || reply.Reason != reasonBadURL {
			t.Fatalf("%s should be %s: %+v, %v", name, reasonBadURL, reply, err)
		}
	}
	reply, err := r.ask(r.token, "https://example.com/"+strings.Repeat("a", maxOpenURL))
	if err == nil || reply.Reason != reasonTooLarge {
		t.Fatalf("an oversize address should be %s: %+v, %v", reasonTooLarge, reply, err)
	}
}

func TestOpenURLNeedsALiveSeatToken(t *testing.T) {
	r := newOpenRig(t)
	for _, token := range []string{"", "not-a-token"} {
		if reply, err := r.ask(token, "https://example.com"); err == nil {
			t.Fatalf("token %q names no seat and must be refused: %+v", token, reply)
		}
	}
}

func TestOpenURLLimitsASeatThatAsksInALoop(t *testing.T) {
	r := newOpenRig(t)
	for i := 0; i < maxOpensPerWindow; i++ {
		if _, err := r.ask(r.token, "https://example.com"); err != nil {
			t.Fatalf("ask %d should be taken: %v", i, err)
		}
	}
	reply, err := r.ask(r.token, "https://example.com")
	if err == nil || reply.Reason != reasonTooMany {
		t.Fatalf("past the limit is %s: %+v, %v", reasonTooMany, reply, err)
	}
}

func TestSeatLaunchGetsTheShimsAndATerminalDoesNot(t *testing.T) {
	r := newOpenRig(t)
	shims := filepath.Join(filepath.Dir(r.socket), shimDirName)
	r.seat.untilMatch("BROWSER and PATH lines", func(out string) bool {
		return strings.Contains(out, "BROWSER=") && strings.Contains(out, "PATH=") && strings.Count(out, "\n") >= 3
	})
	out := r.seat.output.String()
	if !strings.Contains(out, "BROWSER="+filepath.Join(shims, "open")) || !strings.Contains(out, "PATH="+shims+":") {
		t.Fatalf("a seat should lead with the shims:\n%s", out)
	}
	for _, name := range shimNames {
		info, err := os.Stat(filepath.Join(shims, name))
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("%s should be an executable only the user may touch: %v, %v", name, info, err)
		}
	}
	if info, err := os.Stat(shims); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("the shim directory is the user's alone: %v, %v", info, err)
	}
}

func TestATerminalKeepsItsOwnBrowser(t *testing.T) {
	tc := shellDaemon(t)
	terminal := tc.spawnTerminal("/")
	if _, err := tc.c.request(frame{Type: "attach", Session: terminal.Session, Rows: 24, Cols: 80}); err != nil {
		t.Fatal(err)
	}
	tc.type_(terminal.Session, "echo SHIMS=${BROWSER:-none}\n")
	tc.untilMatch("SHIMS= line", func(out string) bool { return strings.Contains(out, "SHIMS=") && strings.Contains(out, "\n") })
	if strings.Contains(tc.output.String(), shimDirName+"/open") {
		t.Fatalf("a login shell is Kai's, so it keeps its browser:\n%s", tc.output.String())
	}
}

func TestShimQuotesTheExecutableAndPassesEveryArgument(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "aterm-shim-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	executable := filepath.Join(dir, "it's here")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s|' \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	shims := filepath.Join(dir, shimDirName)
	if err := writeShims(shims, executable); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(filepath.Join(shims, "xdg-open"), "-a", "two words", "https://x").Output()
	if err != nil || string(out) != "open|-a|two words|https://x|" {
		t.Fatalf("the shim should run `aterm open` with its arguments intact: %q, %v", out, err)
	}
}

func TestWithOpenShimsLeadsPATHAndReplacesBROWSER(t *testing.T) {
	d := newDaemon(func(string, ...any) {})
	if got := d.withOpenShims([]string{"A=1"}); !slices.Equal(got, []string{"A=1"}) {
		t.Fatalf("a daemon with no shims changes nothing: %v", got)
	}
	d.shimDir = "/tmp/aterm-x/shims"
	got := d.withOpenShims([]string{"BROWSER=firefox", "PATH=/usr/bin:/bin", "A=1"})
	want := []string{"PATH=/tmp/aterm-x/shims:/usr/bin:/bin", "A=1", "BROWSER=/tmp/aterm-x/shims/open"}
	if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, entry := range d.withOpenShims([]string{"A=1"}) {
		if strings.HasPrefix(entry, "PATH=") {
			t.Fatalf("a launch with no PATH must not gain a PATH of only the shims: %v", entry)
		}
	}
}

func TestOpenSendsOneWebAddressToTheDaemonAndNoneElse(t *testing.T) {
	type call struct{ asked, native []string }
	run := func(args []string, taken int, askErr error) (call, error) {
		var c call
		err := runOpen(args, openDeps{
			ask:    func(address string) (int, error) { c.asked = append(c.asked, address); return taken, askErr },
			native: func(a []string) error { c.native = a; return nil },
		})
		return c, err
	}
	if c, err := run([]string{"https://example.com"}, 1, nil); err != nil || c.native != nil || len(c.asked) != 1 {
		t.Fatalf("a client took it, so no native browser: %+v, %v", c, err)
	}
	for name, tc := range map[string]struct {
		taken int
		err   error
	}{
		"no client":     {0, nil},
		"daemon absent": {0, errors.New("not answering")},
		"not in a seat": {0, errors.New("not in an aterm seat")},
	} {
		c, err := run([]string{"https://example.com"}, tc.taken, tc.err)
		if err != nil || !slices.Equal(c.native, []string{"https://example.com"}) {
			t.Fatalf("%s falls back to the native browser: %+v, %v", name, c, err)
		}
	}
	for _, args := range [][]string{nil, {"."}, {"notes.txt"}, {"-a", "Safari", "https://example.com"}, {"https://a", "https://b"}, {"mailto:a@b.c"}} {
		c, err := run(args, 1, nil)
		if err != nil || len(c.asked) != 0 || !slices.Equal(c.native, args) {
			t.Fatalf("%v is not one web address, so the native opener gets it unchanged: %+v, %v", args, c, err)
		}
	}
	c, err := run([]string{"https://example.com"}, 0, withReason(reasonTooMany, errors.New("too often")))
	if err == nil || c.native != nil {
		t.Fatalf("a seat refused for asking too often must not open the native browser: %+v, %v", c, err)
	}
}

func TestOpenGoesStraightToNativeInsideTheNativeOpener(t *testing.T) {
	t.Setenv(nativeOpenEnv, "1")
	asked := false
	var native []string
	err := runOpen([]string{"https://example.com"}, openDeps{
		ask:    func(string) (int, error) { asked = true; return 1, nil },
		native: func(a []string) error { native = a; return nil },
	})
	if err != nil || asked || len(native) != 1 {
		t.Fatalf("the loop guard skips the daemon: asked=%v native=%v err=%v", asked, native, err)
	}
}

func TestNativeOpenerEnvironmentCannotReachTheShimsAgain(t *testing.T) {
	shims := "/tmp/aterm-x/shims"
	got := withoutShims([]string{"BROWSER=" + shims + "/open", "PATH=" + shims + ":/usr/bin", "KEEP=1"}, shims)
	if slices.Contains(got, "BROWSER="+shims+"/open") || !slices.Contains(got, "PATH=/usr/bin") ||
		!slices.Contains(got, "KEEP=1") || !slices.Contains(got, nativeOpenEnv+"=1") {
		t.Fatalf("got %v", got)
	}
	own := withoutShims([]string{"BROWSER=/usr/bin/firefox"}, shims)
	if !slices.Contains(own, "BROWSER=/usr/bin/firefox") {
		t.Fatalf("a BROWSER that is not a shim is the person's own: %v", own)
	}
	if runtime.GOOS == "darwin" {
		if opener, err := nativeOpener(""); err != nil || opener != "/usr/bin/open" {
			t.Fatalf("macOS opens by absolute path: %q, %v", opener, err)
		}
	}
}

func TestAskDaemonToOpenSpeaksAsTheSeatThroughARealDaemon(t *testing.T) {
	r := newOpenRig(t)
	t.Setenv(sessionTokenEnv, "")
	if _, err := askDaemonToOpen("https://example.com"); err == nil {
		t.Fatal("outside a seat there is nothing to speak as")
	}
	t.Setenv(sessionTokenEnv, r.token)
	if taken, err := askDaemonToOpen("https://example.com"); err != nil || taken != 0 {
		t.Fatalf("no client yet: %d, %v", taken, err)
	}
	w := r.watcher()
	if taken, err := askDaemonToOpen("https://example.com"); err != nil || taken != 1 {
		t.Fatalf("a client took it: %d, %v", taken, err)
	}
	if pushed := nextFrame(t, w.c, "open_url"); pushed.Session != "scientist-evie" {
		t.Fatalf("%+v", pushed)
	}
}
