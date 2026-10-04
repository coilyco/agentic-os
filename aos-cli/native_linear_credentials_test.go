package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

type linearEnvFake struct {
	env map[string]string
}

func (f *linearEnvFake) lookup(key string) (string, bool) {
	value, ok := f.env[key]
	return value, ok
}

func (f *linearEnvFake) set(key, value string) error {
	f.env[key] = value
	return nil
}

func TestApplyLinearEnvironmentFetchesAndExportsBoth(t *testing.T) {
	t.Parallel()
	fake := &linearEnvFake{env: map[string]string{}}
	var stderr bytes.Buffer
	applyLinearEnvironment(context.Background(), &stderr, fake.lookup, fake.set,
		func(context.Context) (string, error) { return "lin_key\n", nil })
	if fake.env["LINEAR_API_KEY"] != "lin_key" {
		t.Fatalf("LINEAR_API_KEY = %q", fake.env["LINEAR_API_KEY"])
	}
	if fake.env["LINEAR_MCP_AUTHORIZATION"] != "Bearer lin_key" {
		t.Fatalf("LINEAR_MCP_AUTHORIZATION = %q", fake.env["LINEAR_MCP_AUTHORIZATION"])
	}
	if stderr.Len() != 0 {
		t.Fatalf("a clean fetch wrote to stderr: %q", stderr.String())
	}
}

func TestApplyLinearEnvironmentKeepsAmbientAuthorization(t *testing.T) {
	t.Parallel()
	fake := &linearEnvFake{env: map[string]string{"LINEAR_MCP_AUTHORIZATION": "Bearer host"}}
	applyLinearEnvironment(context.Background(), &bytes.Buffer{}, fake.lookup, fake.set,
		func(context.Context) (string, error) { return "", errors.New("must not fetch") })
	if fake.env["LINEAR_MCP_AUTHORIZATION"] != "Bearer host" {
		t.Fatalf("ambient value replaced: %q", fake.env["LINEAR_MCP_AUTHORIZATION"])
	}
	if _, set := fake.env["LINEAR_API_KEY"]; set {
		t.Fatal("ambient authorization still exported LINEAR_API_KEY")
	}
}

func TestApplyLinearEnvironmentDerivesFromAmbientKey(t *testing.T) {
	t.Parallel()
	fake := &linearEnvFake{env: map[string]string{"LINEAR_API_KEY": "ambient"}}
	applyLinearEnvironment(context.Background(), &bytes.Buffer{}, fake.lookup, fake.set,
		func(context.Context) (string, error) { return "", errors.New("must not fetch") })
	if fake.env["LINEAR_MCP_AUTHORIZATION"] != "Bearer ambient" {
		t.Fatalf("LINEAR_MCP_AUTHORIZATION = %q", fake.env["LINEAR_MCP_AUTHORIZATION"])
	}
}

func TestApplyLinearEnvironmentFailsOpenWithoutLeakingTheValue(t *testing.T) {
	t.Parallel()
	for name, fetch := range map[string]linearKeyFetch{
		"error": func(context.Context) (string, error) {
			return "must-not-appear", errors.New("aws ssm get-parameter: exit status 255")
		},
		"empty": func(context.Context) (string, error) { return "\n", nil },
		"none":  func(context.Context) (string, error) { return "None\n", nil },
	} {
		fetch := fetch
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake := &linearEnvFake{env: map[string]string{}}
			var stderr bytes.Buffer
			applyLinearEnvironment(context.Background(), &stderr, fake.lookup, fake.set, fetch)
			if len(fake.env) != 0 {
				t.Fatalf("a failed fetch exported %v", fake.env)
			}
			if !strings.Contains(stderr.String(), "/coilysiren/linear/key") {
				t.Fatalf("warning does not name the parameter: %q", stderr.String())
			}
			if strings.Contains(stderr.String(), "must-not-appear") {
				t.Fatalf("warning leaked the value: %q", stderr.String())
			}
		})
	}
}
