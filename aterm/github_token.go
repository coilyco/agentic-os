package main

import (
	"bytes"
	"cmp"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/urfave/cli/v3"
)

// githubTokenFeature is how a client knows the daemon answers a github_token
// frame. See tooling-aosguard references/gh-replacement.md.
const githubTokenFeature = "github-token"

const (
	githubAPI = "https://api.github.com"
	// githubOwner is the only owner the broker mints for: the App is installed
	// there, and any other owner is refused rather than forwarded.
	githubOwner = "coilyco"
	// tokenReuse is how long before expiry a cached token stops being handed out.
	tokenReuse = 5 * time.Minute

	githubAppIDEnv      = "GITHUB_APP_ID"
	githubAppKeyEnv     = "GITHUB_APP_PRIVATE_KEY_BASE64"
	githubAppIDParam    = "/github/coilyco-ops-app-id"
	githubAppKeyParam   = "/github/coilyco-ops-private-key-base64"
	githubPermissionEnv = "ATERM_GITHUB_PERMISSIONS"
	// githubAPIEnv points the broker at a stand-in API, which the tests need.
	githubAPIEnv = "ATERM_GITHUB_API"
)

// defaultGithubPermissions matches infrastructure's github_app_token.py. GitHub
// refuses a mint asking for more than the installation holds, so widen by env.
var defaultGithubPermissions = map[string]string{
	"contents":      "write",
	"pull_requests": "write",
	"metadata":      "read",
}

var repoSpec = regexp.MustCompile(`^([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)$`)

// parseGithubRepo splits owner/name and refuses anything outside githubOwner.
func parseGithubRepo(spec string) (owner, name string, err error) {
	found := repoSpec.FindStringSubmatch(strings.TrimSpace(spec))
	if found == nil {
		return "", "", fmt.Errorf("%q is not OWNER/REPO", spec)
	}
	if found[1] != githubOwner {
		return "", "", fmt.Errorf("the broker mints only for %s, not %q", githubOwner, found[1])
	}
	return found[1], found[2], nil
}

// parseGithubPermissions reads "a=read,b=write", falling back to the default.
func parseGithubPermissions(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultGithubPermissions, nil
	}
	out := map[string]string{}
	for _, pair := range strings.Split(raw, ",") {
		key, level, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok || key == "" || !slices.Contains([]string{"read", "write"}, level) {
			return nil, fmt.Errorf("%s: %q is not name=read or name=write", githubPermissionEnv, pair)
		}
		out[key] = level
	}
	return out, nil
}

type githubCredentials struct {
	appID string
	key   *rsa.PrivateKey
}

type cachedGithubToken struct {
	token   string
	expires time.Time
}

// githubBroker mints one installation token per repo. creds is a field so a test
// supplies a key without AWS.
type githubBroker struct {
	api    string
	client *http.Client
	creds  func(context.Context) (githubCredentials, error)
	now    func() time.Time
	perms  map[string]string

	mu    sync.Mutex
	cache map[string]cachedGithubToken
}

func newGithubBroker() (*githubBroker, error) {
	perms, err := parseGithubPermissions(os.Getenv(githubPermissionEnv))
	if err != nil {
		return nil, err
	}
	return &githubBroker{
		api:    cmp.Or(strings.TrimRight(os.Getenv(githubAPIEnv), "/"), githubAPI),
		client: &http.Client{Timeout: 30 * time.Second},
		creds:  loadGithubCredentials,
		now:    time.Now,
		perms:  perms,
		cache:  map[string]cachedGithubToken{},
	}, nil
}

// loadGithubCredentials reads the App id and key from the daemon's environment
// first, then SSM through the daemon's own AWS access. A seat never gets here.
func loadGithubCredentials(ctx context.Context) (githubCredentials, error) {
	id := strings.TrimSpace(os.Getenv(githubAppIDEnv))
	if id == "" {
		value, err := ssmValue(ctx, githubAppIDParam)
		if err != nil {
			return githubCredentials{}, err
		}
		id = value
	}
	encoded := strings.TrimSpace(os.Getenv(githubAppKeyEnv))
	if encoded == "" {
		value, err := ssmValue(ctx, githubAppKeyParam)
		if err != nil {
			return githubCredentials{}, err
		}
		encoded = value
	}
	key, err := parseGithubKey(encoded)
	if err != nil {
		return githubCredentials{}, err
	}
	return githubCredentials{appID: id, key: key}, nil
}

func ssmValue(ctx context.Context, name string) (string, error) {
	out, err := exec.CommandContext(ctx, "aws", "ssm", "get-parameter", "--with-decryption",
		"--output", "text", "--query", "Parameter.Value", "--name", name).Output()
	if err != nil {
		// The exit status alone: stderr could echo a parameter value.
		return "", fmt.Errorf("could not read %s from SSM: %w", name, errors.New(exitText(err)))
	}
	return strings.TrimSpace(string(out)), nil
}

func exitText(err error) string {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return fmt.Sprintf("aws exited %d", exit.ExitCode())
	}
	return "aws did not run"
}

func parseGithubKey(encoded string) (*rsa.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, errors.New("the App key is not base64")
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("the App key holds no PEM block")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("the App key is neither PKCS1 nor PKCS8")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("the App key is not RSA")
	}
	return key, nil
}

