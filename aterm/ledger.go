package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ledgerFormat names a session record on disk. A record carries what a relaunch
// needs and never the environment, which holds credentials.
const ledgerFormat = "aterm.ledger.v1"

const (
	stateDirEnv = "ATERM_STATE_DIR"
	// An ended session's record stays resumable this long.
	ledgerKeep = 30 * 24 * time.Hour
)

type ledgerEntry struct {
	Format   string   `json:"format"`
	Name     string   `json:"name"`
	Role     string   `json:"role"`
	Identity string   `json:"identity"`
	Seat     string   `json:"seat"`
	Cwd      string   `json:"cwd"`
	Argv     []string `json:"argv"`
	// Conversation is the harness conversation id a relaunch resumes, claude's
	// today. Empty means this seat cannot be resumed.
	Conversation string `json:"conversation,omitempty"`
	// Home is the HOME the seat was spawned with, a path and not the environment.
	// A shadow moves it, and the harness keeps its transcript there.
	Home string `json:"home,omitempty"`
	// Dirs are the path variables a harness reads its own files from, if the seat set
	// them. They and Home are the only environment that reaches a record.
	Dirs    map[string]string `json:"dirs,omitempty"`
	Started time.Time         `json:"started"`
	Ended   *time.Time        `json:"ended,omitempty"`
	Code    *int              `json:"code,omitempty"`
}

// ledgerDir is under the real home from the password database, since a shadow
// moves $HOME and a daemon may start inside one, and outside /tmp for reboots.
func ledgerDir() string {
	if override := strings.TrimSpace(os.Getenv(stateDirEnv)); override != "" {
		return override
	}
	return defaultLedgerDir()
}

// defaultLedgerDir is the user's own ledger, which the tests must never touch.
func defaultLedgerDir() string {
	return filepath.Join(realHome(), ".local", "state", "aterm", "sessions")
}

// ledgerGuard is nil in the shipped binary. TestMain sets it once, never a test, since
// earlier tests' daemon goroutines read it. Tests pass their own to the *Guarded forms.
var ledgerGuard func(dir string) error

func (e ledgerEntry) file(dir string) string { return filepath.Join(dir, fileStem(e.Name)+".json") }

// writeLedger replaces a record atomically, so a crash never leaves half of one.
func writeLedger(dir string, entry ledgerEntry) error {
	return writeLedgerGuarded(ledgerGuard, dir, entry)
}

func writeLedgerGuarded(guard func(string) error, dir string, entry ledgerEntry) error {
	if guard != nil {
		if err := guard(dir); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	entry.Format = ledgerFormat
	encoded, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".ledger-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(append(encoded, '\n')); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), entry.file(dir))
}

// readLedger returns every record, oldest first. A record it cannot read is
// skipped, not fatal, since one bad file must not hide the rest.
func readLedger(dir string) []ledgerEntry {
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	entries := make([]ledgerEntry, 0, len(files))
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var entry ledgerEntry
		if json.Unmarshal(raw, &entry) != nil || entry.Format != ledgerFormat || entry.Name == "" {
			continue
		}
		entries = append(entries, entry)
	}
	sortLedger(entries)
	return entries
}

func sortLedger(entries []ledgerEntry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].Started.Before(entries[j-1].Started); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

// pruneLedger removes records of sessions that ended past ledgerKeep ago.
func pruneLedger(dir string, now time.Time) {
	pruneLedgerGuarded(ledgerGuard, dir, now)
}

func pruneLedgerGuarded(guard func(string) error, dir string, now time.Time) {
	if guard != nil && guard(dir) != nil {
		return
	}
	for _, entry := range readLedger(dir) {
		if entry.Ended != nil && now.Sub(*entry.Ended) > ledgerKeep {
			_ = os.Remove(entry.file(dir))
		}
	}
}

var conversationFlag = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// conversationOf is the claude conversation id in a launch argv: the uuid after
// the last --session-id, or after --resume, which keeps the id it resumes.
func conversationOf(argv []string) string {
	found := ""
	for index := 0; index+1 < len(argv); index++ {
		switch argv[index] {
		case "--session-id", "--resume", "-r":
			if conversationFlag.MatchString(argv[index+1]) {
				found = argv[index+1]
			}
		}
	}
	return found
}

// newConversationID is a random v4 uuid, claude's --session-id format.
func newConversationID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
}

// hasAnyFlag reports whether args carry one of the flags, bare or as flag=value.
func hasAnyFlag(args []string, flags ...string) bool {
	for _, arg := range args {
		for _, flag := range flags {
			if arg == flag || strings.HasPrefix(arg, flag+"=") {
				return true
			}
		}
	}
	return false
}
