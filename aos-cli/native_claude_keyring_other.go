//go:build !darwin

package main

import "context"

func readClaudeKeyring(context.Context, string, string) ([]byte, error) {
	return nil, errClaudeKeyringUnsupported
}
