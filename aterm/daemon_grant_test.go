package main

import (
	"context"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

type grantSeats struct {
	director, sysadmin, gamedev string
	kai                         *conn
}

func spawnGrantSeats(t *testing.T) grantSeats {
	t.Helper()
	testDaemon(t)
	one := func(name, role, identity string) string {
		seat := dialTest(t)
		seat.spawn(name, role, identity, `echo TOKEN=$ATERM_SESSION_TOKEN; sleep 60`)
		return seat.token()
	}
	return grantSeats{
		director: one("prod-director-griffin-goose", "prod-director", "Griffin-Goose"),
		sysadmin: one("sysadmin-senior-turtle-ox", "sysadmin-senior", "Turtle-Ox"),
		gamedev:  one("game-dev-whale-dragonfly", "game-dev", "Whale-Dragonfly"),
		kai:      subscribed(t),
	}
}

// requestGrant has a seat put a spec to Kai and returns the answered reply once
// Kai picks. pick -1 leaves it unanswered so the caller can answer later.
func (s grantSeats) requestGrant(t *testing.T, token string, spec grantSpec, pick int) (choiceAsk, choiceAnswer) {
	t.Helper()
	asker := dialTest(t).c
	if err := asker.write(frame{Type: "grant_request", ID: "g", Token: token, Grant: &spec}); err != nil {
		t.Fatalf("grant_request: %v", err)
	}
	shown := nextFrame(t, s.kai, "ask").Ask
	if err := s.kai.write(frame{Type: "answer", ID: "a", AskID: shown.ID, Picks: []int{pick}}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	reply := nextFrame(t, asker, "answered")
	return *shown, *reply.Answer
}

func (s grantSeats) check(t *testing.T, token string, check grantCheck) grantVerdict {
	t.Helper()
	c := dialTest(t).c
	reply, err := c.request(frame{Type: "grant_check", Token: token, Check: &check})
	if err != nil {
		t.Fatalf("grant_check: %v", err)
	}
	return *reply.Verdict
}

func TestGrantIsMintedOnlyFromKaisAnswerAndBindsExactly(t *testing.T) {
	seats := spawnGrantSeats(t)
	spec := wipeSpec()
	shown, answer := seats.requestGrant(t, seats.director, spec, 0)
	if !strings.Contains(shown.Question, "wipe the cycle 14 world") || !strings.Contains(shown.Question, "prod-director Griffin-Goose") {
		t.Fatalf("Kai should see the exact action and who asked: %s", shown.Question)
	}
	if answer.GrantID == "" || answer.State != "answered" {
		t.Fatalf("approval should hand the asker a grant id: %+v", answer)
	}
	good := wipeCheck(answer.GrantID)
	// The director carried the id, and the director is not a holder of it.
	if v := seats.check(t, seats.director, good); v.Allow || v.Reason != "wrong_holder" {
		t.Fatalf("the relaying seat must not be able to use the grant: %+v", v)
	}
	// A role claimed in the frame is ignored: the daemon reads it off the token.
	claimed := good
	claimed.Role = "sysadmin-senior"
	if v := seats.check(t, seats.director, claimed); v.Allow || v.Reason != "wrong_holder" {
		t.Fatalf("a claimed role must not stand in for the token's: %+v", v)
	}
	if v := seats.check(t, "forged", good); v.Allow || v.Reason != "unknown_seat" {
		t.Fatalf("a token the daemon never issued gets nothing: %+v", v)
	}
	if v := seats.check(t, seats.sysadmin, good); !v.Allow {
		t.Fatalf("the named executor should be allowed: %+v", v)
	}
	if v := seats.check(t, seats.sysadmin, good); v.Allow || v.Reason != "spent" {
		t.Fatalf("a second use must be refused: %+v", v)
	}
}

func TestGrantDeniedByKaiMintsNothing(t *testing.T) {
	seats := spawnGrantSeats(t)
	_, answer := seats.requestGrant(t, seats.director, wipeSpec(), 1)
	if answer.GrantID != "" {
		t.Fatalf("a denial must not mint: %+v", answer)
	}
}

func TestGrantRequestRefusesWhatIsUnbounded(t *testing.T) {
	seats := spawnGrantSeats(t)
	spec := wipeSpec()
	spec.TTLSeconds = 0
	c := dialTest(t).c
	reply, err := c.request(frame{Type: "grant_request", Token: seats.director, Grant: &spec})
	if err == nil || reply.Code != exitUsage {
		t.Fatalf("an unbounded request must be refused before Kai sees it: %v (code %d)", err, reply.Code)
	}
}

// Replays the COI-2664 gates that took only Kai's own word, one answer per decision.
// Harness prompts and classifier refusals are another kind and are not counted.
func TestReplayOfTheLaunchNightSequenceNeedsOneAnswerPerDecision(t *testing.T) {
	seats := spawnGrantSeats(t)
	decisions := []struct {
		action, target, gate, role string
		typedBefore, steps         int
	}{
		{"wipe the cycle 14 world", "kai-server eco-server", "destructive-wipe", "sysadmin-senior", 1, 4},
		{"go live on cycle 15", "the public server listing", "go-live", "sysadmin-senior", 1, 1},
		{"swap the spawn check", "cycle 15 spawn data check", "data-check-swap", "sysadmin-senior", 2, 2},
		{"deploy the Echo image", "sirens-echo deployment", "live-deploy", "sysadmin-senior", 1, 1},
		{"switch the Steam branch", "the Eco server install", "branch-switch", "sysadmin-senior", 1, 1},
		{"click through the desktop prompt", "the Eco launcher window", "desktop-click", "game-dev", 1, 1},
	}
	token := map[string]string{"sysadmin-senior": seats.sysadmin, "game-dev": seats.gamedev}
	typedBefore, kaiAnswers, allowed := 0, 0, 0
	for _, d := range decisions {
		typedBefore += d.typedBefore
		spec := grantSpec{
			Action: d.action, Target: d.target, Gates: []string{d.gate}, For: []string{d.role},
			TTLSeconds: 7200, Uses: d.steps,
		}
		_, answer := seats.requestGrant(t, seats.director, spec, 0)
		kaiAnswers++
		for step := 0; step < d.steps; step++ {
			check := grantCheck{ID: answer.GrantID, Action: d.action, Target: d.target, Gate: d.gate}
			if v := seats.check(t, token[d.role], check); !v.Allow {
				t.Fatalf("%s step %d refused: %+v", d.action, step+1, v)
			}
			allowed++
		}
		over := grantCheck{ID: answer.GrantID, Action: d.action, Target: d.target, Gate: d.gate}
		if v := seats.check(t, token[d.role], over); v.Allow {
			t.Fatalf("%s ran past its approved uses: %+v", d.action, v)
		}
	}
	if typedBefore != 7 || kaiAnswers != len(decisions) || allowed != 10 {
		t.Fatalf("typed before %d, Kai answered %d times for %d gate steps", typedBefore, kaiAnswers, allowed)
	}
}

// runGrant runs the grant command as the seat holding token and returns its output.
func runGrant(t *testing.T, token string, args ...string) (string, error) {
	t.Helper()
	t.Setenv(sessionTokenEnv, token)
	var out strings.Builder
	app := &cli.Command{Name: "aterm", Writer: &out, ErrWriter: &out, Commands: []*cli.Command{newGrantCommand()}}
	err := app.Run(context.Background(), append([]string{"aterm", "grant"}, args...))
	return strings.TrimSpace(out.String()), err
}

func TestGrantCommandsRoundTripThroughTheDaemon(t *testing.T) {
	seats := spawnGrantSeats(t)
	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := runGrant(t, seats.director, "request",
			"--action", "wipe the cycle 14 world", "--target", "kai-server eco-server",
			"--gate", "destructive-wipe", "--for", "sysadmin-senior", "--ttl", "1h")
		done <- result{out, err}
	}()
	shown := nextFrame(t, seats.kai, "ask").Ask
	if err := seats.kai.write(frame{Type: "answer", ID: "a", AskID: shown.ID, Picks: []int{0}}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	requested := <-done
	if requested.err != nil || !strings.HasPrefix(requested.out, "g-") {
		t.Fatalf("an approved request should print the grant id: %q %v", requested.out, requested.err)
	}
	args := []string{"check", requested.out, "--action", "wipe the cycle 14 world",
		"--target", "kai-server eco-server", "--gate", "destructive-wipe"}
	if out, err := runGrant(t, seats.sysadmin, args...); err != nil || out != "allow, 0 uses left" {
		t.Fatalf("the executor's check should allow: %q %v", out, err)
	}
	if out, err := runGrant(t, seats.sysadmin, args...); err == nil || out != "deny: spent" {
		t.Fatalf("a second check should deny and exit nonzero: %q %v", out, err)
	}
	if out, err := runGrant(t, seats.director, args...); err == nil || out != "deny: wrong_holder" {
		t.Fatalf("the relaying seat should be refused: %q %v", out, err)
	}
}
