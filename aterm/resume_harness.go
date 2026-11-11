package main

import (
	"fmt"
	"slices"
)

// resumeHarnessArgs is what follows `--` on a resume launch, in the harness's own words
// (COI-3609). The id is the record's, else found in the harness's own files.
func resumeHarnessArgs(entry ledgerEntry, all []ledgerEntry) ([]string, error) {
	id := entry.Conversation
	if id == "" {
		var err error
		if id, err = findConversation(entry, all); err != nil {
			return nil, withExit(exitCodeFor(err), fmt.Errorf("%s has no %s conversation to resume: %w", entry.Name, entry.Seat, err))
		}
	}
	switch entry.Seat {
	case "claude":
		return []string{"--resume", id}, nil
	case "codex":
		return []string{"resume", id}, nil
	case "opencode":
		return []string{"--session", id}, nil
	case "goose":
		// agent-compose reads the first word after the scope as goose's verb.
		return []string{"session", "--resume", "--session-id", id}, nil
	}
	return nil, withExit(exitUsage, fmt.Errorf("%s runs %q, which aterm cannot resume", entry.Name, entry.Seat))
}

// findConversation is the id of the harness conversation a seat held, from its own files.
func findConversation(entry ledgerEntry, all []ledgerEntry) (string, error) {
	switch entry.Seat {
	case "codex":
		path, err := codexRollout(entry, all)
		if err != nil {
			return "", err
		}
		return codexSessionID(path)
	case "opencode":
		return opencodeSession(opencodeDB(entry), entry, all)
	case "goose":
		return gooseSession(gooseDB(entry), entry, all)
	}
	return "", withExit(exitUsage, fmt.Errorf("claude records its conversation id at launch, and this record has none: %w", errNoTranscript))
}

// harnessConversation is the id a launch's argv resumes, so a resumed seat's record
// names it and the next resume does not search.
func harnessConversation(seat string, argv []string) string {
	if seat == "claude" || seat == "" {
		return conversationOf(argv)
	}
	// Harness words follow `agent-compose launch <role> <seat>`. aos's own session id comes
	// earlier in the argv, so only what follows that triple is read.
	tail := launchExtras(seat, argv)
	value := func(flags ...string) string {
		for index := 0; index+1 < len(tail); index++ {
			if slices.Contains(flags, tail[index]) {
				return tail[index+1]
			}
		}
		return ""
	}
	switch seat {
	case "codex":
		if slices.Contains(tail, "resume") {
			if found := value("resume"); conversationFlag.MatchString(found) {
				return found
			}
		}
	case "opencode":
		if found := value("--session", "-s"); opencodeSessionID.MatchString(found) {
			return found
		}
	case "goose":
		if slices.Contains(tail, "--resume") || slices.Contains(tail, "-r") {
			if found := value("--session-id"); gooseSessionID.MatchString(found) {
				return found
			}
		}
	}
	return ""
}

// launchExtras is what follows `launch <role> <seat>` in an argv, less a leading `--`.
func launchExtras(seat string, argv []string) []string {
	for index := len(argv) - 3; index >= 0; index-- {
		if argv[index] == "launch" && argv[index+2] == seat {
			tail := argv[index+3:]
			if len(tail) > 0 && tail[0] == "--" {
				tail = tail[1:]
			}
			return tail
		}
	}
	return nil
}
