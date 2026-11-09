package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// importsHome is a host home holding the two approved base files with the given lines.
func importsHome(t *testing.T, base, kai string) string {
	t.Helper()
	home := t.TempDir()
	for relative, body := range map[string]string{
		"projects/coilyco/agentic-os/AGENTS.md":     base,
		"projects/coilyco/agentic-os-kai/AGENTS.md": kai,
	} {
		path := filepath.Join(home, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func writeClaudeMd(t *testing.T, home, name, body string) string {
	t.Helper()
	path := filepath.Join(home, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImportsApprovedOnlyForExactlyTheTwoBaseFiles(t *testing.T) {
	home := importsHome(t, "# base\nNo imports here. `@name` is not one.\n", "# kai\n")
	base := filepath.Join(home, "projects/coilyco/agentic-os/AGENTS.md")
	kai := filepath.Join(home, "projects/coilyco/agentic-os-kai/AGENTS.md")
	pair := writeClaudeMd(t, home, "pair/CLAUDE.md", "@"+base+"\n\n@"+kai+"\n")
	if !nativeClaudeImportsApproved(home, nil, pair) {
		t.Fatal("the two approved files, and nothing else, should be approved")
	}
	third := writeClaudeMd(t, home, "third/CLAUDE.md", "@"+base+"\n@"+kai+"\n@"+filepath.Join(home, "secrets.md")+"\n")
	if nativeClaudeImportsApproved(home, nil, third) {
		t.Fatal("a third import must stop the approval")
	}
	tilde := writeClaudeMd(t, home, "tilde/CLAUDE.md", "@~/projects/coilyco/agentic-os/AGENTS.md\n")
	if !nativeClaudeImportsApproved(home, nil, tilde) {
		t.Fatal("a ~ spelling of an approved file is that file")
	}
}

func TestImportsFollowAnApprovedFilesOwnImports(t *testing.T) {
	home := importsHome(t, "# base\n", "# kai\n")
	base := filepath.Join(home, "projects/coilyco/agentic-os/AGENTS.md")
	if err := os.WriteFile(base, []byte("# base\n@"+filepath.Join(home, "elsewhere.md")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := writeClaudeMd(t, home, "CLAUDE.md", "@"+base+"\n")
	if nativeClaudeImportsApproved(home, nil, source) {
		t.Fatal("an approved file that imports a third file makes the dialog list it, so no approval")
	}
}

func TestImportsInTheSessionsOwnTreeAreNotExternal(t *testing.T) {
	home := importsHome(t, "# base\n", "# kai\n")
	session := filepath.Join(home, "session")
	base := filepath.Join(home, "projects/coilyco/agentic-os/AGENTS.md")
	source := writeClaudeMd(t, home, "CLAUDE.md", "@"+base+"\n@"+filepath.Join(session, "local.md")+"\n")
	if !nativeClaudeImportsApproved(home, []string{session}, source) {
		t.Fatal("an import under the session's own tree is not an external one")
	}
	if nativeClaudeImportsApproved(home, nil, source) {
		t.Fatal("the same import is a third file when it is not under an ignored root")
	}
}

func TestNoReadableSourceOrARelativeOneApprovesAsBefore(t *testing.T) {
	home := importsHome(t, "# base\n", "# kai\n")
	if !nativeClaudeImportsApproved(home, nil, filepath.Join(home, "missing.md"), "", ".claude/CLAUDE.md") {
		t.Fatal("a missing, empty or relative source imports nothing")
	}
}

func TestSeedLeavesTheExternalApprovalOffUnlessAskedAndKeepsTheRest(t *testing.T) {
	config := filepath.Join(t.TempDir(), ".claude.json")
	project := filepath.Join(t.TempDir(), "projects")
	if err := seedNativeClaudeTrust(config, []string{project}, false); err != nil {
		t.Fatal(err)
	}
	entry := projectEntry(t, config, project)
	if string(entry["hasTrustDialogAccepted"]) != "true" || entry["hasClaudeMdExternalIncludesApproved"] != nil || entry["hasClaudeMdExternalIncludesWarningShown"] != nil {
		t.Fatalf("trust should be seeded and the external approval left off: %v", entry)
	}
	if err := seedNativeClaudeTrust(config, []string{project}, true); err != nil {
		t.Fatal(err)
	}
	if entry := projectEntry(t, config, project); string(entry["hasClaudeMdExternalIncludesApproved"]) != "true" {
		t.Fatalf("asked, the external approval should be set: %v", entry)
	}
}

func projectEntry(t *testing.T, config, project string) map[string]json.RawMessage {
	t.Helper()
	top, err := readNativeClaudeJSON(config)
	if err != nil {
		t.Fatal(err)
	}
	var entries map[string]map[string]json.RawMessage
	if err := json.Unmarshal(top["projects"], &entries); err != nil {
		t.Fatal(err)
	}
	return entries[project]
}

// Another claude rewriting the shared config at the wrong instant erased the seed with no
// error, which left five seats at the imports dialog (COI-3305).
func TestAnErasedSeedIsRepairedByTheReadBack(t *testing.T) {
	saved := nativeClaudeSeedChecks
	nativeClaudeSeedChecks = []time.Duration{40 * time.Millisecond, 40 * time.Millisecond}
	t.Cleanup(func() { nativeClaudeSeedChecks = saved })
	config := filepath.Join(t.TempDir(), ".claude.json")
	project := filepath.Join(t.TempDir(), "projects")
	var erased sync.WaitGroup
	erased.Add(1)
	go func() {
		defer erased.Done()
		time.Sleep(15 * time.Millisecond)
		// A claude whose snapshot predates the seed writes the whole file back without it.
		if err := writeNativeClaudeJSON(config, map[string]json.RawMessage{"projects": json.RawMessage("{}")}); err != nil {
			t.Error(err)
		}
	}()
	if _, err := ensureNativeClaudeTrust(config, []string{project}, true, nativeClaudeSeedChecks); err != nil {
		t.Fatal(err)
	}
	erased.Wait()
	entry := projectEntry(t, config, project)
	if string(entry["hasTrustDialogAccepted"]) != "true" || string(entry["hasClaudeMdExternalIncludesApproved"]) != "true" {
		t.Fatalf("the erased seed should have been written again: %v", entry)
	}
}
