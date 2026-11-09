package main

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
)

// notifyResult is what a notify answered: the id, and how many clients were offered it.
type notifyResult struct {
	ID        string `json:"id"`
	Session   string `json:"session"`
	Delivered int    `json:"delivered"`
}

// notifyKai is the one path the MCP tool uses. A refusal returns as an error, with the
// daemon's reason in front, so the seat reads why it was refused.
func notifyKai(title, body string) (notifyResult, error) {
	token := strings.TrimSpace(os.Getenv(sessionTokenEnv))
	if token == "" {
		return notifyResult{}, withExit(exitUsage, fmt.Errorf("notify speaks for a session aterm launched, "+
			"and %s is not set here", sessionTokenEnv))
	}
	c, err := dialDaemon(false)
	if err != nil {
		return notifyResult{}, withExit(exitMissing, err)
	}
	defer c.Close()
	if !slices.Contains(c.features, notifyFeature) {
		return notifyResult{}, errors.New("the running aterm daemon predates notify. " +
			"It restarts on the upgraded binary after five idle minutes")
	}
	reply, err := c.request(frame{Type: "notify", Token: token, Title: title, Body: body})
	if err != nil {
		if reply.Reason != "" {
			return notifyResult{}, withExit(reply.Code, fmt.Errorf("%s: %w", reply.Reason, err))
		}
		return notifyResult{}, err
	}
	return notifyResult{ID: reply.NotifyID, Session: reply.Session, Delivered: reply.Delivered}, nil
}
