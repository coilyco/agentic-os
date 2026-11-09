package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

// pingKai sends one `notify` as the seat the token names. The daemon decides whether the
// role may, so a refusal comes back with its reason. references/notify.md
func pingKai(title, body string) (frame, error) {
	token := strings.TrimSpace(os.Getenv(sessionTokenEnv))
	if token == "" {
		return frame{}, withExit(exitUsage, fmt.Errorf("aterm notify speaks for a session aterm launched, and %s is not set here", sessionTokenEnv))
	}
	c, err := dialDaemon(false)
	if err != nil {
		return frame{}, withExit(exitMissing, err)
	}
	defer c.Close()
	if !slices.Contains(c.features, notifyFeature) {
		return frame{}, fmt.Errorf("the running aterm daemon predates notify. It restarts on the upgraded binary after five idle minutes")
	}
	reply, err := c.request(frame{Type: "notify", Token: token, Title: title, Body: body})
	if err != nil {
		if reply.Code != 0 {
			return frame{}, withExit(reply.Code, withReason(reply.Reason, err))
		}
		return frame{}, withReason(reply.Reason, err)
	}
	return reply, nil
}
