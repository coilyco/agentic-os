package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

func appFetch(creds linearAppCredentials, found bool, err error) linearAppFetch {
	return func(context.Context, string) (linearAppCredentials, bool, error) { return creds, found, err }
}

func appMint(token string, err error) linearAppMint {
	return func(context.Context, linearAppCredentials) (string, error) { return token, err }
}

func TestApplyLinearRoleAppSwapsOnlyTheMCPAuthorization(t *testing.T) {
	t.Parallel()
	fake := &linearEnvFake{env: map[string]string{
		"LINEAR_API_KEY":           "kai",
		"LINEAR_MCP_AUTHORIZATION": "Bearer kai",
	}}
	var stderr bytes.Buffer
	applyLinearRoleApp(context.Background(), &stderr, "eng-platform", fake.set,
		appFetch(linearAppCredentials{ClientID: "id", ClientSecret: "secret"}, true, nil),
		appMint("app-token", nil))
	if fake.env["LINEAR_MCP_AUTHORIZATION"] != "Bearer app-token" {
		t.Fatalf("LINEAR_MCP_AUTHORIZATION = %q", fake.env["LINEAR_MCP_AUTHORIZATION"])
	}
	if fake.env["LINEAR_API_KEY"] != "kai" {
		t.Fatalf("LINEAR_API_KEY changed to %q", fake.env["LINEAR_API_KEY"])
	}
	if stderr.Len() != 0 {
		t.Fatalf("a clean swap wrote to stderr: %q", stderr.String())
	}
}

func TestApplyLinearRoleAppIsSilentWhenTheRoleHasNoApp(t *testing.T) {
	t.Parallel()
	fake := &linearEnvFake{env: map[string]string{"LINEAR_MCP_AUTHORIZATION": "Bearer kai"}}
	var stderr bytes.Buffer
	applyLinearRoleApp(context.Background(), &stderr, "psych", fake.set,
		appFetch(linearAppCredentials{}, false, nil),
		appMint("", errors.New("must not mint")))
	if fake.env["LINEAR_MCP_AUTHORIZATION"] != "Bearer kai" || stderr.Len() != 0 {
		t.Fatalf("env %v stderr %q", fake.env, stderr.String())
	}
}

func TestApplyLinearRoleAppKeepsKaiAndSaysSoWhenItFails(t *testing.T) {
	t.Parallel()
	for name, run := range map[string]func(*linearEnvFake, *bytes.Buffer){
		"fetch": func(f *linearEnvFake, w *bytes.Buffer) {
			applyLinearRoleApp(context.Background(), w, "eng-platform", f.set,
				appFetch(linearAppCredentials{}, false, errors.New("aws ssm get-parameters-by-path: exit status 255")),
				appMint("must-not-appear", nil))
		},
		"mint": func(f *linearEnvFake, w *bytes.Buffer) {
			applyLinearRoleApp(context.Background(), w, "eng-platform", f.set,
				appFetch(linearAppCredentials{ClientID: "id", ClientSecret: "must-not-appear"}, true, nil),
				appMint("", errors.New("token request answered 401")))
		},
	} {
		run := run
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake := &linearEnvFake{env: map[string]string{"LINEAR_MCP_AUTHORIZATION": "Bearer kai"}}
			var stderr bytes.Buffer
			run(fake, &stderr)
			if fake.env["LINEAR_MCP_AUTHORIZATION"] != "Bearer kai" {
				t.Fatalf("authorization changed to %q", fake.env["LINEAR_MCP_AUTHORIZATION"])
			}
			if !strings.Contains(stderr.String(), "eng-platform") || !strings.Contains(stderr.String(), "post as Kai") {
				t.Fatalf("warning does not name the role and the fallback: %q", stderr.String())
			}
			if strings.Contains(stderr.String(), "must-not-appear") {
				t.Fatalf("warning leaked a value: %q", stderr.String())
			}
		})
	}
}

func TestParseLinearAppCredentials(t *testing.T) {
	t.Parallel()
	root := linearAppParameterRoot + "eng-platform/"
	for name, tc := range map[string]struct {
		listing string
		want    linearAppCredentials
		found   bool
		fails   bool
	}{
		"both":    {root + "client-id\tcid\n" + root + "client-secret\tsec\n", linearAppCredentials{"cid", "sec"}, true, false},
		"none":    {"", linearAppCredentials{}, false, false},
		"other":   {linearAppParameterRoot + "scientist/client-id\tx\n", linearAppCredentials{}, false, false},
		"id only": {root + "client-id\tcid\n", linearAppCredentials{}, false, true},
	} {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, found, err := parseLinearAppCredentials("eng-platform", tc.listing)
			if (err != nil) != tc.fails || found != tc.found || got != tc.want {
				t.Fatalf("got %+v found=%v err=%v", got, found, err)
			}
		})
	}
}

func TestMintLinearAppTokenSendsTheGrantInTheBody(t *testing.T) {
	var form map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = map[string]string{}
		for _, key := range []string{"grant_type", "scope", "client_id", "client_secret"} {
			form[key] = r.PostForm.Get(key)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("credentials in the query: %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"access_token":"app-token","token_type":"Bearer","expires_in":2591999}`))
	}))
	defer server.Close()
	previous := linearTokenEndpoint
	linearTokenEndpoint = server.URL
	defer func() { linearTokenEndpoint = previous }()
	token, err := mintLinearAppToken(context.Background(), linearAppCredentials{ClientID: "cid", ClientSecret: "sec"})
	if err != nil || token != "app-token" {
		t.Fatalf("token %q err %v", token, err)
	}
	want := map[string]string{"grant_type": "client_credentials", "scope": "read,write", "client_id": "cid", "client_secret": "sec"}
	for key, value := range want {
		if form[key] != value {
			t.Fatalf("%s = %q, want %q", key, form[key], value)
		}
	}
}

func TestMintLinearAppTokenReportsOnlyTheStatus(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"rejected": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("client cid-in-body is unknown"))
		},
		"empty": func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) },
	} {
		handler := handler
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(handler)
			defer server.Close()
			previous := linearTokenEndpoint
			linearTokenEndpoint = server.URL
			defer func() { linearTokenEndpoint = previous }()
			_, err := mintLinearAppToken(context.Background(), linearAppCredentials{ClientID: "cid", ClientSecret: "sec"})
			if err == nil || strings.Contains(err.Error(), "cid-in-body") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
