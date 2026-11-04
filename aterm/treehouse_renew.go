package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// renewBefore is how long before expiry the daemon renews, and a redial renews inside it.
var renewBefore = 10 * time.Minute

// renewRetry paces a failed renewal. The certificate lasts an hour, so it tries often.
var renewRetry = 30 * time.Second

// accountsPoster posts a JSON body to accounts and decodes the reply. A non-2xx reply
// becomes an error carrying treehouse.error.v1's message.
func accountsPoster(base string) func(ctx context.Context, path string, body, out any) error {
	return func(ctx context.Context, path string, body, out any) error {
		return postAccounts(ctx, strings.TrimRight(base, "/")+path, body, out)
	}
}

func postAccounts(ctx context.Context, address string, body, out any) error {
	if !strings.HasPrefix(address, "https://") && !isLoopbackURL(address) {
		return errors.New("accounts must be https")
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		var e struct{ Error, Message string }
		_ = json.Unmarshal(data, &e)
		return fmt.Errorf("accounts answered %d %s %s", resp.StatusCode, e.Error, e.Message)
	}
	return json.Unmarshal(data, out)
}

func isLoopbackURL(address string) bool {
	rest, ok := strings.CutPrefix(address, "http://")
	if !ok {
		return false
	}
	host, _, _ := strings.Cut(rest, "/")
	return isLoopbackHost(host)
}

// certReply is accounts' answer to an enrollment or a renewal.
type certReply struct {
	DaemonID  string `json:"daemon_id"`
	Cert      string `json:"cert"`
	ExpiresIn int    `json:"expires_in"`
}

func (l *brokerLink) currentCert() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cert
}

// certExpiry reads exp from the certificate this daemon holds, which was verified when
// it was loaded or renewed.
func (l *brokerLink) certExpiry() time.Time { return certExpiry(l.currentCert()) }

func certExpiry(cert string) time.Time {
	parts := strings.Split(cert, ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	raw, err := rawURL.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var c struct{ Exp int64 }
	if json.Unmarshal(raw, &c) != nil {
		return time.Time{}
	}
	return time.Unix(c.Exp, 0)
}

// renew trades the certificate for a new one at accounts, which takes a lapsed one on a
// good proof. The reply must verify and name this daemon and owner before it is kept.
func (l *brokerLink) renew(ctx context.Context) error {
	proof, err := signDaemonProof(l.key, "treehouse-accounts", "", time.Now())
	if err != nil {
		return err
	}
	var reply certReply
	if err := l.post(ctx, "/v1/daemons/renew", map[string]string{"cert": l.currentCert(), "proof": proof}, &reply); err != nil {
		return err
	}
	identity, err := loadDaemonIdentity(ctx, l.keys, l.accounts, reply.Cert, time.Now(), false)
	if err != nil {
		return err
	}
	if identity != l.identity {
		return errors.New("accounts renewed a certificate for another daemon")
	}
	if err := writeFileAtomic(l.certPath, []byte(strings.TrimSpace(reply.Cert)+"\n"), 0o600); err != nil {
		return err
	}
	l.mu.Lock()
	l.cert = strings.TrimSpace(reply.Cert)
	l.mu.Unlock()
	l.d.logf("renewed the Treehouse daemon certificate")
	return nil
}

// writeFileAtomic replaces path with data through a temp file beside it.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-"+filepath.Base(path))
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// renewLoop renews ahead of expiry for as long as the link lives, and tells the broker.
func (s *brokerSession) renewLoop(ctx context.Context) {
	for {
		wait := max(time.Until(s.link.certExpiry())-renewBefore, 0)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if err := s.link.renew(ctx); err != nil {
			s.link.d.logf("could not renew the Treehouse daemon certificate: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(renewRetry):
			}
			continue
		}
		if err := s.send(ctx, brokerMsg{Type: "treehouse.reg.cert.v1", Cert: s.link.currentCert()}); err != nil {
			return
		}
	}
}

// emit tells the broker a seat is waiting. With no link up the event is dropped, and the
// broker's reachable push covers the return.
func (l *brokerLink) emit(e waitEvent) {
	msg := brokerMsg{Type: "treehouse.reg.event.v1", Kind: e.Kind, SessionName: e.Session, Role: e.Role, Identity: e.Identity}
	if l.sendEvent != nil {
		l.sendEvent(msg)
		return
	}
	l.mu.Lock()
	s := l.cur
	l.mu.Unlock()
	if s == nil {
		return
	}
	go func() {
		defer l.d.guard("broker event")
		_ = s.send(context.Background(), msg)
	}()
}

// watchSeats reads the roster on a timer and emits what the wait tracker finds, as the
// local Web Push watcher does, on a tracker of its own.
func (l *brokerLink) watchSeats(ctx context.Context, every time.Duration) {
	defer l.d.guard("broker seat watcher")
	tracker := waitTracker{settle: l.settle}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			for _, event := range tracker.observe(l.views(), now) {
				l.emit(event)
			}
		}
	}
}
