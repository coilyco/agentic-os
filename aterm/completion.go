package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

// completeInvocation feeds the shell the same roster the launcher validates
// against, so a slug that turned over stops being completable at all.
func completeInvocation(ctx context.Context, deps commandDeps, cmd *cli.Command) {
	args := cmd.Args().Slice()
	if len(args) > 0 && strings.HasPrefix(args[len(args)-1], "-") {
		cli.DefaultCompleteWithFlags(ctx, cmd)
		return
	}
	// Past the seat there is nothing of ours to offer, and skipping the roster
	// read keeps that keystroke free of a subprocess.
	if len(args) > 1 {
		return
	}
	// Completion runs on a keystroke, so every failure here is silence. A shell
	// that prints an error mid-word is worse than one offering nothing.
	agentCompose, err := requireBinary(deps.lookPath, cmd.String("agent-compose-bin"))
	if err != nil {
		return
	}
	roster, err := loadRoster(ctx, deps, agentCompose)
	if err != nil {
		return
	}
	writeCompletions(cmd.Root().Writer, roster, args)
}

func writeCompletions(writer io.Writer, roster rosterDocument, args []string) {
	if len(args) > 1 {
		return
	}
	if len(args) == 0 {
		for _, role := range roster.Items {
			if len(role.nativeSeats()) == 0 {
				continue
			}
			fmt.Fprintf(writer, "%s:%s\n", role.Slug, completionDetail(role.DisplayName))
		}
		return
	}
	role, ok := roster.role(strings.TrimSpace(args[0]))
	if !ok {
		return
	}
	for _, seat := range role.nativeSeats() {
		fmt.Fprintf(writer, "%s:%s\n", seat.Harness, completionDetail(seatDetail(seat)))
	}
}

// completionDetail keeps a description on one colon-free line, because both
// shipped shell scripts split each candidate on its first colon.
func completionDetail(value string) string {
	value = strings.ReplaceAll(value, ":", " ")
	if index := strings.IndexAny(value, "\r\n"); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}

func seatDetail(seat rosterSeat) string {
	detail := strings.TrimSpace(seat.Name)
	subject, _, _ := strings.Cut(strings.TrimSpace(seat.Pronouns), "/")
	if subject = strings.TrimSpace(subject); subject != "" && detail != "" {
		detail += " [" + subject + "]"
	}
	if tier := strings.TrimSpace(seat.Tier); tier != "" {
		if detail == "" {
			return tier
		}
		detail += " // " + tier
	}
	return detail
}

// sessionCompletionTimeout bounds the daemon read, because `request` waits for
// a reply with no deadline and a stuck daemon would otherwise hold the shell.
const sessionCompletionTimeout = 750 * time.Millisecond

// completeSessionName offers the live sessions the daemon holds, the list
// `aterm agents` prints, for the commands whose first argument names one.
func completeSessionName(ctx context.Context, cmd *cli.Command) {
	args := cmd.Args().Slice()
	if len(args) > 0 && strings.HasPrefix(args[len(args)-1], "-") {
		cli.DefaultCompleteWithFlags(ctx, cmd)
		return
	}
	// Only the first argument is a session, and for `send` what follows is the message.
	if len(args) > 0 {
		return
	}
	writeSessionCompletions(cmd.Root().Writer, liveSessionViews(sessionCompletionTimeout))
}

// liveSessionViews is silent on every failure, as the roster completion is.
func liveSessionViews(timeout time.Duration) []sessionView {
	type listed struct {
		views []sessionView
		err   error
	}
	done := make(chan listed, 1)
	go func() {
		views, err := listAgents()
		done <- listed{views, err}
	}()
	select {
	case result := <-done:
		if result.err != nil {
			return nil
		}
		return result.views
	case <-time.After(timeout):
		return nil
	}
}

func writeSessionCompletions(writer io.Writer, views []sessionView) {
	sorted := append([]sessionView(nil), views...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	for _, view := range sorted {
		name := strings.TrimSpace(view.Name)
		if name == "" {
			continue
		}
		// A bare colon would end the name early, and `\:` is a literal one to `_describe`.
		name = strings.ReplaceAll(name, ":", `\:`)
		if detail := sessionCompletionDetail(view); detail != "" {
			fmt.Fprintf(writer, "%s:%s\n", name, detail)
			continue
		}
		fmt.Fprintln(writer, name)
	}
}

func sessionCompletionDetail(view sessionView) string {
	role := strings.TrimSpace(view.Role)
	identity := strings.TrimSpace(view.Identity)
	switch {
	case role != "" && identity != "":
		return completionDetail(role + " // " + identity)
	case role != "":
		return completionDetail(role)
	}
	return completionDetail(identity)
}
