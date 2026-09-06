//go:build darwin

package main

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

func readClaudeKeyring(ctx context.Context, service, account string) ([]byte, error) {
	command := exec.CommandContext(
		ctx,
		"/usr/bin/security",
		"find-generic-password",
		"-s", service,
		"-a", account,
		"-w",
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if claudeKeyringMissing(err, stderr.Bytes()) {
			return nil, errClaudeKeyringNotFound
		}
		return nil, err
	}
	return bytes.TrimSpace(stdout.Bytes()), nil
}

func claudeKeyringMissing(err error, stderr []byte) bool {
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return false
	}
	return bytes.Contains(stderr, []byte("-25300")) ||
		bytes.Contains(stderr, []byte("could not be found"))
}

// The secret never reaches stderr, so the message is safe to surface.
func claudeKeyringFailure(err error, stderr []byte) string {
	message := string(bytes.TrimSpace(stderr))
	if message == "" {
		return err.Error()
	}
	return message
}
