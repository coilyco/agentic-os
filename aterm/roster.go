package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	catalogFormat = "agent-compose.catalog.v1"
	// The machine twin of `--list`. aterm's own contract rather than a relay of
	// the catalogue, because the launchable view is what aterm knows.
	rosterFormat = "aterm.roster.v1"
)

type listedRole struct {
	Slug          string         `json:"slug"`
	DisplayName   string         `json:"display_name"`
	Purpose       string         `json:"purpose"`
	FavoriteColor string         `json:"favorite_color"`
	Identity      rosterIdentity `json:"identity"`
	Launchable    bool           `json:"launchable"`
	Seats         []rosterSeat   `json:"seats"`
}

type listedRoster struct {
	Format string       `json:"format"`
	Roles  []listedRole `json:"roles"`
}

// nativeHarnesses is the set `agent-compose launch` accepts. A catalogue seat
// outside it has nothing to start, so aterm refuses before opening a window.
var nativeHarnesses = []string{"claude", "codex", "goose", "opencode"}

type rosterSeat struct {
	Key      string `json:"key"`
	Harness  string `json:"harness"`
	Name     string `json:"name"`
	Pronouns string `json:"pronouns"`
	Tier     string `json:"tier"`
}

type rosterIdentity struct {
	Name     string `json:"name"`
	Pronouns string `json:"pronouns"`
}

type rosterRole struct {
	Slug          string         `json:"slug"`
	DisplayName   string         `json:"display_name"`
	Purpose       string         `json:"purpose"`
	Identity      rosterIdentity `json:"identity"`
	Seats         []rosterSeat   `json:"seats"`
	Personalities []string       `json:"personalities"`
	FavoriteColor string         `json:"favorite_color"`
	// An archived role arrives fully populated, native seats included, so this
	// flag is all that separates it from a live one.
	Archived bool `json:"archived,omitempty"`
}

type rosterDocument struct {
	Format string       `json:"format"`
	Items  []rosterRole `json:"items"`
}

func isNativeHarness(value string) bool {
	for _, harness := range nativeHarnesses {
		if harness == value {
			return true
		}
	}
	return false
}

// nativeSeats keeps catalogue order so the first entry is the frontier seat.
func (r rosterRole) nativeSeats() []rosterSeat {
	seats := make([]rosterSeat, 0, len(r.Seats))
	for _, seat := range r.Seats {
		if isNativeHarness(seat.Harness) {
			seats = append(seats, seat)
		}
	}
	return seats
}

func (r rosterRole) label() string {
	label := strings.TrimSpace(r.DisplayName)
	if label == "" {
		return r.Slug
	}
	return r.Slug + " // " + label
}

func (d rosterDocument) role(slug string) (rosterRole, bool) {
	for _, item := range d.Items {
		if item.Slug == slug {
			return item, true
		}
	}
	return rosterRole{}, false
}

func (d rosterDocument) slugs() []string {
	slugs := make([]string, 0, len(d.Items))
	for _, item := range d.Items {
		slugs = append(slugs, item.Slug)
	}
	return slugs
}

// listRoster is the launchable projection: every live role, and for each one
// only the seats `agent-compose launch` can actually start.
func listRoster(document rosterDocument) listedRoster {
	roles := make([]listedRole, 0, len(document.Items))
	for _, item := range document.Items {
		seats := item.nativeSeats()
		roles = append(roles, listedRole{
			Slug:          item.Slug,
			DisplayName:   item.DisplayName,
			Purpose:       item.Purpose,
			FavoriteColor: item.FavoriteColor,
			Identity:      item.Identity,
			Launchable:    len(seats) > 0,
			Seats:         seats,
		})
	}
	return listedRoster{Format: rosterFormat, Roles: roles}
}

func loadRoster(ctx context.Context, deps commandDeps, agentCompose string) (rosterDocument, error) {
	command := []string{agentCompose, "catalog", "roles", "--json"}
	raw, err := whileWaiting2(deps.notice, command, func() ([]byte, error) {
		return deps.output(ctx, command[0], command[1:]...)
	})
	if err != nil {
		return rosterDocument{}, fmt.Errorf("load the Agent Compose roster: %w", err)
	}
	return parseRoster(raw)
}

func parseRoster(raw []byte) (rosterDocument, error) {
	var document rosterDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return rosterDocument{}, fmt.Errorf("decode the Agent Compose roster: %w", err)
	}
	if document.Format != catalogFormat {
		return rosterDocument{}, fmt.Errorf(
			"the Agent Compose roster has unsupported contract %q",
			document.Format,
		)
	}
	if len(document.Items) == 0 {
		return rosterDocument{}, fmt.Errorf("the Agent Compose roster is empty")
	}
	for _, item := range document.Items {
		if !safeRoleSlug(item.Slug) {
			return rosterDocument{}, fmt.Errorf("the Agent Compose roster has unsafe role %q", item.Slug)
		}
	}
	// Dropping archived roles at this seam is what takes a retired seat out of
	// the picker, completion, a named launch, and bundles at once. docs/aterm.md.
	live := make([]rosterRole, 0, len(document.Items))
	for _, item := range document.Items {
		if !item.Archived {
			live = append(live, item)
		}
	}
	if len(live) == 0 {
		return rosterDocument{}, fmt.Errorf("every role in the Agent Compose roster is archived")
	}
	document.Items = live
	return document, nil
}

