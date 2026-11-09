package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"
)

// The daemon's half of the Treehouse contract in the treehouse-servers repository:
// verifying the tokens accounts signs. Standard library only, ES256 only.

const (
	unlockTokenType = "treehouse.token.unlock.v1"
	// unlockMaxLife bounds exp - iat, so a captured token is useless within two minutes.
	unlockMaxLife = 120 * time.Second
	unlockSkew    = 30 * time.Second
	// A key set is fresh for an hour and usable stale for a day, as the contract says.
	keysFresh = time.Hour
	keysStale = 24 * time.Hour
	// A kid the cache does not know costs one fetch per interval, however many arrive.
	keysRefetch = 30 * time.Second
	// reasonUnlockRefused is every way an unlock token can fail, so a prober learns nothing.
	reasonUnlockRefused = "unlock_refused"
)

// accountsKeys is the accounts signing keys a daemon trusts, read from its JWKS.
type accountsKeys struct {
	mu      sync.Mutex
	url     string
	fetch   func(ctx context.Context, url string) ([]byte, error)
	now     func() time.Time
	keys    map[string]*ecdsa.PublicKey
	fetched time.Time
	tried   time.Time
}

func newAccountsKeys(url string, fetch func(context.Context, string) ([]byte, error)) *accountsKeys {
	return &accountsKeys{url: url, fetch: fetch, now: time.Now}
}

// get returns the key for kid. With no key, or none fresh enough, it fails closed.
func (a *accountsKeys) get(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	fresh := a.keys != nil && now.Sub(a.fetched) < keysFresh
	if key, ok := a.keys[kid]; ok && fresh {
		return key, nil
	}
	if now.Sub(a.tried) >= keysRefetch {
		a.tried = now
		if raw, err := a.fetch(ctx, a.url); err == nil {
			if keys, err := parseJWKS(raw); err == nil {
				a.keys, a.fetched = keys, now
			}
		}
	}
	if a.keys != nil && now.Sub(a.fetched) < keysStale {
		if key, ok := a.keys[kid]; ok {
			return key, nil
		}
	}
	return nil, errors.New("no accounts key for that token")
}

func parseJWKS(raw []byte) (map[string]*ecdsa.PublicKey, error) {
	var set struct {
		Keys []struct {
			Kty, Crv, Kid, X, Y, Alg, Use string
		} `json:"keys"`
	}
	if err := json.Unmarshal(raw, &set); err != nil {
		return nil, err
	}
	keys := map[string]*ecdsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "EC" || k.Crv != "P-256" || k.Kid == "" || (k.Alg != "" && k.Alg != "ES256") || (k.Use != "" && k.Use != "sig") {
			continue
		}
		key, err := parseP256(k.X, k.Y)
		if err != nil {
			continue
		}
		keys[k.Kid] = key
	}
	if len(keys) == 0 {
		return nil, errors.New("the key set holds no usable ES256 key")
	}
	return keys, nil
}

func parseP256(x, y string) (*ecdsa.PublicKey, error) {
	xb, err := rawURL.DecodeString(x)
	if err != nil || len(xb) != 32 {
		return nil, errors.New("bad x")
	}
	yb, err := rawURL.DecodeString(y)
	if err != nil || len(yb) != 32 {
		return nil, errors.New("bad y")
	}
	key := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(xb), Y: new(big.Int).SetBytes(yb)}
	if !key.Curve.IsOnCurve(key.X, key.Y) {
		return nil, errors.New("not on the curve")
	}
	return key, nil
}

// verifyJWS checks a compact ES256 token against the accounts keys and returns its
// payload. alg and typ are exact matches, so no other algorithm or token kind passes.
func verifyJWS(ctx context.Context, keys *accountsKeys, raw, wantTyp string) ([]byte, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, errors.New("not a compact token")
	}
	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("bad header")
	}
	var header struct{ Alg, Typ, Kid string }
	if err := json.Unmarshal(headerRaw, &header); err != nil {
		return nil, errors.New("bad header")
	}
	if header.Alg != "ES256" || header.Typ != wantTyp || header.Kid == "" {
		return nil, errors.New("wrong alg, typ or kid")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return nil, errors.New("bad signature")
	}
	key, err := keys.get(ctx, header.Kid)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		return nil, errors.New("signature does not verify")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("bad payload")
	}
	return payload, nil
}

