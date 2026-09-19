package main

import "strings"

// The one name every aterm session carries, and the one a launch clears.
// See docs/aterm-bundles.md.
const stableSessionName = "aterm"

func hasNameFlag(arguments []string) bool {
	for _, argument := range arguments {
		if argument == "--name" || argument == "-n" || strings.HasPrefix(argument, "--name=") {
			return true
		}
	}
	return false
}

// claudeSessionName is the name a running claude was started with, read from its
// command line, or "" for any other process. The last flag wins, as it does there.
func claudeSessionName(command string) string {
	tokens := strings.Fields(command)
	// A script or node wrapper puts the interpreter first.
	if !isClaude(tokens, 0) && !(len(tokens) > 1 && interpreters[baseName(tokens[0])] && isClaude(tokens, 1)) {
		return ""
	}
	name := ""
	for index := 0; index < len(tokens); index++ {
		var words []string
		switch token := tokens[index]; {
		case token == "--name" || token == "-n":
			index++
			// The first word is the value whatever it looks like.
			if index < len(tokens) {
				words = append(words, tokens[index])
				index++
			}
		case strings.HasPrefix(token, "--name="):
			words = append(words, strings.TrimPrefix(token, "--name="))
			index++
		default:
			continue
		}
		// The name may hold spaces, which ps flattens, so it runs to the next flag.
		for ; index < len(tokens) && !strings.HasPrefix(tokens[index], "-"); index++ {
			words = append(words, tokens[index])
		}
		index--
		name = strings.Join(words, " ")
	}
	return name
}

var interpreters = map[string]bool{"sh": true, "bash": true, "zsh": true, "node": true, "env": true}

func isClaude(tokens []string, position int) bool {
	return position < len(tokens) && baseName(tokens[position]) == "claude"
}

func baseName(token string) string {
	if slash := strings.LastIndex(token, "/"); slash >= 0 {
		return token[slash+1:]
	}
	return token
}