// unknownRoleError names the live roster rather than only refusing, because the
// slugs turn over and a stale one in muscle memory is the common way to get here.
func unknownRoleError(slug string, document rosterDocument) error {
	message := &strings.Builder{}
	fmt.Fprintf(message, "%q is not a live role", slug)
	if suggestions := suggest(slug, document.slugs()); len(suggestions) > 0 {
		fmt.Fprintf(message, ". Did you mean %s?", humanList(suggestions))
	}
	message.WriteString("\n\nlive roles:")
	for _, item := range document.Items {
		fmt.Fprintf(message, "\n  %s", item.label())
	}
	return withExit(exitOffRoster, fmt.Errorf("%s", message.String()))
}

func unknownSeatError(seat string, role rosterRole) error {
	message := &strings.Builder{}
	native := role.nativeSeats()
	names := make([]string, 0, len(native))
	for _, item := range native {
		names = append(names, item.Harness)
	}
	if seatInRole(seat, role) {
		fmt.Fprintf(
			message,
			"%s seat %q has no native harness to launch",
			role.Slug,
			seat,
		)
	} else {
		fmt.Fprintf(message, "%q is not a %s seat", seat, role.Slug)
		if suggestions := suggest(seat, names); len(suggestions) > 0 {
			fmt.Fprintf(message, ". Did you mean %s?", humanList(suggestions))
		}
	}
	fmt.Fprintf(message, "\n\nlaunchable %s seats: %s", role.Slug, strings.Join(names, ", "))
	return withExit(exitOffRoster, fmt.Errorf("%s", message.String()))
}

func seatInRole(seat string, role rosterRole) bool {
	for _, item := range role.Seats {
		if item.Harness == seat {
			return true
		}
	}
	return false
}

func humanList(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	case 2:
		return values[0] + " or " + values[1]
	}
	return strings.Join(values[:len(values)-1], ", ") + ", or " + values[len(values)-1]
}

// suggest ranks candidates by edit distance and keeps only near misses, so a
// slug that shares nothing with the roster produces the plain list instead.
func suggest(value string, candidates []string) []string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return nil
	}
	type scored struct {
		candidate string
		distance  int
	}
	limit := suggestionLimit(len(value))
	ranked := make([]scored, 0, len(candidates))
	for _, candidate := range candidates {
		distance := editDistance(value, strings.ToLower(candidate))
		if strings.HasPrefix(candidate, value) || strings.HasPrefix(value, candidate) {
			distance = 0
		}
		if distance <= limit {
			ranked = append(ranked, scored{candidate: candidate, distance: distance})
		}
	}
	sort.SliceStable(ranked, func(first, second int) bool {
		if ranked[first].distance != ranked[second].distance {
			return ranked[first].distance < ranked[second].distance
		}
		return ranked[first].candidate < ranked[second].candidate
	})
	suggestions := make([]string, 0, len(ranked))
	for _, item := range ranked {
		suggestions = append(suggestions, item.candidate)
		if len(suggestions) == 3 {
			break
		}
	}
	return suggestions
}

// suggestionLimit scales with length because a fixed distance turns every short
// slug into a neighbour of every other one. "ops" is not a near miss for "director".
func suggestionLimit(length int) int {
	switch {
	case length <= 4:
		return 1
	case length <= 8:
		return 2
	default:
		return 3
	}
}

func editDistance(first, second string) int {
	firstRunes := []rune(first)
	secondRunes := []rune(second)
	previous := make([]int, len(secondRunes)+1)
	current := make([]int, len(secondRunes)+1)
	for index := range previous {
		previous[index] = index
	}
	for firstIndex := 1; firstIndex <= len(firstRunes); firstIndex++ {
		current[0] = firstIndex
		for secondIndex := 1; secondIndex <= len(secondRunes); secondIndex++ {
			cost := 1
			if firstRunes[firstIndex-1] == secondRunes[secondIndex-1] {
				cost = 0
			}
			current[secondIndex] = minimum(
				current[secondIndex-1]+1,
				previous[secondIndex]+1,
				previous[secondIndex-1]+cost,
			)
		}
		previous, current = current, previous
	}
	return previous[len(secondRunes)]
}

func minimum(values ...int) int {
	smallest := values[0]
	for _, value := range values[1:] {
		if value < smallest {
			smallest = value
		}
	}
	return smallest
}

func safeRoleSlug(value string) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}
