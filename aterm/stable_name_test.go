package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestClaudeSessionNameReadsTheNameTheProcessWasStartedWith(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		command string
		want    string
	}{
		{"plain", "claude --name aterm --settings s.json", "aterm"},
		{"a name with spaces ps flattened", "claude --name Angie [she] (Platform Engineer) zr87 --settings s.json --model sonnet", "Angie [she] (Platform Engineer) zr87"},
		{"the short flag", "/Users/k/.local/bin/claude -n aterm --model sonnet", "aterm"},
		{"the equals form", "claude --name=aterm --model sonnet", "aterm"},
		{"the last flag wins", "claude --name first --settings s.json --name aterm", "aterm"},
		{"an interpreter first", "/bin/sh /tmp/x/claude --name aterm", "aterm"},
		{"a name running to the end", "claude --model sonnet --name aterm", "aterm"},
		{"not claude", "vim --name aterm", ""},
		{"claude with no name", "claude --model sonnet", ""},
		{"a claude only mentioned later", "grep claude --name aterm", ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := claudeSessionName(testCase.command); got != testCase.want {
				t.Fatalf("claudeSessionName(%q) = %q, want %q", testCase.command, got, testCase.want)
			}
		})
	}
}

func TestHasNameFlagSeesEveryWayACallerNamesASession(t *testing.T) {
	for arguments, want := range map[string]bool{
		"--resume": false, "--name x": true, "-n x": true, "--name=x": true, "--model sonnet --name x": true,
	} {
		if got := hasNameFlag(strings.Fields(arguments)); got != want {
			t.Fatalf("hasNameFlag(%q) = %v, want %v", arguments, got, want)
		}
	}
}

func TestLaunchPlanNamesOnlyClaudeSessionsTheCallerLeftUnnamed(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		args  []string
		named bool
	}{
		{"claude", []string{"eng-platform", "claude"}, true},
		{"claude opted out", []string{"--no-stable-name", "eng-platform", "claude"}, false},
		{"claude with the caller's own name", []string{"eng-platform", "claude", "--", "--name", "mine"}, false},
		{"another seat", []string{"prod-director", "codex"}, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var spawns []recordedSpawn
			args := append([]string{"--dry-run", "--json"}, testCase.args...)
			out, err := runAterm(t, stubDeps(t, &spawns, true), args...)
			if err != nil {
				t.Fatalf("dry run: %v", err)
			}
			var plan launchPlan
			if err := json.Unmarshal([]byte(out), &plan); err != nil {
				t.Fatalf("decode plan: %v", err)
			}
			if plan.StableName != testCase.named {
				t.Fatalf("plan.StableName = %v, want %v", plan.StableName, testCase.named)
			}
			if slices.Contains(plan.Arguments, "--stable-name") != testCase.named {
				t.Fatalf("session flag mismatch in %v", plan.Arguments)
			}
			// agent-compose drops its own name when it is handed one, so the flag
			// has to reach it ahead of the caller's own arguments.
			carriesName := slices.Contains(plan.Child, stableSessionName("Angie", "eng-platform"))
			if carriesName != testCase.named {
				t.Fatalf("the child should carry the name only when named: %v", plan.Child)
			}
		})
	}
}

// fakeProcesses answers the listing and the signals from a table, and treats a
// process as gone once it has been sent SIGTERM.
func fakeProcesses(self int, entries []processEntry) (sessionReaper, *[]int, *bytes.Buffer) {
	var stopped []int
	ended := map[int]bool{}
	notice := &bytes.Buffer{}
	return sessionReaper{
		alive: func(pid int) bool { return !ended[pid] },
		signal: func(pid int, _ syscall.Signal) error {
			ended[pid] = true
			stopped = append(stopped, pid)
			return nil
		},
		processes: func() ([]processEntry, error) { return entries, nil },
		self:      self,
		wait:      func(time.Duration) {},
		notice:    notice,
	}, &stopped, notice
}

func TestClearClaudeStopsOnlyRunningSessionsOfThatName(t *testing.T) {
	reaper, stopped, _ := fakeProcesses(900, []processEntry{
		{PID: 10, PPID: 1, Command: "claude --name eng-platform-angie --settings s.json"},
		{PID: 11, PPID: 1, Command: "claude --name Sprite [they] (Game Developer) ea64 --model sonnet"},
		{PID: 12, PPID: 1, Command: "vim --name eng-platform-angie"},
		{PID: 13, PPID: 1, Command: "claude --model sonnet"},
		{PID: 900, PPID: 899, Command: "aterm _session --stable-name -- claude"},
	})
	if got := reaper.clearClaude(stableSessionName("Angie", "eng-platform")); got != 1 {
		t.Fatalf("stopped = %d, want 1", got)
	}
	if !slices.Equal(*stopped, []int{10}) {
		t.Fatalf("stopped pids = %v, want only 10", *stopped)
	}
}

