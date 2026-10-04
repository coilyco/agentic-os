package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	linearKeyParameter        = "/coilysiren/linear/key"
	linearAPIKeyEnv           = "LINEAR_API_KEY"
	linearMCPAuthorizationEnv = "LINEAR_MCP_AUTHORIZATION"
	linearKeyFetchTimeout     = 15 * time.Second
)

type linearKeyFetch func(context.Context) (string, error)

// applyNativeLinearEnvironment exports the Linear key for public_coilyco_linear.
// A failed read never blocks the launch. See docs/aos-auth.md.
func applyNativeLinearEnvironment(ctx context.Context, stderr io.Writer) {
	applyLinearEnvironment(ctx, stderr, os.LookupEnv, os.Setenv, fetchLinearKey)
}

func applyLinearEnvironment(
	ctx context.Context,
	stderr io.Writer,
	lookup environmentLookup,
	set func(key, value string) error,
	fetch linearKeyFetch,
) {
	if value, ok := lookup(linearMCPAuthorizationEnv); ok && strings.TrimSpace(value) != "" {
		return
	}
	key, ok := lookup(linearAPIKeyEnv)
	key = strings.TrimSpace(key)
	if !ok || key == "" {
		fetched, err := fetch(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "aos: warning: read %s for the Linear MCP: %v\n", linearKeyParameter, err)
			return
		}
		key = strings.TrimSpace(fetched)
		if key == "" || key == "None" {
			fmt.Fprintf(stderr, "aos: warning: %s is empty, so the Linear MCP lists no tools\n", linearKeyParameter)
			return
		}
		if err := set(linearAPIKeyEnv, key); err != nil {
			fmt.Fprintf(stderr, "aos: warning: set %s: %v\n", linearAPIKeyEnv, err)
			return
		}
	}
	// Codex sends an env_http_headers variable verbatim, so it holds the whole value.
	if err := set(linearMCPAuthorizationEnv, "Bearer "+key); err != nil {
		fmt.Fprintf(stderr, "aos: warning: set %s: %v\n", linearMCPAuthorizationEnv, err)
	}
}

func fetchLinearKey(ctx context.Context) (string, error) {
	awsPath, err := exec.LookPath("aws")
	if err != nil {
		return "", fmt.Errorf("find AWS CLI: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, linearKeyFetchTimeout)
	defer cancel()
	command := exec.CommandContext(
		ctx,
		awsPath,
		"ssm",
		"get-parameter",
		"--name",
		linearKeyParameter,
		"--with-decryption",
		"--query",
		"Parameter.Value",
		"--output",
		"text",
	)
	var output bytes.Buffer
	command.Stdout = &output
	// Stderr stays unattached so an AWS error body cannot reach the seat's terminal.
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("aws ssm get-parameter: %w", err)
	}
	return output.String(), nil
}
