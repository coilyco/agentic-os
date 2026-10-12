package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	attestationStatusURL = "https://android.googleapis.com/attestation/status"
	// Google serves the list with max-age 86400. A copy older than revocationStaleMax is
	// not trusted when a refetch fails.
	revocationFreshFor = 24 * time.Hour
	revocationStaleMax = 7 * 24 * time.Hour
	maxRevocationBytes = 4 << 20
	revocationFetchTTL = 15 * time.Second
	reasonRevocation   = "attestation_revoked"
)

// revocationEntry is one status the list gives a certificate serial.
type revocationEntry struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// revocationSource answers the list at enrollment. A failure is the source's
// to report, and the caller applies the fail-closed policy (COI-2626).
type revocationSource interface {
	list(ctx context.Context, now time.Time) (map[string]revocationEntry, error)
}

// googleRevocations fetches Google's list and keeps the last good copy in memory.
type googleRevocations struct {
	url    string
	client *http.Client

	mu      sync.Mutex
	entries map[string]revocationEntry
	fetched time.Time
}

func newGoogleRevocations() *googleRevocations {
	return &googleRevocations{url: attestationStatusURL, client: &http.Client{Timeout: revocationFetchTTL}}
}

func (g *googleRevocations) list(ctx context.Context, now time.Time) (map[string]revocationEntry, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.entries != nil && now.Sub(g.fetched) < revocationFreshFor {
		return g.entries, nil
	}
	entries, err := g.fetch(ctx)
	if err == nil {
		g.entries, g.fetched = entries, now
		return entries, nil
	}
	if g.entries != nil && now.Sub(g.fetched) < revocationStaleMax {
		return g.entries, nil
	}
	return nil, err
}

func (g *googleRevocations) fetch(ctx context.Context) (map[string]revocationEntry, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, g.url, nil)
	if err != nil {
		return nil, err
	}
	response, err := g.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the status list answered %s", response.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxRevocationBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxRevocationBytes {
		return nil, errors.New("the status list is over its size limit")
	}
	var parsed struct {
		Entries map[string]revocationEntry `json:"entries"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Entries == nil {
		return nil, errors.New("the status list is not the entries document")
	}
	return parsed.Entries, nil
}

// revocationKeys are the two spellings Google keys a serial by: lowercase hex
// without leading zeros for current certificates, decimal for the legacy batch ones.
func revocationKeys(cert *x509.Certificate) []string {
	return []string{cert.SerialNumber.Text(16), cert.SerialNumber.Text(10)}
}

// checkRevocation refuses a chain holding a certificate the list marks revoked or
// suspended, and fails closed when the list cannot be read at all.
func checkRevocation(ctx context.Context, source revocationSource, chain []*x509.Certificate, now time.Time) error {
	if source == nil {
		return nil
	}
	entries, err := source.list(ctx, now)
	if err != nil {
		return withReason(reasonRevocation, fmt.Errorf(
			"the Android attestation revocation list could not be read, so enrollment is refused: %w", err))
	}
	for position, cert := range chain {
		for _, key := range revocationKeys(cert) {
			if entry, listed := entries[key]; listed {
				return withReason(reasonRevocation, fmt.Errorf(
					"attestation certificate %d is %s (%s) on Google's revocation list",
					position, entry.Status, entry.Reason))
			}
		}
	}
	return nil
}
