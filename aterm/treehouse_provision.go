package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	mathrand "math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// A target Treehouse starts itself claims its certificate with a secret from its secret
// store. Accounts names the owner, never this claim (treehouse-servers contract flows).

// provisionBackoff paces a refused claim. Accounts answers 401 until the owner has
// signed up, so a target that starts first keeps asking without hammering.
var provisionBackoff = struct{ first, max time.Duration }{5 * time.Second, 5 * time.Minute}

type provisionOptions struct {
	Accounts, Dir, SecretFile, Name, Substrate string
}

// readProvisionSecret returns the secret, or an error that never carries its value.
func readProvisionSecret(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("the provisioned secret is not readable yet: %w", err)
	}
	secret := strings.TrimSpace(string(raw))
	if secret == "" {
		return "", errors.New("the provisioned secret file is empty")
	}
	return secret, nil
}

// provisionKey loads the daemon key, or stores a new one before the first claim, so a
// claim whose reply was lost retries with the same key and gets the same daemon.
func provisionKey(dir string) (*ecdsa.PrivateKey, error) {
	path := filepath.Join(dir, "daemon.key")
	if _, err := os.Stat(path); err == nil {
		return loadDaemonKey(path)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := writeKeyFile(path, key); err != nil {
		return nil, err
	}
	return key, nil
}

// writeKeyFile stores a P-256 key as PKCS8 PEM with mode 0600 in a 0700 directory.
func writeKeyFile(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600)
}

// claimProvisioned makes one claim and writes daemon.cert only after the certificate
// verifies against the accounts keys and binds this key.
func claimProvisioned(ctx context.Context, o provisionOptions, keys *accountsKeys, post func(ctx context.Context, path string, body, out any) error) (daemonIdentity, error) {
	secret, err := readProvisionSecret(o.SecretFile)
	if err != nil {
		return daemonIdentity{}, err
	}
	key, err := provisionKey(o.Dir)
	if err != nil {
		return daemonIdentity{}, err
	}
	body := map[string]any{"secret": secret, "name": o.Name, "substrate": o.Substrate, "daemon_pub": map[string]string{
		"kty": "EC", "crv": "P-256",
		"x": rawURL.EncodeToString(key.X.FillBytes(make([]byte, 32))), "y": rawURL.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
	}}
	var reply certReply
	if err := post(ctx, "/v1/daemons/provision", body, &reply); err != nil {
		return daemonIdentity{}, err
	}
	identity, err := loadDaemonIdentity(ctx, keys, o.Accounts, reply.Cert, time.Now(), false)
	if err != nil {
		return daemonIdentity{}, err
	}
	if identity.ID != reply.DaemonID || !certBindsKey(reply.Cert, key) {
		return daemonIdentity{}, errors.New("accounts issued a certificate that does not name this daemon and key")
	}
	if err := writeFileAtomic(filepath.Join(o.Dir, "daemon.cert"), []byte(strings.TrimSpace(reply.Cert)+"\n"), 0o600); err != nil {
		return daemonIdentity{}, err
	}
	return identity, nil
}

// provisionDaemon claims until it succeeds or ctx ends, never logging the secret.
func (d *daemon) provisionDaemon(ctx context.Context, o provisionOptions) (daemonIdentity, error) {
	keys := newAccountsKeys(strings.TrimRight(o.Accounts, "/")+"/.well-known/jwks.json", fetchJWKS)
	post := accountsPoster(o.Accounts)
	delay := provisionBackoff.first
	for {
		identity, err := claimProvisioned(ctx, o, keys, post)
		if err == nil {
			return identity, nil
		}
		d.logf("the Treehouse claim with the provisioned secret failed, trying again: %v", err)
		wait := delay/2 + mathrand.N(delay/2+1)
		select {
		case <-ctx.Done():
			return daemonIdentity{}, ctx.Err()
		case <-time.After(wait):
		}
		delay = min(delay*2, provisionBackoff.max)
	}
}

func provisionName(name string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	host, _ := os.Hostname()
	return host[:min(len(host), 64)]
}

func provisionSubstrate(s string) string {
	if slices.Contains([]string{"mac", "microvm", "gke", "other"}, s) {
		return s
	}
	return "other"
}
