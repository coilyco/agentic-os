package main

import "fmt"

// maxSendBody is the longest message aterm send carries, in bytes. A longer one
// would sever the target's holder link at about 3 MiB (COI-2619).
const maxSendBody = 256 << 10

// errSendTooLong names the limit, so a long message is refused rather than cut.
func errSendTooLong() error {
	return withExit(exitUsage, fmt.Errorf(
		"the message is over %d bytes (256 KiB), the most aterm send carries. "+
			"Write it to a file and send the path, so the seat reads it whole", maxSendBody))
}
