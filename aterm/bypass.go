package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

// bypassFooter is what claude's status line reads once a session is in bypass.
const bypassFooter = "bypass permissions on"

// bypassItem is one live claude seat and what the verb does with it.
type bypassItem struct {
	Name, Role string
	Move       bool
	Reason     string
	Entry      ledgerEntry
}

// bypassDeps are what the verb acts through, so a test needs no daemon or process.
type bypassDeps struct {
	views       func() ([]sessionView, error)
	entries     func() []ledgerEntry
	inBypass    func(name string) bool
	closeSeat   func(name string) error
	gone        func(name string) bool
	relaunch    func(entry ledgerEntry) error
	self        string
	includeBusy bool
}

// planBypass decides each live claude seat. It never moves the caller, a seat with no
// recorded conversation or a draft, or a busy one unless asked.
func planBypass(deps bypassDeps) ([]bypassItem, error) {
	views, err := deps.views()
	if err != nil {
		return nil, err
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Name < views[j].Name })
	entries := deps.entries()
	var plan []bypassItem
	for _, view := range views {
		if view.Seat != "claude" {
			continue
		}
		item := bypassItem{Name: view.Name, Role: view.Role}
		entry, found := latestWithConversation(entries, view.Name)
		item.Entry = entry
		switch {
		case view.Name == deps.self:
			item.Reason = "this is the calling session"
		case !found:
			item.Reason = "no recorded conversation to resume"
		case view.Drafted || view.Pending > 0:
			item.Reason = "holds a draft or undelivered messages"
		case !deps.includeBusy && view.State != "idle" && view.State != "":
			item.Reason = "state is " + view.State + ", pass --busy to include it"
		case deps.inBypass(view.Name):
			item.Reason = "already in bypass permissions"
		default:
			item.Move = true
		}
		plan = append(plan, item)
	}
	return plan, nil
}

func latestWithConversation(entries []ledgerEntry, name string) (ledgerEntry, bool) {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Name == name && entries[i].Conversation != "" {
			return entries[i], true
		}
	}
	return ledgerEntry{}, false
}

// runBypass moves each planned seat: close it, wait for it to go, reopen it headless.
func runBypass(w io.Writer, deps bypassDeps, dryRun bool) error {
	plan, err := planBypass(deps)
	if err != nil {
		return err
	}
	failed := 0
	for _, item := range plan {
		switch {
		case !item.Move:
			fmt.Fprintf(w, "kept    %s: %s\n", item.Name, item.Reason)
		case dryRun:
			fmt.Fprintf(w, "would move %s (%s)\n", item.Name, item.Role)
		default:
			if err := moveSeat(deps, item); err != nil {
				failed++
				fmt.Fprintf(w, "FAILED  %s: %v\n", item.Name, err)
				continue
			}
			fmt.Fprintf(w, "moved   %s (%s)\n", item.Name, item.Role)
		}
	}
	if len(plan) == 0 {
		fmt.Fprintln(w, "no live claude seats")
	}
	if failed > 0 {
		return fmt.Errorf("%d seat(s) failed. Each one that failed before its close is still live", failed)
	}
	return nil
}

func moveSeat(deps bypassDeps, item bypassItem) error {
	if err := deps.closeSeat(item.Name); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	for waited := 0; !deps.gone(item.Name); waited++ {
		if waited == 30 {
			return fmt.Errorf("still live 30s after close, so it was not reopened")
		}
		time.Sleep(time.Second)
	}
	if err := deps.relaunch(item.Entry); err != nil {
		return fmt.Errorf("closed but not reopened, `aterm resume %s` retries it: %w", item.Name, err)
	}
	return nil
}

// screenShowsBypass reads the footer the proof in COI-2739 names, and is false when the
// status cannot be read, so an unreadable seat is moved rather than trusted.
func screenShowsBypass(name string) bool {
	status, err := sessionStatusOf(name, 40)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(strings.Join(status.Screen, "\n")), bypassFooter)
}

func newBypassCommand() *cli.Command {
	return &cli.Command{
		Name:  "bypass",
		Usage: "move every live claude seat to bypass permissions, keeping each conversation",
		Description: "Each eligible seat is closed and reopened headless with --resume <conversation> and\n" +
			"--permission-mode " + defaultPermissionMode + ", so its context carries over. A seat already in bypass\n" +
			"stays. A seat that is busy, holds a draft, has no recorded conversation, or is the\n" +
			"caller is skipped and named. Reopening headless drops a window, and `aterm attach` returns.",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "dry-run", Usage: "name what would move without closing anything"},
			&cli.BoolFlag{Name: "busy", Usage: "also move seats that are mid-turn or on a prompt"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			self, err := os.Executable()
			if err != nil {
				return err
			}
			deps := bypassDeps{
				views:       listAgents,
				entries:     func() []ledgerEntry { return readLedger(ledgerDir()) },
				inBypass:    screenShowsBypass,
				closeSeat:   func(name string) error { _, _, err := closeSession(name, false); return err },
				gone:        func(name string) bool { return !liveSessionNames()[name] },
				self:        strings.TrimSpace(os.Getenv(sessionNameEnv)),
				includeBusy: cmd.Bool("busy"),
				relaunch: func(entry ledgerEntry) error {
					args, err := resumeArgs(entry, true, false)
					if err != nil {
						return err
					}
					args = append(args, harnessArgs(defaultPermissionMode, nil)...)
					return resumeRun(self, args, cmd.Root().Writer, cmd.Root().ErrWriter)
				},
			}
			return runBypass(cmd.Root().Writer, deps, cmd.Bool("dry-run"))
		},
	}
}