func TestStableSessionNameIsPerRole(t *testing.T) {
	if stableSessionName("Angie", "eng-platform") == stableSessionName("Evie", "scientist") {
		t.Fatal("two roles must not share a name, or a launch of one ends the other")
	}
	if stableSessionName("Angie", "eng-platform") != stableSessionName("Angie", "eng-platform") {
		t.Fatal("a role must get the same name every launch, or nothing is cleared")
	}
	if stableSessionName("", "") != "" {
		t.Fatalf("no role should mean no name, got %q", stableSessionName("", ""))
	}
}

func TestStableSessionNameIsForWhoAnswers(t *testing.T) {
	for _, testCase := range []struct {
		name string
		role string
		want string
	}{
		{"Vera", "sysadmin-senior", "sysadmin-senior-vera"},
		{"Angie", "eng-platform", "eng-platform-angie"},
		{"Valerie", "sysadmin-junior", "sysadmin-junior-valerie"},
		{"Vera", "sysadmin-access", "sysadmin-access-vera"},
		{"Vera", "", ""},
	} {
		if got := stableSessionName(testCase.name, testCase.role); got != testCase.want {
			t.Fatalf("stableSessionName(%q, %q) = %q, want %q", testCase.name, testCase.role, got, testCase.want)
		}
	}
}

func TestClearClaudeLeavesAnotherRolesSessionRunning(t *testing.T) {
	reaper, stopped, _ := fakeProcesses(900, []processEntry{
		{PID: 10, PPID: 1, Command: "claude --name eng-platform-angie"},
		{PID: 11, PPID: 1, Command: "claude --name scientist-evie"},
		{PID: 12, PPID: 1, Command: "claude --name sysadmin-senior-vera"},
	})
	if got := reaper.clearClaude(stableSessionName("Angie", "eng-platform")); got != 1 {
		t.Fatalf("stopped = %d, want 1", got)
	}
	if !slices.Equal(*stopped, []int{10}) {
		t.Fatalf("stopped pids = %v, want only the platform session 10", *stopped)
	}
}

func TestClearClaudeWithNoNameStopsNothing(t *testing.T) {
	// claudeSessionName is "" for an unnamed claude, so an empty name would match it.
	reaper, stopped, _ := fakeProcesses(900, []processEntry{
		{PID: 10, PPID: 1, Command: "claude --model sonnet"},
		{PID: 11, PPID: 1, Command: "claude --name scientist-evie"},
	})
	if got := reaper.clearClaude(stableSessionName("", "")); got != 0 || len(*stopped) != 0 {
		t.Fatalf("stopped = %d pids %v, want none", got, *stopped)
	}
}

func TestClearClaudeNeverStopsTheProcessThatHostsTheSession(t *testing.T) {
	// A launch run from inside a claude named aterm must not end that claude,
	// because it is an ancestor of the process asking.
	reaper, stopped, _ := fakeProcesses(900, []processEntry{
		{PID: 50, PPID: 1, Command: "claude --name eng-platform-angie"},
		{PID: 60, PPID: 50, Command: "zsh"},
		{PID: 900, PPID: 60, Command: "aterm _session --stable-name -- claude"},
		{PID: 70, PPID: 1, Command: "claude --name eng-platform-angie"},
	})
	if got := reaper.clearClaude(stableSessionName("Angie", "eng-platform")); got != 1 {
		t.Fatalf("stopped = %d, want 1", got)
	}
	if !slices.Equal(*stopped, []int{70}) {
		t.Fatalf("stopped pids = %v, want only the unrelated 70", *stopped)
	}
}

func TestClearClaudeNamesASurvivor(t *testing.T) {
	reaper, _, notice := fakeProcesses(900, []processEntry{{PID: 10, PPID: 1, Command: "claude --name eng-platform-angie"}})
	reaper.alive = func(int) bool { return true }
	if got := reaper.clearClaude(stableSessionName("Angie", "eng-platform")); got != 0 {
		t.Fatalf("stopped = %d, want 0", got)
	}
	if !strings.Contains(notice.String(), "would not end") {
		t.Fatalf("the survivor should be named: %q", notice.String())
	}
}

func TestListProcessesSeesThisProcessAndItsParent(t *testing.T) {
	entries, err := listProcesses()
	if err != nil {
		t.Skipf("this host's ps cannot list processes: %v", err)
	}
	for _, entry := range entries {
		if entry.PID == os.Getpid() {
			if entry.PPID != os.Getppid() || entry.Command == "" {
				t.Fatalf("entry = %+v, want ppid %d and a command", entry, os.Getppid())
			}
			return
		}
	}
	t.Fatalf("this process %d is missing from the listing", os.Getpid())
}

func TestSystemReaperReadsAnEndedButUnreapedProcessAsGone(t *testing.T) {
	child := exec.Command("sh", "-c", "exit 0")
	if err := child.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Not waited on yet, so it stays a zombie until the test reaps it.
	t.Cleanup(func() { _ = child.Wait() })
	deadline := time.Now().Add(5 * time.Second)
	for !isZombie(child.Process.Pid) {
		if time.Now().After(deadline) {
			t.Skip("this host's ps does not report a zombie state")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if systemReaper(&bytes.Buffer{}).alive(child.Process.Pid) {
		t.Fatal("an ended process awaiting its parent should not read as alive")
	}
}
