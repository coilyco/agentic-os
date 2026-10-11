package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// The table here is a fixture, not the shipped one: the owning loader is the
// shipped table, and these tests cover what the applier does with any table.
var mcpCredentialFixture = []nativeMCPCredential{
	{Env: "A_AUTH", Parameter: "/p/a", Roles: []string{"director"}},
	{Env: "B_AUTH", Parameter: "/p/b", Roles: []string{"director", "engineer"}},
}

func applyFixture(t *testing.T, role string, env map[string]string, fetch mcpCredentialFetch) (map[string]string, string) {
	t.Helper()
	fake := &linearEnvFake{env: env}
	var stderr bytes.Buffer
	applyMCPCredentials(context.Background(), &stderr, role, mcpCredentialFixture, fake.lookup, fake.set, fetch)
	return fake.env, stderr.String()
}

func TestApplyMCPCredentialsExportsOnlyTheRolesEntries(t *testing.T) {
	t.Parallel()
	var fetched []string
	env, stderr := applyFixture(t, "engineer", map[string]string{}, func(_ context.Context, parameter string) (string, error) {
		fetched = append(fetched, parameter)
		return "tok\n", nil
	})
	if env["B_AUTH"] != "Bearer tok" {
		t.Fatalf("B_AUTH = %q", env["B_AUTH"])
	}
	if _, set := env["A_AUTH"]; set {
		t.Fatal("a role not listed for A_AUTH received it")
	}
	if len(fetched) != 1 || fetched[0] != "/p/b" {
		t.Fatalf("an unlisted entry was read from SSM: %v", fetched)
	}
	if stderr != "" {
		t.Fatalf("a clean fetch wrote to stderr: %q", stderr)
	}
}

func TestApplyMCPCredentialsReadsNothingForAnEmptyOrUnlistedRole(t *testing.T) {
	t.Parallel()
	for _, role := range []string{"", "scientist"} {
		env, _ := applyFixture(t, role, map[string]string{}, func(context.Context, string) (string, error) {
			return "", errors.New("must not fetch")
		})
		if len(env) != 0 {
			t.Fatalf("role %q exported %v", role, env)
		}
	}
}

func TestApplyMCPCredentialsKeepsAmbientValues(t *testing.T) {
	t.Parallel()
	env, _ := applyFixture(t, "director", map[string]string{"A_AUTH": "Bearer host"}, func(_ context.Context, parameter string) (string, error) {
		if parameter == "/p/a" {
			return "", errors.New("must not fetch an ambient entry")
		}
		return "tok", nil
	})
	if env["A_AUTH"] != "Bearer host" {
		t.Fatalf("ambient value replaced: %q", env["A_AUTH"])
	}
	if env["B_AUTH"] != "Bearer tok" {
		t.Fatalf("the other entry was skipped: %q", env["B_AUTH"])
	}
}

func TestApplyMCPCredentialsFailsOpenWithoutLeakingTheValue(t *testing.T) {
	t.Parallel()
	for name, fetch := range map[string]mcpCredentialFetch{
		"error": func(context.Context, string) (string, error) {
			return "", errors.New("aws ssm get-parameter: exit 254")
		},
		"empty": func(context.Context, string) (string, error) { return "  \n", nil },
		"none":  func(context.Context, string) (string, error) { return "None", nil },
	} {
		env, stderr := applyFixture(t, "director", map[string]string{}, fetch)
		if len(env) != 0 {
			t.Fatalf("%s: exported %v", name, env)
		}
		if !strings.Contains(stderr, "/p/a") || !strings.Contains(stderr, "/p/b") {
			t.Fatalf("%s: warning did not name both parameters: %q", name, stderr)
		}
	}
}

func TestApplyMCPCredentialsKeepsGoingPastAFailedEntry(t *testing.T) {
	t.Parallel()
	env, stderr := applyFixture(t, "director", map[string]string{}, func(_ context.Context, parameter string) (string, error) {
		if parameter == "/p/a" {
			return "", errors.New("not found")
		}
		return "tok", nil
	})
	if env["B_AUTH"] != "Bearer tok" {
		t.Fatalf("a failed first entry blocked the second: %v", env)
	}
	if strings.Contains(stderr, "tok") {
		t.Fatalf("the token reached stderr: %q", stderr)
	}
}
