package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLiveCwdReadsAProcessWorkingDirectory(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command("sleep", "30")
	child.Dir = dir
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	got := liveCwd(child.Process.Pid)
	if got == "" {
		t.Skip("this host will not read another process's working directory")
	}
	if resolved, _ := filepath.EvalSymlinks(got); resolved != dir {
		t.Fatalf("liveCwd = %q, want %q", got, dir)
	}
	if liveCwd(0) != "" || liveCwd(-1) != "" {
		t.Fatal("no pid has no folder")
	}
}

func TestWithLiveCwdOnlyMovesACodexOrOpencodeSeat(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	child := exec.Command("sleep", "30")
	child.Dir = dir
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	if liveCwd(child.Process.Pid) == "" {
		t.Skip("this host will not read another process's working directory")
	}
	for _, seat := range []string{"codex", "opencode"} {
		moved := withLiveCwd(ledgerEntry{Seat: seat, Cwd: "/launch/folder"}, child.Process.Pid)
		if resolved, _ := filepath.EvalSymlinks(moved.Cwd); resolved != dir {
			t.Fatalf("%s kept the launch folder: %q", seat, moved.Cwd)
		}
	}
	if kept := withLiveCwd(ledgerEntry{Seat: "claude", Cwd: "/launch/folder"}, child.Process.Pid); kept.Cwd != "/launch/folder" {
		t.Fatalf("claude is found by conversation id, so its folder stays: %q", kept.Cwd)
	}
	if kept := withLiveCwd(ledgerEntry{Seat: "codex", Cwd: "/launch/folder"}, 0); kept.Cwd != "/launch/folder" {
		t.Fatalf("an unreadable pid keeps the record: %q", kept.Cwd)
	}
}
