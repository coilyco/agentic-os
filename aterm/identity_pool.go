package main

import (
	"fmt"
	"slices"
	"strings"
)

// identityEnv is the variable agent-compose reads to name a launch or a card.
const identityEnv = "AGENT_COMPOSE_IDENTITY"

// identityPool is every name a role's live instances take, primary first.
func identityPool(roster rosterDocument, role string) []string {
	for _, item := range roster.Items {
		if item.Slug == role {
			return append([]string{item.Identity.Name}, item.Identity.Alternates...)
		}
	}
	return nil
}

// chooseIdentity picks the first name in the pool no live session holds. ""
// keeps the primary and the slot suffix. See docs/aterm-daemon.md.
func chooseIdentity(pool []string, role, requested string, claim func(string, bool) string) (string, error) {
	if requested = strings.TrimSpace(requested); requested != "" {
		if !slices.Contains(pool, requested) {
			return "", withExit(exitUsage, fmt.Errorf("%q is not one of %s's names (%s)",
				requested, role, strings.Join(pool, ", ")))
		}
		if requested == pool[0] {
			return "", nil
		}
		return requested, nil
	}
	if len(pool) < 2 || claim == nil {
		return "", nil
	}
	for index, name := range pool {
		base := sessionName(name, role, "")
		if claim(base, true) != base {
			continue
		}
		if index == 0 {
			return "", nil
		}
		return name, nil
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
