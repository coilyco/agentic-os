package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

var enrollCodePattern = regexp.MustCompile(`^[A-Z0-9]{4}-[A-Z0-9]{4}$`)

// enrollOptions are what `aterm treehouse enroll` is given.
type enrollOptions struct {
	Accounts, Dir, Code, Name, Substrate string
	Replace                              bool
}

// jwkThumbprint is the RFC 7638 thumbprint of a P-256 key, the certificate's cnf.jkt.
func jwkThumbprint(key *ecdsa.PublicKey) string {
	canonical := fmt.Sprintf(`{"crv":"P-256","kty":"EC","x":%q,"y":%q}`,
		rawURL.EncodeToString(key.X.FillBytes(make([]byte, 32))), rawURL.EncodeToString(key.Y.FillBytes(make([]byte, 32))))
	sum := sha256.Sum256([]byte(canonical))
	return rawURL.EncodeToString(sum[:])
}

// enrollDaemon redeems a one-time code for a certificate and writes nothing until it
// verifies against the accounts keys and binds this key, so a failure leaves no orphan.
func enrollDaemon(ctx context.Context, o enrollOptions) (daemonIdentity, error) {
	code := strings.ToUpper(strings.TrimSpace(o.Code))
	if !enrollCodePattern.MatchString(code) {
		return daemonIdentity{}, errors.New("the code looks like ABCD-1234")
	}
	if !strings.HasPrefix(o.Accounts, "https://") && !isLoopbackURL(o.Accounts) {
		return daemonIdentity{}, errors.New("--treehouse-accounts must be https")
	}
	if o.Dir == "" {
		return daemonIdentity{}, errors.New("--treehouse-dir is required")
	}
	name := strings.TrimSpace(o.Name)
	if name == "" {
		name, _ = os.Hostname()
	}
	if name == "" || len(name) > 64 {
		return daemonIdentity{}, errors.New("--name needs 1 to 64 characters")
	}
	substrate := o.Substrate
	if substrate == "" {
		substrate = "other"
	}
	if !slices.Contains([]string{"mac", "microvm", "gke", "other"}, substrate) {
		return daemonIdentity{}, errors.New("--substrate is mac, microvm, gke or other")
	}
	keyPath, certPath := filepath.Join(o.Dir, "daemon.key"), filepath.Join(o.Dir, "daemon.cert")
	if !o.Replace {
		for _, path := range []string{keyPath, certPath} {
			if _, err := os.Stat(path); err == nil {
				return daemonIdentity{}, fmt.Errorf("%s exists, so this daemon is enrolled. Pass --replace to enroll again", path)
			}
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return daemonIdentity{}, err
	}
	body := map[string]any{"code": code, "name": name, "substrate": substrate, "daemon_pub": map[string]string{
		"kty": "EC", "crv": "P-256",
		"x": rawURL.EncodeToString(key.X.FillBytes(make([]byte, 32))), "y": rawURL.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
	}}
	var reply certReply
	if err := postAccounts(ctx, strings.TrimRight(o.Accounts, "/")+"/v1/daemons", body, &reply); err != nil {
		return daemonIdentity{}, err
	}
	keys := newAccountsKeys(strings.TrimRight(o.Accounts, "/")+"/.well-known/jwks.json", fetchJWKS)
	identity, err := loadDaemonIdentity(ctx, keys, o.Accounts, reply.Cert, time.Now(), false)
	if err != nil {
		return daemonIdentity{}, err
	}
	if identity.ID != reply.DaemonID || !certBindsKey(reply.Cert, key) {
		return daemonIdentity{}, errors.New("accounts issued a certificate that does not name this daemon and key")
	}
	if err := writeKeyFile(keyPath, key); err != nil {
		return daemonIdentity{}, err
	}
	if err := writeFileAtomic(certPath, []byte(strings.TrimSpace(reply.Cert)+"\n"), 0o600); err != nil {
		return daemonIdentity{}, err
	}
	return identity, nil
}

// certBindsKey is true when the certificate's cnf.jkt is this key's thumbprint.
func certBindsKey(cert string, key *ecdsa.PrivateKey) bool {
	parts := strings.Split(cert, ".")
	if len(parts) != 3 {
		return false
	}
	raw, err := rawURL.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var c struct{ Cnf struct{ Jkt string } }
	return json.Unmarshal(raw, &c) == nil && c.Cnf.Jkt != "" && c.Cnf.Jkt == jwkThumbprint(&key.PublicKey)
}

func newTreehouseCommand() *cli.Command {
	return &cli.Command{
		Name:  "treehouse",
		Usage: "enroll this daemon with Treehouse accounts",
		Commands: []*cli.Command{{
			Name:        "enroll",
			Usage:       "redeem a one-time code from Treehouse accounts for this daemon's key and certificate",
			ArgsUsage:   "<code>",
			Description: "Mint the code in the Treehouse client from a signed-in, user-verified session. The key is made here\nand never leaves this host. Then start the daemon with the same --treehouse-dir.",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "treehouse-accounts", Required: true, Usage: "https address of Treehouse accounts", Sources: cli.EnvVars("ATERM_TREEHOUSE_ACCOUNTS")},
				&cli.StringFlag{Name: "treehouse-dir", Required: true, Usage: "directory for daemon.key and daemon.cert", Sources: cli.EnvVars("ATERM_TREEHOUSE_DIR")},
				&cli.StringFlag{Name: "name", Usage: "this daemon's name in the client, the host name when empty"},
				&cli.StringFlag{Name: "substrate", Value: "other", Usage: "mac, microvm, gke or other"},
				&cli.BoolFlag{Name: "replace", Usage: "enroll again over an existing key and certificate"},
			},
			Action: func(ctx context.Context, cmd *cli.Command) error {
				if cmd.Args().Len() != 1 {
					return withExit(exitUsage, errors.New("give the one-time code"))
				}
				identity, err := enrollDaemon(ctx, enrollOptions{
					Accounts: cmd.String("treehouse-accounts"), Dir: cmd.String("treehouse-dir"), Code: cmd.Args().First(),
					Name: cmd.String("name"), Substrate: cmd.String("substrate"), Replace: cmd.Bool("replace"),
				})
				if err != nil {
					return withExit(exitUsage, err)
				}
				_, err = fmt.Fprintf(cmd.Root().Writer, "enrolled as %s for %s. Start the daemon with --treehouse-broker, --treehouse-accounts and --treehouse-dir %s.\n",
					identity.ID, identity.Owner, cmd.String("treehouse-dir"))
				return err
			},
		}},
	}
}
