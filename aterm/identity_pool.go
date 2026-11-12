package main

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
)

// identityEnv is the variable agent-compose reads to name a launch or a card.
const identityEnv = "AGENT_COMPOSE_IDENTITY"

// identityPool is a role's one base name and the roster's adjectives. A seat is the base
// name or "<Adjective> <base name>", and nothing here reads the base name's parts.
type identityPool struct {
	Base       string
	Adjectives []string
}

func identityPoolOf(roster rosterDocument, role string) identityPool {
	item, ok := roster.role(role)
	if !ok {
		return identityPool{}
	}
	return identityPool{Base: item.Identity.Name, Adjectives: roster.IdentityAdjectives}
}

// allows says whether name is one a seat of this role may answer to.
func (p identityPool) allows(name string) bool {
	if p.Base == "" {
		return false
	}
	if name == p.Base {
		return true
	}
	adjective, rest, found := strings.Cut(name, " ")
	return found && rest == p.Base && slices.Contains(p.Adjectives, adjective)
}

// shuffleAdjectives orders the draw. A test replaces it to fix the order.
var shuffleAdjectives = func(words []string) {
	rand.Shuffle(len(words), func(i, j int) { words[i], words[j] = words[j], words[i] })
}

// chooseIdentity picks a new seat's name, "" keeping the base name and slot suffix. A
// requested name must be allowed, a resume's preferred one is kept only if free.
func chooseIdentity(pool identityPool, role, requested, preferred string, claim func(string, bool) string) (string, error) {
	free := func(name string) bool {
		base := sessionName(name, role, "")
		return claim == nil || claim(base, true) == base
	}
	keep := func(name string) string {
		if name == pool.Base {
			return ""
		}
		return name
	}
	if requested = strings.TrimSpace(requested); requested != "" {
		if !pool.allows(requested) {
			return "", withExit(exitUsage, fmt.Errorf("%q is not one of %s's names (%q, or an adjective from the roster's list before it)",
				requested, role, pool.Base))
		}
		return keep(requested), nil
	}
	if preferred = strings.TrimSpace(preferred); preferred != "" && pool.allows(preferred) && free(preferred) {
		return keep(preferred), nil
	}
	if len(pool.Adjectives) == 0 || claim == nil {
		return "", nil
	}
	drawn := slices.Clone(pool.Adjectives)
	shuffleAdjectives(drawn)
	for _, adjective := range drawn {
		if name := adjective + " " + pool.Base; free(name) {
			return name, nil
		}
	}
	return "", nil
}

// withIdentityEnv runs argv with the identity variable set, so the name reaches
// the card and the harness launch without touching this process's environment.
func withIdentityEnv(name string, argv []string) []string {
	if name == "" {
		return argv
	}
	return append([]string{"/usr/bin/env", identityEnv + "=" + name}, argv...)
}
