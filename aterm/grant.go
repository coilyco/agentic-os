package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// A grant carries one decision Kai made to the seats that execute it, minted only
// from her answer. See .agents/skills/tooling-aterm-client/references/grants.md.

const (
	maxGrantTTL  = 12 * time.Hour
	maxGrantUses = 10
	// grantProvenance is the one value mint writes, so a record without it was
	// not made from Kai's answer and every check refuses it.
	grantProvenance = "client-answer"
)

// grantSpec is what a seat asks Kai to approve. The card Kai sees is rendered from
// these fields by the daemon, never from seat-supplied prose.
type grantSpec struct {
	Action     string   `json:"action"`
	Target     string   `json:"target"`
	Gates      []string `json:"gates"`
	For        []string `json:"for"`
	TTLSeconds int      `json:"ttl_seconds"`
	Uses       int      `json:"uses,omitempty"`
}

// normalize trims, defaults uses to one, and refuses anything unbounded or vague.
func (s *grantSpec) normalize() error {
	s.Action = strings.TrimSpace(s.Action)
	s.Target = strings.TrimSpace(s.Target)
	if s.Action == "" || s.Target == "" {
		return errors.New("a grant names an exact action and an exact target")
	}
	var err error
	if s.Gates, err = cleanNames("gate", s.Gates); err != nil {
		return err
	}
	if s.For, err = cleanNames("holder role", s.For); err != nil {
		return err
	}
	ttl := time.Duration(s.TTLSeconds) * time.Second
	if ttl <= 0 || ttl > maxGrantTTL {
		return fmt.Errorf("a grant lasts more than 0 and at most %s", maxGrantTTL)
	}
	if s.Uses == 0 {
		s.Uses = 1
	}
	if s.Uses < 1 || s.Uses > maxGrantUses {
		return fmt.Errorf("a grant allows 1 to %d uses", maxGrantUses)
	}
	return nil
}

func cleanNames(what string, names []string) ([]string, error) {
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("a %s cannot be empty", what)
		}
		if slices.Contains(out, name) {
			return nil, fmt.Errorf("duplicate %s %q", what, name)
		}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("a grant names at least one %s", what)
	}
	return out, nil
}

// ask renders the question Kai answers. Approve is always option 0.
func (s grantSpec) ask(asker string) choiceAsk {
	ttl := (time.Duration(s.TTLSeconds) * time.Second).String()
	uses := fmt.Sprintf("%d use", s.Uses)
	if s.Uses != 1 {
		uses += "s"
	}
	question := fmt.Sprintf(
		"%s asks to be allowed: %s, on %s. Gates it covers: %s. Seats that may present it: %s. Valid %s, %s.",
		asker, s.Action, s.Target, strings.Join(s.Gates, ", "), strings.Join(s.For, ", "), ttl, uses,
	)
	return choiceAsk{
		Header:   "Approval",
		Question: question,
		Options: []choiceOption{
			{Label: "Approve", Description: "mint the grant for exactly this"},
			{Label: "Deny", Description: "no grant is made"},
		},
	}
}

// grant is the record. Remaining counts down on each allowed check.
type grant struct {
	ID         string
	Spec       grantSpec
	Asker      string
	AskID      string
	Decided    time.Time
	Expires    time.Time
	Remaining  int
	Provenance string
}

// grantCheck is one gate asking whether the record covers what it is about to do.
type grantCheck struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	Target string `json:"target"`
	Gate   string `json:"gate"`
	// Role is the presenting seat's role. The daemon fills it from the token.
	Role string `json:"role,omitempty"`
}

// grantVerdict is what the executing seat sees. Reason is a stable code on a refusal.
type grantVerdict struct {
	Allow     bool      `json:"allow"`
	Reason    string    `json:"reason,omitempty"`
	Remaining int       `json:"remaining,omitempty"`
	Expires   time.Time `json:"expires,omitempty"`
}

// grantStore holds grants in memory. A restart voids them, which fails closed.
type grantStore struct {
	mu     sync.Mutex
	grants map[string]*grant
	now    func() time.Time
}

func newGrantStore() *grantStore {
	return &grantStore{grants: map[string]*grant{}, now: time.Now}
}

// mint records a decision. Only the answer path calls it, after the typing guard.
func (s *grantStore) mint(spec grantSpec, asker, askID string) grant {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now().UTC()
	g := &grant{
		ID:         "g-" + randomID(8),
		Spec:       spec,
		Asker:      asker,
		AskID:      askID,
		Decided:    now,
		Expires:    now.Add(time.Duration(spec.TTLSeconds) * time.Second),
		Remaining:  spec.Uses,
		Provenance: grantProvenance,
	}
	s.grants[g.ID] = g
	return *g
}

// revoke removes a grant, for a mint whose ask had already settled.
func (s *grantStore) revoke(id string) {
	s.mu.Lock()
	delete(s.grants, id)
	s.mu.Unlock()
}

// check refuses unless every condition holds, in this order, and consumes one
// use only on an allow. A refusal names the first condition that failed.
func (s *grantStore) check(req grantCheck) grantVerdict {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.grants[strings.TrimSpace(req.ID)]
	switch {
	case g == nil:
		return grantVerdict{Reason: "unknown_grant"}
	case g.Provenance != grantProvenance:
		return grantVerdict{Reason: "no_provenance"}
	case !slices.Contains(g.Spec.For, req.Role):
		// Ahead of expiry and uses, so a seat not named learns nothing of its state.
		return grantVerdict{Reason: "wrong_holder"}
	case !s.now().Before(g.Expires):
		delete(s.grants, g.ID)
		return grantVerdict{Reason: "expired"}
	case g.Remaining < 1:
		return grantVerdict{Reason: "spent"}
	case strings.TrimSpace(req.Action) != g.Spec.Action:
		return grantVerdict{Reason: "wrong_action"}
	case strings.TrimSpace(req.Target) != g.Spec.Target:
		return grantVerdict{Reason: "wrong_target"}
	case !slices.Contains(g.Spec.Gates, strings.TrimSpace(req.Gate)):
		return grantVerdict{Reason: "gate_not_named"}
	}
	g.Remaining--
	return grantVerdict{Allow: true, Remaining: g.Remaining, Expires: g.Expires}
}
