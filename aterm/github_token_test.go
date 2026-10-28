package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGithub stands in for the two App endpoints the broker calls, and checks
// the JWT signature against the key under test.
type fakeGithub struct {
	t        *testing.T
	pub      *rsa.PublicKey
	mu       sync.Mutex
	mints    int
	lastBody map[string]any
	expires  time.Time
	refuse   bool
}

func (f *fakeGithub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.t.Helper()
	if !f.verified(r.Header.Get("Authorization")) {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/repos/coilyco/bluesky-mcp/installation":
		if f.refuse {
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"id":42}`))
	case r.Method == http.MethodPost && r.URL.Path == "/app/installations/42/access_tokens":
		f.mu.Lock()
		f.mints++
		_ = json.NewDecoder(r.Body).Decode(&f.lastBody)
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs_fake", "expires_at": f.expires})
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeGithub) verified(header string) bool {
	jwt, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return false
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(f.pub, crypto.SHA256, digest[:], signature) != nil {
		return false
	}
	claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var decoded struct {
		Iss string `json:"iss"`
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
	}
	_ = json.Unmarshal(claims, &decoded)
	return decoded.Iss == "12345" && decoded.Exp-decoded.Iat <= 600 && decoded.Exp > decoded.Iat
}

func testApp(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	block := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return key, base64.StdEncoding.EncodeToString(block)
}

func testBroker(t *testing.T, fake *fakeGithub, key *rsa.PrivateKey, now *time.Time) *githubBroker {
	t.Helper()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return &githubBroker{
		api:    server.URL,
		client: server.Client(),
		creds: func(context.Context) (githubCredentials, error) {
			return githubCredentials{appID: "12345", key: key}, nil
		},
		now:   func() time.Time { return *now },
		perms: defaultGithubPermissions,
		cache: map[string]cachedGithubToken{},
	}
}

func TestBrokerMintsOneRepoWithTheNarrowPermissions(t *testing.T) {
	key, _ := testApp(t)
	now := time.Now()
	fake := &fakeGithub{t: t, pub: &key.PublicKey, expires: now.Add(time.Hour)}
	broker := testBroker(t, fake, key, &now)

	token, expires, err := broker.token(t.Context(), "coilyco/bluesky-mcp")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if token != "ghs_fake" || expires.Sub(fake.expires).Abs() > time.Second {
		t.Fatalf("token = %q expires %v", token, expires)
	}
	repos, _ := fake.lastBody["repositories"].([]any)
	if len(repos) != 1 || repos[0] != "bluesky-mcp" {
		t.Fatalf("repositories = %v, want exactly [bluesky-mcp]", fake.lastBody["repositories"])
	}
	perms, _ := fake.lastBody["permissions"].(map[string]any)
	if len(perms) != 3 || perms["contents"] != "write" || perms["pull_requests"] != "write" || perms["metadata"] != "read" {
		t.Fatalf("permissions = %v, want the three the default names", perms)
	}
}

func TestBrokerRefusesAnotherOwnerAndMalformedSpecsBeforeAnyCall(t *testing.T) {
	key, _ := testApp(t)
	now := time.Now()
	fake := &fakeGithub{t: t, pub: &key.PublicKey, expires: now.Add(time.Hour)}
	broker := testBroker(t, fake, key, &now)
	broker.creds = func(context.Context) (githubCredentials, error) {
		t.Fatal("credentials were read for a repo the broker should refuse")
		return githubCredentials{}, nil
	}
	for _, spec := range []string{"", "bluesky-mcp", "coilysiren/website", "other/x", "coilyco/", "coilyco/a/b", "coilyco/a b"} {
		if _, _, err := broker.token(t.Context(), spec); err == nil {
			t.Fatalf("spec %q minted", spec)
		}
	}
}

func TestBrokerReusesATokenUntilNearExpiryThenMintsAgain(t *testing.T) {
	key, _ := testApp(t)
	now := time.Now()
	fake := &fakeGithub{t: t, pub: &key.PublicKey, expires: now.Add(time.Hour)}
	broker := testBroker(t, fake, key, &now)

	for range 3 {
		if _, _, err := broker.token(t.Context(), "coilyco/bluesky-mcp"); err != nil {
			t.Fatalf("mint: %v", err)
		}
	}
	if fake.mints != 1 {
		t.Fatalf("mints = %d, want the cache to serve the repeats", fake.mints)
	}
	now = now.Add(56 * time.Minute)
	if _, _, err := broker.token(t.Context(), "coilyco/bluesky-mcp"); err != nil {
		t.Fatalf("remint: %v", err)
	}
	if fake.mints != 2 {
		t.Fatalf("mints = %d, want a fresh one inside the %v reuse margin", fake.mints, tokenReuse)
	}
}

func TestBrokerReportsGithubsRefusalWithoutAToken(t *testing.T) {
	key, _ := testApp(t)
	now := time.Now()
	fake := &fakeGithub{t: t, pub: &key.PublicKey, expires: now.Add(time.Hour), refuse: true}
	broker := testBroker(t, fake, key, &now)

	_, _, err := broker.token(t.Context(), "coilyco/bluesky-mcp")
	if err == nil || !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "installation") {
		t.Fatalf("err = %v, want the 404 on the installation lookup", err)
	}
}

func TestGithubPermissionsParseOrFailLoudly(t *testing.T) {
	got, err := parseGithubPermissions("actions=write, contents=read")
	if err != nil || got["actions"] != "write" || got["contents"] != "read" || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	if got, _ := parseGithubPermissions(""); len(got) != len(defaultGithubPermissions) {
		t.Fatalf("empty should be the default, got %v", got)
	}
	for _, bad := range []string{"actions", "actions=admin", "=read"} {
		if _, err := parseGithubPermissions(bad); err == nil {
			t.Fatalf("%q parsed", bad)
		}
	}
}

func TestADaemonSeatGetsATokenAndNothingElseDoes(t *testing.T) {
	key, encoded := testApp(t)
	fake := &fakeGithub{t: t, pub: &key.PublicKey, expires: time.Now().Add(time.Hour)}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	t.Setenv(githubAPIEnv, server.URL)
	t.Setenv(githubAppIDEnv, "12345")
	t.Setenv(githubAppKeyEnv, encoded)
	testDaemon(t)
	seat := dialTest(t)
	seat.spawn("eng-platform-beetle-ox", "eng-platform", "Beetle-Ox", `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 30`)
	seatToken := seat.token()

	t.Setenv(sessionTokenEnv, seatToken)
	token, err := requestGithubToken("coilyco/bluesky-mcp")
	if err != nil || token != "ghs_fake" {
		t.Fatalf("seat token = %q, %v", token, err)
	}
	if _, err := requestGithubToken("coilysiren/website"); err == nil || !strings.Contains(err.Error(), "mints only for") {
		t.Fatalf("another owner should refuse: %v", err)
	}

	t.Setenv(sessionTokenEnv, "")
	if _, err := requestGithubToken("coilyco/bluesky-mcp"); err == nil || !strings.Contains(err.Error(), "is unset") {
		t.Fatalf("no token should say so: %v", err)
	}
	t.Setenv(sessionTokenEnv, "not-a-token")
	if _, err := requestGithubToken("coilyco/bluesky-mcp"); err == nil || !strings.Contains(err.Error(), "names no live session") {
		t.Fatalf("a token naming nothing should refuse: %v", err)
	}
}
