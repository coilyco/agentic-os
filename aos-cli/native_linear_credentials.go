package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	linearAppParameterRoot    = "/coilysiren/linear/apps/"
	linearAppScope            = "read,write"
)

// linearTokenEndpoint is a variable so a test can point it at a local server.
var linearTokenEndpoint = "https://api.linear.app/oauth/token"

type linearAppCredentials struct {
	ClientID     string
	ClientSecret string
}

// linearAppFetch reports found=false for a role with no Linear app, which is
// the normal state until its app exists.
type linearAppFetch func(ctx context.Context, role string) (creds linearAppCredentials, found bool, err error)

type linearAppMint func(ctx context.Context, creds linearAppCredentials) (string, error)

type linearKeyFetch func(context.Context) (string, error)

// applyNativeLinearEnvironment exports the Linear key for public_coilyco_linear.
// A failed read never blocks the launch. See docs/aos-auth.md.
func applyNativeLinearEnvironment(ctx context.Context, stderr io.Writer, role string) {
	ambient, _ := os.LookupEnv(linearMCPAuthorizationEnv)
	applyLinearEnvironment(ctx, stderr, os.LookupEnv, os.Setenv, fetchLinearKey)
	if strings.TrimSpace(ambient) != "" || role == "" {
		return
	}
	applyLinearRoleApp(ctx, stderr, role, os.Setenv, fetchLinearAppCredentials, mintLinearAppToken)
}

// applyLinearRoleApp swaps the MCP authorization for the role's app token, so its
// comments show the app. A failure keeps Kai's key and warns. See docs/aos-auth.md.
func applyLinearRoleApp(
	ctx context.Context,
	stderr io.Writer,
	role string,
	set func(key, value string) error,
	fetch linearAppFetch,
	mint linearAppMint,
) {
	creds, found, err := fetch(ctx, role)
	if err != nil {
		fmt.Fprintf(stderr, "aos: warning: read the Linear app for %s: %v. Its Linear comments will post as Kai\n", role, err)
		return
	}
	if !found {
		return
	}
	token, err := mint(ctx, creds)
	if err != nil {
		fmt.Fprintf(stderr, "aos: warning: mint the Linear app token for %s: %v. Its Linear comments will post as Kai\n", role, err)
		return
	}
	if err := set(linearMCPAuthorizationEnv, "Bearer "+token); err != nil {
		fmt.Fprintf(stderr, "aos: warning: set %s: %v\n", linearMCPAuthorizationEnv, err)
	}
}

func fetchLinearAppCredentials(ctx context.Context, role string) (linearAppCredentials, bool, error) {
	awsPath, err := exec.LookPath("aws")
	if err != nil {
		return linearAppCredentials{}, false, fmt.Errorf("find AWS CLI: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, linearKeyFetchTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, awsPath, "ssm", "get-parameters-by-path",
		"--path", linearAppParameterRoot+role, "--with-decryption",
		"--query", "Parameters[].[Name,Value]", "--output", "text")
	var output bytes.Buffer
	command.Stdout = &output
	if err := command.Run(); err != nil {
		return linearAppCredentials{}, false, fmt.Errorf("aws ssm get-parameters-by-path: %w", err)
	}
	return parseLinearAppCredentials(role, output.String())
}

// parseLinearAppCredentials reads "name<TAB>value" lines. Half a pair is an error,
// not a quiet fallback.
func parseLinearAppCredentials(role, listing string) (linearAppCredentials, bool, error) {
	var creds linearAppCredentials
	for _, line := range strings.Split(listing, "\n") {
		name, value, ok := strings.Cut(strings.TrimRight(line, "\r"), "\t")
		if !ok {
			continue
		}
		switch name {
		case linearAppParameterRoot + role + "/client-id":
			creds.ClientID = strings.TrimSpace(value)
		case linearAppParameterRoot + role + "/client-secret":
			creds.ClientSecret = strings.TrimSpace(value)
		}
	}
	switch {
	case creds.ClientID == "" && creds.ClientSecret == "":
		return linearAppCredentials{}, false, nil
	case creds.ClientID == "" || creds.ClientSecret == "":
		return linearAppCredentials{}, false, fmt.Errorf("%s%s needs both client-id and client-secret", linearAppParameterRoot, role)
	}
	return creds, true, nil
}

// mintLinearAppToken runs Linear's client_credentials grant (30-day app token).
// The secret travels in the body, never argv.
func mintLinearAppToken(ctx context.Context, creds linearAppCredentials) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, linearKeyFetchTimeout)
	defer cancel()
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"scope":         {linearAppScope},
		"client_id":     {creds.ClientID},
		"client_secret": {creds.ClientSecret},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, linearTokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		// The status only: an error body can echo the client id.
		return "", fmt.Errorf("token request answered %d", response.StatusCode)
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<16)).Decode(&body); err != nil || body.AccessToken == "" {
		return "", fmt.Errorf("token response had no access_token")
	}
	return body.AccessToken, nil
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
