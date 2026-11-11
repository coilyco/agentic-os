package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCaskInstallIsTheNameAndVersionDirectory(t *testing.T) {
	t.Parallel()
	room := "/opt/homebrew/Caskroom"
	got, ok := caskInstall(room+"/codex/0.167.1/bin/codex", room)
	if !ok || got != room+"/codex/0.167.1" {
		t.Fatalf("caskInstall = %q, %v", got, ok)
	}
	for _, outside := range []string{"/usr/local/bin/codex", room + "/codex", room + "/codex/0.167.1"} {
		if got, ok := caskInstall(outside, room); ok {
			t.Fatalf("caskInstall(%q) = %q, want no install", outside, got)
		}
	}
}

func TestAQuarantinedCaskBinaryIsClearedBeforeItRuns(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("quarantine is a macOS attribute")
	}
	room := t.TempDir()
	install := filepath.Join(room, "fakeharness", "1.0")
	if err := os.MkdirAll(filepath.Join(install, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(install, "bin", "fakeharness")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := t.TempDir()
	if err := os.Symlink(binary, filepath.Join(link, "fakeharness")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", link)
	var calls []string
	t.Cleanup(func(previous func(...string) error) func() { return func() { runXattr = previous } }(runXattr))
	runXattr = func(args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		return nil
	}
	got, err := clearHarnessQuarantine("fakeharness", room)
	if err != nil {
		t.Fatal(err)
	}
	resolved, _ := filepath.EvalSymlinks(install)
	if got != resolved {
		t.Fatalf("cleared %q, want %q", got, resolved)
	}
	if len(calls) != 2 || !strings.HasPrefix(calls[0], "-p "+quarantineAttribute) || !strings.HasPrefix(calls[1], "-dr "+quarantineAttribute+" ") {
		t.Fatalf("xattr calls = %v, want a read then a recursive delete", calls)
	}
}

func TestACaskBinaryWithNoQuarantineIsLeftAlone(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("quarantine is a macOS attribute")
	}
	room := t.TempDir()
	binary := filepath.Join(room, "fakeharness", "1.0", "bin", "fakeharness")
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(binary))
	deleted := false
	t.Cleanup(func(previous func(...string) error) func() { return func() { runXattr = previous } }(runXattr))
	runXattr = func(args ...string) error {
		if args[0] == "-dr" {
			deleted = true
		}
		return os.ErrNotExist
	}
	got, err := clearHarnessQuarantine("fakeharness", room)
	if err != nil || got != "" || deleted {
		t.Fatalf("got %q, %v, deleted %v, want nothing touched", got, err, deleted)
	}
}

func TestABinaryOutsideTheCaskroomIsNeverTouched(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("quarantine is a macOS attribute")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "elsewhere"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Cleanup(func(previous func(...string) error) func() { return func() { runXattr = previous } }(runXattr))
	runXattr = func(args ...string) error {
		t.Fatalf("xattr %v ran for a binary outside the Caskroom", args)
		return nil
	}
	got, err := clearHarnessQuarantine("elsewhere", t.TempDir())
	if err != nil || got != "" {
		t.Fatalf("got %q, %v", got, err)
	}
}