// appJWT is the RS256 token GitHub takes from an App. It is back-dated a minute
// for clock skew and lives nine, inside the ten-minute cap.
func appJWT(creds githubCredentials, now time.Time) (string, error) {
	segment := func(v any) string {
		encoded, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(encoded)
	}
	signing := segment(map[string]string{"alg": "RS256", "typ": "JWT"}) + "." + segment(map[string]any{
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": creds.appID,
	})
	digest := sha256.Sum256([]byte(signing))
	signature, err := rsa.SignPKCS1v15(rand.Reader, creds.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// token returns a single-repo installation token, reusing one until it is near
// expiry. The returned time is when GitHub will stop honoring it.
func (b *githubBroker) token(ctx context.Context, spec string) (string, time.Time, error) {
	owner, name, err := parseGithubRepo(spec)
	if err != nil {
		return "", time.Time{}, err
	}
	repo := owner + "/" + name
	b.mu.Lock()
	defer b.mu.Unlock()
	if hit, ok := b.cache[repo]; ok && b.now().Add(tokenReuse).Before(hit.expires) {
		return hit.token, hit.expires, nil
	}
	creds, err := b.creds(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	jwt, err := appJWT(creds, b.now())
	if err != nil {
		return "", time.Time{}, err
	}
	var installation struct {
		ID int64 `json:"id"`
	}
	if err := b.call(ctx, http.MethodGet, "/repos/"+repo+"/installation", jwt, nil, &installation); err != nil {
		return "", time.Time{}, fmt.Errorf("find the App installation for %s: %w", repo, err)
	}
	var minted struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	body := map[string]any{"repositories": []string{name}, "permissions": b.perms}
	path := fmt.Sprintf("/app/installations/%d/access_tokens", installation.ID)
	if err := b.call(ctx, http.MethodPost, path, jwt, body, &minted); err != nil {
		return "", time.Time{}, fmt.Errorf("mint a token for %s: %w", repo, err)
	}
	if minted.Token == "" {
		return "", time.Time{}, fmt.Errorf("mint a token for %s: GitHub returned none", repo)
	}
	b.cache[repo] = cachedGithubToken{token: minted.Token, expires: minted.ExpiresAt}
	return minted.Token, minted.ExpiresAt, nil
}

func (b *githubBroker) call(ctx context.Context, method, path, bearer string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, b.api+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode/100 != 2 {
		var failure struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(payload, &failure)
		return fmt.Errorf("GitHub answered %d %s", response.StatusCode, failure.Message)
	}
	return json.Unmarshal(payload, out)
}

// githubToken answers one github_token frame. Only a live seat's own token
// qualifies, and a browser client never does: the reply is a credential.
func (d *daemon) githubToken(cl *client, message frame) error {
	if cl.web {
		return withExit(exitUsage, errors.New("a browser client cannot ask for a GitHub token"))
	}
	if d.byToken(message.Token) == nil {
		return withExit(exitUsage, errors.New("the token names no live session, and a GitHub token belongs to one"))
	}
	d.mu.Lock()
	if d.github == nil {
		broker, err := newGithubBroker()
		if err != nil {
			d.mu.Unlock()
			return err
		}
		d.github = broker
	}
	broker := d.github
	d.mu.Unlock()
	token, expires, err := broker.token(context.Background(), message.Repo)
	if err != nil {
		return err
	}
	return cl.c.write(frame{Type: "github_token", ID: message.ID, Repo: message.Repo, GithubToken: token, Expires: expires.UTC().Format(time.RFC3339)})
}

// requestGithubToken is the client side: the session token names the caller, so
// a seat can ask only as itself.
func requestGithubToken(repo string) (string, error) {
	token := strings.TrimSpace(os.Getenv(sessionTokenEnv))
	if token == "" {
		return "", withExit(exitUsage, fmt.Errorf(
			"%s is unset: a GitHub token belongs to a session aterm launched, so ask from inside one", sessionTokenEnv))
	}
	c, err := dialDaemon(false)
	if err != nil {
		return "", withExit(exitMissing, err)
	}
	defer c.Close()
	if !slices.Contains(c.features, githubTokenFeature) {
		return "", errors.New("the running aterm daemon predates the GitHub broker. " +
			"It restarts on the upgraded binary after five idle minutes")
	}
	reply, err := c.request(frame{Type: "github_token", Token: token, Repo: repo})
	if err != nil {
		if reply.Code != 0 {
			return "", withExit(reply.Code, err)
		}
		return "", err
	}
	return reply.GithubToken, nil
}

func newGithubTokenCommand() *cli.Command {
	return &cli.Command{
		Name:  "github-token",
		Usage: "print a short-lived GitHub App token for one repo, minted by the daemon, for the guarded gh",
		Flags: []cli.Flag{&cli.StringFlag{Name: "repo", Required: true, Usage: "OWNER/REPO the token is scoped to"}},
		Action: func(_ context.Context, cmd *cli.Command) error {
			token, err := requestGithubToken(cmd.String("repo"))
			if err != nil {
				return err
			}
			// Only the token, so command substitution captures nothing else.
			_, err = fmt.Fprintln(cmd.Root().Writer, token)
			return err
		},
	}
}
