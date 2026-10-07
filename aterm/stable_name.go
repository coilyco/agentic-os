package main

import (
	"strconv"
	"strings"
)

// sessionName names a session for who answers, not the harness: role slug, identity
// slug, then the pool slot of a concurrent instance. See docs/aterm-daemon.md.
func sessionName(name, role, instance string) string {
	if role == "" {
		return ""
	}
	if instance == "" {
		return role + "-" + slugify(name)
	}
	return role + "-" + slugify(name) + "-" + instance
}

// slugify lowercases a value and collapses any run of non-alphanumeric
// characters to a single hyphen, trimming a leading or trailing one.
func slugify(value string) string {
	var builder strings.Builder
	dashed := true // suppresses a leading hyphen
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			dashed = false
			continue
		}
		if !dashed {
			builder.WriteByte('-')
			dashed = true
		}
	}
	return strings.TrimRight(builder.String(), "-")
}

func hasNameFlag(arguments []string) bool {
	for _, argument := range arguments {
		if argument == "--name" || argument == "-n" || strings.HasPrefix(argument, "--name=") {
			return true
		}
	}
	return false
}

// poolName is the first of base, base-2, base-3 that taken does not refuse. The
// daemon and a client facing an older daemon share it.
func poolName(base string, taken func(string) bool) string {
	if !taken(base) {
		return base
	}
	for slot := 2; ; slot++ {
		if name := base + "-" + strconv.Itoa(slot); !taken(name) {
			return name
		}
	}
}

// poolSlot is the suffix a pool name carries past its base, "" for the base.
func poolSlot(base, name string) string {
	return strings.TrimPrefix(strings.TrimPrefix(name, base), "-")
}
