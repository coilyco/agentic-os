package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const quarantineAttribute = "com.apple.quarantine"

// runXattr is swapped by tests, so none of them touches a real file's attributes.
var runXattr = func(args ...string) error {
	return exec.Command("xattr", args...).Run()
}

// clearHarnessQuarantine drops the download quarantine from a cask harness before it
// runs, and returns the install it cleared. Why: docs/native-harness-config.md.
func clearHarnessQuarantine(harness, caskroom string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", nil
	}
	binary, err := exec.LookPath(harness)
	if err != nil {
		return "", nil
	}
	real, err := filepath.EvalSymlinks(binary)
	if err != nil {
		return "", nil
	}
	install, ok := caskInstall(real, caskroom)
	if !ok {
		return "", nil
	}
	if runXattr("-p", quarantineAttribute, real) != nil {
		return "", nil
	}
	if err := runXattr("-dr", quarantineAttribute, install); err != nil {
		return "", fmt.Errorf("clear the quarantine on %s: %w", install, err)
	}
	return install, nil
}

// caskInstall is the Caskroom/<name>/<version> directory a binary sits in.
func caskInstall(path, caskroom string) (string, bool) {
	relative, err := filepath.Rel(caskroom, path)
	if err != nil || strings.HasPrefix(relative, "..") {
		return "", false
	}
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) < 3 {
		return "", false
	}
	return filepath.Join(caskroom, parts[0], parts[1]), true
}

func homebrewCaskroom() string {
	prefix := os.Getenv("HOMEBREW_PREFIX")
	if prefix == "" {
		prefix = "/opt/homebrew"
	}
	return filepath.Join(prefix, "Caskroom")
}
