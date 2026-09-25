package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

const agentsFormat = "aterm.agents.v1"

func newDaemonCommand() *cli.Command {
	return &cli.Command{
		Name:  "daemon",
		Usage: "run the host daemon that owns every session's terminal, in the foreground",
		Description: "A session starts the daemon on its own, so this verb is for a service\n" +
			"manager or for watching one. It exits after five idle minutes.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "socket", Value: daemonSocket(), Usage: "unix socket to serve"},
			&cli.DurationFlag{Name: "idle", Value: daemonIdle, Usage: "exit after this long with no session and no client"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return runDaemon(cmd.String("socket"), cmd.Duration("idle"), cmd.Root().ErrWriter)
		},
	}
}

func newSendCommand() *cli.Command {
	return &cli.Command{
		Name:      "send",
		Usage:     "type a message into another live session, stamped with this session's seat",
		ArgsUsage: "<role|seat|session> <message...>",
		Description: "The daemon stamps `[from <role> <identity>]` from this session's token,\n" +
			"so the sender cannot choose that line. A message of `-` reads stdin.\n" +
			"It waits while Kai is typing in the target and lands after her draft.",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "launch", Usage: "when no session answers to a role, open one and deliver into it"},
			&cli.BoolFlag{Name: "json", Usage: "print the message state as JSON"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			args := cmd.Args().Slice()
			if len(args) < 2 {
				return withExit(exitUsage, fmt.Errorf("aterm send needs a target and a message. `aterm agents` lists targets"))
			}
			body := strings.Join(args[1:], " ")
			if body == "-" {
				raw, err := io.ReadAll(os.Stdin)
				if err != nil {
					return err
				}
				body = string(raw)
			}
			state, err := sendMessage(args[0], body, cmd.Bool("launch"))
			if err != nil {
				return err
			}
			if cmd.Bool("json") {
				encoded, _ := json.MarshalIndent(state, "", "  ")
				_, err = fmt.Fprintf(cmd.Root().Writer, "%s\n", encoded)
				return err
			}
			_, err = fmt.Fprintln(cmd.Root().Writer, describeMessage(state))
			return err
		},
	}
}

// sendMessage is the one path both front doors take, the CLI and the MCP
// tool, so the two cannot drift.
func sendMessage(target, body string, launch bool) (peerMessage, error) {
	token := strings.TrimSpace(os.Getenv(sessionTokenEnv))
	if token == "" {
		return peerMessage{}, withExit(exitUsage, fmt.Errorf(
			"%s is unset: aterm send speaks for a session aterm launched, so run it from inside one", sessionTokenEnv))
	}
	c, err := dialDaemon(false)
	if err != nil {
		return peerMessage{}, withExit(exitMissing, err)
	}
	defer c.Close()
	reply, err := c.request(frame{Type: "send", Token: token, Target: target, Body: body, Launch: launch})
	if err != nil {
		if reply.Code != 0 {
			return peerMessage{}, withExit(reply.Code, err)
		}
		return peerMessage{}, err
	}
	if reply.Message == nil {
		return peerMessage{}, fmt.Errorf("the daemon answered without a message state")
	}
	state := *reply.Message
	if state.State == "launching" {
		if err := launchTarget(target); err != nil {
			return state, fmt.Errorf("the message waits for %s, but opening it failed: %w", target, err)
		}
	}
	return state, nil
}

// launchTarget opens the role the way a person would, through this binary,
// so the roster check and the window are the ordinary ones.
func launchTarget(role string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	output, err := exec.Command(self, role).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func describeMessage(state peerMessage) string {
	where := state.Session
	if where == "" {
		where = state.Target
	}
	line := fmt.Sprintf("%s %s to %s", state.State, state.ID, where)
	if state.Reason != "" {
		line += ": " + state.Reason
	}
	return line
}

func newAgentsCommand() *cli.Command {
	return &cli.Command{
		Name:  "agents",
		Usage: "list the live sessions the daemon holds, the targets `aterm send` takes",
		Flags: []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "machine-readable, as " + agentsFormat}},
		Action: func(_ context.Context, cmd *cli.Command) error {
			views, err := listAgents()
			if err != nil {
				return err
			}
			writer := cmd.Root().Writer
			if cmd.Bool("json") {
				encoded, _ := json.MarshalIndent(agentsDocument(views), "", "  ")
				_, err := fmt.Fprintf(writer, "%s\n", encoded)
				return err
			}
			if len(views) == 0 {
				_, err := fmt.Fprintln(writer, "no live session")
				return err
			}
			self := os.Getenv(sessionNameEnv)
			for _, view := range views {
				marker := " "
				if view.Name == self {
					marker = "*"
				}
				fmt.Fprintf(writer, "%s %-32s %-16s %-8s %s\n", marker, view.Name, view.Identity, view.Seat, agentState(view))
			}
			return nil
		},
	}
}

type agentsDoc struct {
	Format   string        `json:"format"`
	Self     string        `json:"self,omitempty"`
	Sessions []sessionView `json:"sessions"`
}

func agentsDocument(views []sessionView) agentsDoc {
	return agentsDoc{Format: agentsFormat, Self: os.Getenv(sessionNameEnv), Sessions: views}
}

func agentState(view sessionView) string {
	parts := []string{}
	if !view.Ready {
		parts = append(parts, "starting")
	}
	if view.Drafted {
		parts = append(parts, "Kai drafting")
	}
	if view.Pending > 0 {
		parts = append(parts, fmt.Sprintf("%d pending", view.Pending))
	}
	parts = append(parts, fmt.Sprintf("%d client(s), up %s", view.Clients, time.Since(view.Started).Round(time.Second)))
	return strings.Join(parts, ", ")
}

func listAgents() ([]sessionView, error) {
	c, err := dialDaemon(false)
	if err != nil {
		// No daemon means no session it holds, which is an answer, not a fault.
		return nil, nil
	}
	defer c.Close()
	reply, err := c.request(frame{Type: "list"})
	if err != nil {
		return nil, err
	}
	return reply.Sessions, nil
}