// daemonIdentity is who this daemon is, from its accounts-signed certificate.
type daemonIdentity struct {
	ID    string
	Owner string
}

// unlockVerifier judges the unlock token of one brokered channel. A daemon without
// accounts keys refuses every token, so it fails closed.
type unlockVerifier struct {
	keys     *accountsKeys
	issuer   string
	identity daemonIdentity
	now      func() time.Time
	mu       sync.Mutex
	spent    map[string]time.Time
}

func newUnlockVerifier(keys *accountsKeys, issuer string, identity daemonIdentity) *unlockVerifier {
	return &unlockVerifier{keys: keys, issuer: issuer, identity: identity, now: time.Now, spent: map[string]time.Time{}}
}

type unlockClaims struct {
	Iss   string `json:"iss"`
	Use   string `json:"use"`
	Sub   string `json:"sub"`
	Aud   string `json:"aud"`
	Nonce string `json:"nonce"`
	How   string `json:"how"`
	Acr   string `json:"acr"`
	Jti   string `json:"jti"`
	Iat   int64  `json:"iat"`
	Exp   int64  `json:"exp"`
}

// check spends the jti and returns nil only when every claim holds for this daemon,
// owner and channel. The error names the failed check for the log, not the client.
func (v *unlockVerifier) check(ctx context.Context, raw, nonce string) error {
	payload, err := verifyJWS(ctx, v.keys, raw, unlockTokenType)
	if err != nil {
		return err
	}
	var c unlockClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return errors.New("bad claims")
	}
	now := v.now()
	iat, exp := time.Unix(c.Iat, 0), time.Unix(c.Exp, 0)
	switch {
	case c.Iss == "" || c.Iss != v.issuer:
		return errors.New("wrong issuer")
	case c.Use != "unlock":
		return errors.New("not an unlock token")
	case c.Aud != v.identity.ID || v.identity.ID == "":
		return errors.New("for another daemon")
	case c.Sub != v.identity.Owner || v.identity.Owner == "":
		return errors.New("not this daemon's owner")
	case nonce == "" || subtle.ConstantTimeCompare([]byte(c.Nonce), []byte(nonce)) != 1:
		return errors.New("for another channel")
	case c.Acr != "uv":
		return errors.New("no user verification")
	case c.How != "passkey" && c.How != "device-key" && c.How != "session":
		return errors.New("unknown method")
	case len(c.Jti) < 8:
		return errors.New("no jti")
	case iat.After(now.Add(unlockSkew)):
		return errors.New("issued in the future")
	case !exp.After(now):
		return errors.New("expired")
	case exp.Sub(iat) > unlockMaxLife:
		return errors.New("lives too long")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	for jti, until := range v.spent {
		if !until.After(now) {
			delete(v.spent, jti)
		}
	}
	if _, replay := v.spent[c.Jti]; replay {
		return errors.New("already spent")
	}
	v.spent[c.Jti] = exp.Add(unlockSkew)
	return nil
}

// loadDaemonIdentity reads a daemon certificate and verifies it, so a tampered file
// cannot change who the daemon says it is or who owns it.
func loadDaemonIdentity(ctx context.Context, keys *accountsKeys, issuer, cert string, now time.Time, lapsedOK bool) (daemonIdentity, error) {
	payload, err := verifyJWS(ctx, keys, strings.TrimSpace(cert), "treehouse.token.daemon.v1")
	if err != nil {
		return daemonIdentity{}, fmt.Errorf("daemon certificate: %w", err)
	}
	var c struct {
		Iss, Use, Sub, Acct string
		Exp                 int64
	}
	if err := json.Unmarshal(payload, &c); err != nil || c.Iss != issuer || c.Use != "daemon" || c.Sub == "" || c.Acct == "" {
		return daemonIdentity{}, errors.New("daemon certificate: wrong claims")
	}
	if !lapsedOK && !time.Unix(c.Exp, 0).After(now) {
		return daemonIdentity{}, errors.New("daemon certificate: expired")
	}
	return daemonIdentity{ID: c.Sub, Owner: c.Acct}, nil
}
