package main

import (
	"fmt"
	"strings"
)

// envelopeOpen starts every peer message and nothing else, so a line opening
// with it inside a body is escaped. See docs/aterm-daemon.md.
const envelopeOpen = "[from "

// envelope stamps a body with the sender the daemon resolved from its token.
// The sender never supplies this line, and the body cannot forge a second one.
func envelope(role, identity, body string) string {
	return fmt.Sprintf("%s%s %s] %s", envelopeOpen, role, identity, escapeBody(body))
}

// escapeBody neutralises an envelope-shaped line and every control byte, since
// an escape byte would end a bracketed paste and send the rest as keys.
func escapeBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")
	for index, line := range lines {
		line = caretControls(line)
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), envelopeOpen) {
			line = `\` + line
		}
		lines[index] = line
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// caretControls writes each C0 control byte other than tab, and DEL, in caret
// notation, which is how a terminal shows one it is not acting on.
func caretControls(line string) string {
	var builder strings.Builder
	for _, r := range line {
		switch {
		case r == '\t':
			builder.WriteRune(r)
		case r < 0x20:
			builder.WriteByte('^')
			builder.WriteRune(r + '@')
		case r == 0x7f:
			builder.WriteString("^?")
		case r >= 0x80 && r < 0xa0:
			// C1 controls include a one-byte CSI, so they go too.
			builder.WriteString(fmt.Sprintf("<U+%04X>", r))
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
