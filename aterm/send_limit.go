package main

import (
	"fmt"
	"strings"
)

// maxSendBody is the longest message aterm send carries, in bytes. A longer one
// would sever the target's holder link at about 3 MiB (COI-2619).
const maxSendBody = 256 << 10

// errSendTooLong names the limit, so a long message is refused rather than cut.
func errSendTooLong() error {
	return withExit(exitUsage, fmt.Errorf(
		"the message is over %d bytes (256 KiB), the most aterm send carries. "+
			"Write it to a file and send the path, so the seat reads it whole", maxSendBody))
}

// messageTextHead is the most of a body a `message` event carries, in bytes. Why
// it is cut: .agents/skills/tooling-aterm-client/references/message-text.md.
const messageTextHead = 32 << 10

// bodyHead cuts body to the head a `message` event carries, at a rune
// boundary, and reports whether anything was left off.
func bodyHead(body string) (string, bool) {
	if len(body) <= messageTextHead {
		return body, false
	}
	return strings.ToValidUTF8(body[:messageTextHead], ""), true
}
