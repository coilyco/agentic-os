package main

import (
	"strings"
	"testing"
	"time"
)

func wipeSpec() grantSpec {
	return grantSpec{
		Action: "wipe the cycle 14 world", Target: "kai-server eco-server",
		Gates: []string{"destructive-wipe"}, For: []string{"sysadmin-senior"}, TTLSeconds: 3600, Uses: 1,
	}
}

func clockedStore(now *time.Time) *grantStore {
	s := newGrantStore()
	s.now = func() time.Time { return *now }
	return s
}

func wipeCheck(id string) grantCheck {
	return grantCheck{ID: id, Action: "wipe the cycle 14 world", Target: "kai-server eco-server", Gate: "destructive-wipe", Role: "sysadmin-senior"}
}

func TestGrantAllowsTheExactDecisionOnce(t *testing.T) {
	now := time.Now()
	s := clockedStore(&now)
	g := s.mint(wipeSpec(), "prod-director Griffin-Goose", "ask1")
	if v := s.check(wipeCheck(g.ID)); !v.Allow || v.Remaining != 0 {
		t.Fatalf("the exact action should be allowed once: %+v", v)
	}
	if v := s.check(wipeCheck(g.ID)); v.Allow || v.Reason != "spent" {
		t.Fatalf("a second use must be refused as spent: %+v", v)
	}
}

// Each case is one way a gate must refuse. The reason names the first failed condition.
func TestGrantFailsClosed(t *testing.T) {
	now := time.Now()
	s := clockedStore(&now)
	g := s.mint(wipeSpec(), "prod-director Griffin-Goose", "ask1")
	cases := map[string]struct {
		edit   func(*grantCheck)
		reason string
	}{
		"unknown id":            {func(c *grantCheck) { c.ID = "g-nope" }, "unknown_grant"},
		"empty id":              {func(c *grantCheck) { c.ID = "" }, "unknown_grant"},
		"other seat's role":     {func(c *grantCheck) { c.Role = "prod-director" }, "wrong_holder"},
		"no role":               {func(c *grantCheck) { c.Role = "" }, "wrong_holder"},
		"wrong action":          {func(c *grantCheck) { c.Action = "wipe the cycle 15 world" }, "wrong_action"},
		"action as a prefix":    {func(c *grantCheck) { c.Action = "wipe the cycle 14" }, "wrong_action"},
		"wrong target":          {func(c *grantCheck) { c.Target = "ser8 eco-server" }, "wrong_target"},
		"a gate it never named": {func(c *grantCheck) { c.Gate = "credential-read" }, "gate_not_named"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			check := wipeCheck(g.ID)
			tc.edit(&check)
			if v := s.check(check); v.Allow || v.Reason != tc.reason {
				t.Fatalf("want refusal %q, got %+v", tc.reason, v)
			}
		})
	}
	// None of the refusals above spent the use.
	if v := s.check(wipeCheck(g.ID)); !v.Allow {
		t.Fatalf("a refused check must not consume the grant: %+v", v)
	}
}

func TestGrantExpires(t *testing.T) {
	now := time.Now()
	s := clockedStore(&now)
	g := s.mint(wipeSpec(), "prod-director Griffin-Goose", "ask1")
	now = now.Add(time.Hour)
	if v := s.check(wipeCheck(g.ID)); v.Allow || v.Reason != "expired" {
		t.Fatalf("a grant at its expiry must be refused: %+v", v)
	}
	if v := s.check(wipeCheck(g.ID)); v.Reason != "unknown_grant" {
		t.Fatalf("an expired grant is dropped, so it reads as unknown: %+v", v)
	}
}

// A record that did not come from mint carries no provenance, wherever it came from.
func TestGrantWithoutProvenanceIsRefused(t *testing.T) {
	s := newGrantStore()
	spec := wipeSpec()
	if err := spec.normalize(); err != nil {
		t.Fatal(err)
	}
	s.grants["g-forged"] = &grant{
		ID: "g-forged", Spec: spec, Expires: time.Now().Add(time.Hour), Remaining: 1,
	}
	if v := s.check(wipeCheck("g-forged")); v.Allow || v.Reason != "no_provenance" {
		t.Fatalf("a record not made by mint must be refused: %+v", v)
	}
}

func TestGrantSpecRefusesWhatIsUnbounded(t *testing.T) {
	cases := map[string]func(*grantSpec){
		"no action":     func(s *grantSpec) { s.Action = " " },
		"no target":     func(s *grantSpec) { s.Target = "" },
		"no gate":       func(s *grantSpec) { s.Gates = nil },
		"empty gate":    func(s *grantSpec) { s.Gates = []string{""} },
		"repeated gate": func(s *grantSpec) { s.Gates = []string{"a", "a"} },
		"no holder":     func(s *grantSpec) { s.For = nil },
		"no lifetime":   func(s *grantSpec) { s.TTLSeconds = 0 },
		"negative life": func(s *grantSpec) { s.TTLSeconds = -5 },
		"a day":         func(s *grantSpec) { s.TTLSeconds = 24 * 3600 },
		"too many uses": func(s *grantSpec) { s.Uses = maxGrantUses + 1 },
		"negative uses": func(s *grantSpec) { s.Uses = -1 },
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			spec := wipeSpec()
			edit(&spec)
			if err := spec.normalize(); err == nil {
				t.Fatalf("%s was accepted: %+v", name, spec)
			}
		})
	}
	spec := wipeSpec()
	spec.Uses = 0
	if err := spec.normalize(); err != nil || spec.Uses != 1 {
		t.Fatalf("an unset use count defaults to one: %v %+v", err, spec)
	}
}

// The card is rendered from the spec, so everything Kai approves is on it.
func TestGrantAskStatesExactlyWhatIsBound(t *testing.T) {
	ask := wipeSpec().ask("prod-director Griffin-Goose")
	for _, want := range []string{"wipe the cycle 14 world", "kai-server eco-server", "destructive-wipe", "sysadmin-senior", "1h0m0s", "1 use"} {
		if !strings.Contains(ask.Question, want) {
			t.Errorf("the card omits %q: %s", want, ask.Question)
		}
	}
	if len(ask.Options) != 2 || ask.Options[0].Label != "Approve" || ask.AllowOther || ask.Multi {
		t.Fatalf("approve must be option 0 with no free text: %+v", ask)
	}
}
