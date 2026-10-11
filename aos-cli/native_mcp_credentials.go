package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

const mcpCredentialFetchTimeout = 15 * time.Second

// nativeMCPCredential feeds one inventory entry's Authorization header, and only
// the listed Roles read it, so no other seat touches the key. See docs/aos-auth.md.
type nativeMCPCredential struct {
	Env       string
	Parameter string
	Roles     []string
}

var nativeMCPCredentials = []nativeMCPCredential{
	{Env: "SLACK_MCP_AUTHORIZATION", Parameter: "/coilysiren/slack/mcp-user-token", Roles: []string{"prod-director"}},
	{Env: "STRIPE_MCP_AUTHORIZATION", Parameter: "/coilysiren/stripe/agent-key", Roles: []string{"prod-director"}},
}

type mcpCredentialFetch func(ctx context.Context, parameter string) (string, error)

// applyNativeMCPCredentials exports each credential the role is listed for. A
// failed read never blocks the launch. See docs/aos-auth.md.
func applyNativeMCPCredentials(ctx context.Context, stderr io.Writer, role string) {
	applyMCPCredentials(ctx, stderr, role, nativeMCPCredentials, os.LookupEnv, os.Setenv, fetchMCPCredential)
}

func applyMCPCredentials(
	ctx context.Context,
	stderr io.Writer,
	role string,
	table []nativeMCPCredential,
	lookup environmentLookup,
	set func(key, value string) error,
	fetch mcpCredentialFetch,
) {
	if role == "" {
		return
	}
	for _, credential := range table {
		if !slices.Contains(credential.Roles, role) {
			continue
		}
		if value, ok := lookup(credential.Env); ok && strings.TrimSpace(value) != "" {
			continue
		}
		token, err := fetch(ctx, credential.Parameter)
		if err != nil {
			fmt.Fprintf(stderr, "aos: warning: read %s for %s: %v\n", credential.Parameter, credential.Env, err)
			continue
		}
		token = strings.TrimSpace(token)
		if token == "" || token == "None" {
			fmt.Fprintf(stderr, "aos: warning: %s is empty, so the MCP entry reading %s lists no tools\n", credential.Parameter, credential.Env)
			continue
		}
		// Codex sends an env_http_headers variable verbatim, so it holds the scheme too.
		if err := set(credential.Env, "Bearer "+token); err != nil {
			fmt.Fprintf(stderr, "aos: warning: set %s: %v\n", credential.Env, err)
		}
	}
}

func fetchMCPCredential(ctx context.Context, parameter string) (string, error) {
	awsPath, err := exec.LookPath("aws")
	if err != nil {
		return "", fmt.Errorf("find AWS CLI: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, mcpCredentialFetchTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, awsPath, "ssm", "get-parameter", "--name", parameter,
		"--with-decryption", "--query", "Parameter.Value", "--output", "text")
	var output bytes.Buffer
	command.Stdout = &output
	// Stderr stays unattached so an AWS error body cannot reach the seat's terminal.
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("aws ssm get-parameter: %w", err)
	}
	return output.String(), nil
}
