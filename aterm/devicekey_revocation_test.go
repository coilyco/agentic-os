package main

import (
	"context"
	"crypto/x509"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// staticRevocations is a list a test fixes, so no test reaches Google.
type staticRevocations map[string]revocationEntry

func (s staticRevocations) list(context.Context, time.Time) (map[string]revocationEntry, error) {
	return s, nil
}

type brokenRevocations struct{}

func (brokenRevocations) list(context.Context, time.Time) (map[string]revocationEntry, error) {
	return nil, errors.New("no route to android.googleapis.com")
}

func certWithSerial(serial int64) *x509.Certificate {
	return &x509.Certificate{SerialNumber: big.NewInt(serial)}
}

func TestCheckRevocationMatchesBothSerialSpellingsAndNamesTheStatus(t *testing.T) {
	// 0xabcdef is the hex key, 255 is "ff" in hex and "255" in decimal.
	chain := []*x509.Certificate{certWithSerial(1), certWithSerial(0xabcdef)}
	err := checkRevocation(context.Background(), staticRevocations{"abcdef": {Status: "SUSPENDED", Reason: "SOFTWARE_FLAW"}}, chain, time.Now())
	if err == nil || !strings.Contains(err.Error(), "certificate 1 is SUSPENDED (SOFTWARE_FLAW)") {
		t.Fatalf("a hex-keyed suspension read as %v", err)
	}
	if reasonFor(err) != reasonRevocation {
		t.Fatalf("reason = %q, want %q", reasonFor(err), reasonRevocation)
	}
	legacy := []*x509.Certificate{certWithSerial(255)}
	if err := checkRevocation(context.Background(), staticRevocations{"255": {Status: "REVOKED", Reason: "KEY_COMPROMISE"}}, legacy, time.Now()); err == nil || !strings.Contains(err.Error(), "REVOKED") {
		t.Fatalf("a decimal-keyed revocation read as %v", err)
	}
	if err := checkRevocation(context.Background(), staticRevocations{"abcdef": {Status: "REVOKED"}}, []*x509.Certificate{certWithSerial(7)}, time.Now()); err != nil {
		t.Fatalf("an unlisted chain was refused: %v", err)
	}
}

func TestCheckRevocationFailsClosedWhenTheListCannotBeRead(t *testing.T) {
	err := checkRevocation(context.Background(), brokenRevocations{}, []*x509.Certificate{certWithSerial(1)}, time.Now())
	if err == nil || reasonFor(err) != reasonRevocation || !strings.Contains(err.Error(), "enrollment is refused") {
		t.Fatalf("an unreadable list read as %v", err)
	}
}

func statusServer(t *testing.T, body string, status *atomic.Int32, hits *atomic.Int32) *googleRevocations {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		if code := int(status.Load()); code != http.StatusOK {
			w.WriteHeader(code)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return &googleRevocations{url: server.URL, client: server.Client()}
}

func TestGoogleRevocationsCachesThenFallsBackToAStaleCopyThenRefuses(t *testing.T) {
	var status, hits atomic.Int32
	status.Store(http.StatusOK)
	source := statusServer(t, `{"entries":{"abcdef":{"status":"REVOKED","reason":"KEY_COMPROMISE"}}}`, &status, &hits)
	start := time.Now()
	for i := 0; i < 2; i++ {
		entries, err := source.list(context.Background(), start.Add(time.Hour))
		if err != nil || entries["abcdef"].Status != "REVOKED" {
			t.Fatalf("list = %v, %v", entries, err)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("a fresh copy was fetched %d times, want 1", hits.Load())
	}
	status.Store(http.StatusServiceUnavailable)
	if _, err := source.list(context.Background(), start.Add(3*24*time.Hour)); err != nil {
		t.Fatalf("a 3 day old copy should cover a failed refetch: %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("a stale copy should have tried a refetch, hits = %d", hits.Load())
	}
	if _, err := source.list(context.Background(), start.Add(8*24*time.Hour)); err == nil {
		t.Fatal("an 8 day old copy covered a failed refetch")
	}
}

func TestGoogleRevocationsRefusesAnythingButTheEntriesDocument(t *testing.T) {
	var status, hits atomic.Int32
	status.Store(http.StatusOK)
	for _, body := range []string{`not json`, `{}`, `{"entries":null}`} {
		source := statusServer(t, body, &status, &hits)
		if _, err := source.list(context.Background(), time.Now()); err == nil {
			t.Errorf("%q was accepted as a status list", body)
		}
	}
}

func TestEnrollmentRefusesAChainTheListRevokesAndStoresNothing(t *testing.T) {
	d, _ := deviceDaemon(t, newPhone(t, attest{}))
	device := pipeClient(t, d, peerStanding{web: true, remote: true})
	nextFrame(t, device, "welcome")
	begin, err := device.request(frame{Type: "device_enroll_begin", EnrollCode: mintCode(t, d)})
	if err != nil {
		t.Fatal(err)
	}
	challenge, _ := rawURL.DecodeString(begin.Challenge)
	p := newPhone(t, attest{challenge: challenge})
	d.passkeys.roots = p.roots
	// The test phone's intermediate has serial 2.
	d.passkeys.revocations = staticRevocations{"2": {Status: "REVOKED", Reason: "KEY_COMPROMISE"}}
	_, err = device.request(frame{Type: "device_enroll_finish", PublicKey: rawURL.EncodeToString(p.spki), Attestation: jsonOf(p.chain)})
	if err == nil || !strings.Contains(err.Error(), "REVOKED (KEY_COMPROMISE)") {
		t.Fatalf("a revoked intermediate read as %v", err)
	}
	if d.passkeys.deviceKeyEnrolled() {
		t.Fatal("a refused enrollment stored a key")
	}
}
