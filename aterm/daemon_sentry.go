package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const sentryDSNEnv = "ATERM_SENTRY_DSN"

// sentryCheckIn paces the check-ins. Each carries the monitor's own config, so
// the daemon owns it, and two failed or missed in a row alert.
var sentryCheckIn = struct {
	first, every, timeout time.Duration
	margin, failures      int
}{30 * time.Second, 5 * time.Minute, 20 * time.Second, 5, 2}

// sentryCron posts check-ins to one Sentry cron monitor. See docs/aterm-bundles.md.
type sentryCron struct {
	endpoint string
	client   *http.Client
}

var slugUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

// sentryMonitorSlug names this host's monitor, so each machine alerts alone.
func sentryMonitorSlug(host string) string {
	slug := strings.Trim(slugUnsafe.ReplaceAllString(strings.ToLower(host), "-"), "-")
	if slug == "" {
		slug = "host"
	}
	return "aterm-daemon-" + slug[:min(len(slug), 30)]
}

// newSentryCron derives the cron check-in endpoint from a DSN. Errors never
// quote the DSN, which is a credential.
func newSentryCron(dsn, slug string) (*sentryCron, error) {
	parsed, err := url.Parse(strings.TrimSpace(dsn))
	if err != nil || parsed.Host == "" || parsed.User == nil || parsed.User.Username() == "" {
		return nil, errors.New("the Sentry DSN is not <scheme>://<key>@<host>/<project>")
	}
	project := strings.Trim(parsed.Path, "/")
	if project == "" || strings.Contains(project, "/") {
		return nil, errors.New("the Sentry DSN names no single project")
	}
	endpoint := (&url.URL{
		Scheme: parsed.Scheme,
		Host:   parsed.Host,
		Path:   "/api/" + project + "/cron/" + slug + "/" + parsed.User.Username() + "/",
	}).String()
	return &sentryCron{endpoint: endpoint, client: &http.Client{Timeout: sentryCheckIn.timeout}}, nil
}

// checkIn sends one status, ok or error, upserting the monitor on the way.
func (c *sentryCron) checkIn(ctx context.Context, ok bool) error {
	status := "error"
	if ok {
		status = "ok"
	}
	body, err := json.Marshal(map[string]any{
		"status": status,
		"monitor_config": map[string]any{
			"schedule":                map[string]any{"type": "interval", "value": int(sentryCheckIn.every / time.Minute), "unit": "minute"},
			"checkin_margin":          sentryCheckIn.margin,
			"failure_issue_threshold": sentryCheckIn.failures,
			"recovery_threshold":      1,
		},
	})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		// A transport error quotes the URL, which carries the DSN's key.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("sentry check-in: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<16))
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("sentry check-in: HTTP %d", response.StatusCode)
	}
	return nil
}

// sentryCheckIns reports each interval whether the tailnet listener completes
// a handshake now. A stopped daemon sends nothing, which Sentry reads as missed.
func (d *daemon) sentryCheckIns(done <-chan struct{}, cron *sentryCron) {
	defer d.guard("check-in")
	wait, lastHealth, lastSend := sentryCheckIn.first, "", ""
	for {
		select {
		case <-done:
			return
		case <-time.After(wait):
		}
		ctx, cancel := context.WithTimeout(context.Background(), sentryCheckIn.timeout)
		health := d.tailnetHealth(ctx)
		sent := cron.checkIn(ctx, health == nil)
		cancel()
		if state := fmt.Sprint(health); state != lastHealth {
			if health == nil {
				d.logf("tailnet listener healthy, reporting ok to Sentry")
			} else {
				d.logf("tailnet listener unhealthy, reporting an error to Sentry: %v", health)
			}
			lastHealth = state
		}
		if state := fmt.Sprint(sent); state != lastSend {
			if sent != nil {
				d.logf("%v", sent)
			}
			lastSend = state
		}
		wait = sentryCheckIn.every
	}
}

// hostName is this machine's short name, which names its monitor.
func hostName() string {
	name, _ := os.Hostname()
	return strings.SplitN(name, ".", 2)[0]
}
