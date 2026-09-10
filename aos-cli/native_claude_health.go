// Reports the canonical Claude credential's state without touching the token.
// See docs/native-claude-credentials.md.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Credential states doctor renders. Only claudeCredentialLive is healthy.
const (
	claudeCredentialMissing     = "missing"
	claudeCredentialUnstamped   = "unstamped"
	claudeCredentialStale       = "stale"
	claudeCredentialRefreshable = "refreshable"
	claudeCredentialLive        = "live"
)

// claudeCredentialHealth carries state and reasoning, never token material.
type claudeCredentialHealth struct {
	State  string `json:"state"`
	Detail string `json:"detail"`
	Path   string `json:"path"`
}

// Healthy reports whether a launch can expect to start logged in.
func (health claudeCredentialHealth) Healthy() bool {
	return health.State == claudeCredentialLive ||
		health.State == claudeCredentialRefreshable
}

func (health claudeCredentialHealth) Line() string {
	return fmt.Sprintf("%s: %s", health.State, health.Detail)
}

// inspectCanonicalClaudeCredential is the read every doctor surface shares, so
// no caller reimplements the stamp comparison.
func inspectCanonicalClaudeCredential(home string, now time.Time) claudeCredentialHealth {
	path := canonicalClaudeCredentialPath(home)
	health := claudeCredentialHealth{Path: path}
	payload, err := os.ReadFile(path)
	if err != nil {
		health.State = claudeCredentialMissing
		health.Detail = "absent, so the next launch seeds one from the host Keychain"
		return health
	}
	access, refresh := claudeCredentialStamps(payload)
	switch {
	case access.IsZero():
		health.State = claudeCredentialUnstamped
		health.Detail = "no comparable expiresAt, so the harness deletes every staged " +
			"link and each session pays its own login"
	case now.Before(access):
		health.State = claudeCredentialLive
		health.Detail = "valid until " + stampUTC(access)
	case !refresh.IsZero() && now.Before(refresh):
		health.State = claudeCredentialRefreshable
		health.Detail = "access expired " + stampUTC(access) +
			", refresh valid until " + stampUTC(refresh)
	default:
		health.State = claudeCredentialStale
		health.Detail = "expired " + stampUTC(access) +
			" with no live refresh, so the next launch costs one login"
	}
	return health
}

func stampUTC(at time.Time) string { return at.UTC().Format(time.RFC3339) }

// claudeCredentialStamps reads both expiries. A zero return means unusable,
// matching how claudeCredentialExpiry treats an unstamped payload.
func claudeCredentialStamps(payload []byte) (access, refresh time.Time) {
	var envelope struct {
		OAuth struct {
			ExpiresAt        int64 `json:"expiresAt"`
			RefreshExpiresAt int64 `json:"refreshTokenExpiresAt"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return time.Time{}, time.Time{}
	}
	return millisStamp(envelope.OAuth.ExpiresAt), millisStamp(envelope.OAuth.RefreshExpiresAt)
}

func millisStamp(millis int64) time.Time {
	if millis <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(millis)
}
