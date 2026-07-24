// Package main launches the generated Aguard binary with its bundled bridge.
package main

import (
	"embed"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

//go:embed payload/aguard payload/agentic_os/*
var payload embed.FS

func writePayload(root string) error {
	return fs.WalkDir(payload, "payload", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel("payload", path)
		if err != nil || relative == "." {
			return err
		}
		destination := filepath.Join(root, relative)
		if relative == "aguard" && runtime.GOOS == "windows" {
			destination += ".exe"
		}
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		contents, err := payload.ReadFile(path)
		if err != nil {
			return err
		}
		mode := fs.FileMode(0o644)
		if relative == "aguard" {
			mode = 0o755
		}
		return os.WriteFile(destination, contents, mode)
	})
}

func main() {
	temporary, err := os.MkdirTemp("", "aguard-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(temporary)

	if err := writePayload(temporary); err != nil {
		panic(err)
	}

	binary := filepath.Join(temporary, "aguard")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	bridgeRoot := filepath.Join(temporary, "agentic_os_parent")
	if err := os.MkdirAll(bridgeRoot, 0o755); err != nil {
		panic(err)
	}
	if err := os.Rename(filepath.Join(temporary, "agentic_os"), filepath.Join(bridgeRoot, "agentic_os")); err != nil {
		panic(err)
	}
	command := exec.Command(binary, os.Args[1:]...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = append(os.Environ(), "PYTHONPATH="+bridgeRoot)
	if err := command.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		panic(err)
	}
}
