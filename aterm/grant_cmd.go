package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
)

// grantFrame sends one frame from this session and returns the reply.
func grantFrame(message frame) (frame, error) {
	token := strings.TrimSpace(os.Getenv(sessionTokenEnv))
	if token == "" {
		return frame{}, withExit(exitUsage, fmt.Errorf(
			"%s is unset: aterm grant speaks for a session aterm launched, so run it from inside one", sessionTokenEnv))
	}
	c, err := dialDaemon(false)
	if err != nil {
		return frame{}, withExit(exitMissing, err)
	}
	defer c.Close()
	message.Token = token
	reply, err := c.request(message)
	if err != nil && reply.Code != 0 {
		return frame{}, withExit(reply.Code, err)
	}
	return reply, err
}

func newGrantCommand() *cli.Command {
	return &cli.Command{
		Name:  "grant",
		Usage: "carry one of Kai's decisions to the seats that execute it",
		Description: "A grant binds Kai's answer to an exact action, target and set of gates for a bounded\n" +
			"time and number of uses. Only Kai's answer mints one.\n" +
			"See .agents/skills/tooling-aterm-client/references/grants.md.",
		Commands: []*cli.Command{newGrantRequestCommand(), newGrantCheckCommand()},
	}
}

func newGrantRequestCommand() *cli.Command {
	return &cli.Command{
		Name:  "request",
		Usage: "ask Kai for a grant and print its id once she approves",
		Description: "The daemon renders the question from these fields, so what Kai approves is what is\n" +
			"bound. It exits 1 when she denies, or the ask is cancelled or times out.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "action", Required: true, Usage: "the exact action to approve"},
			&cli.StringFlag{Name: "target", Required: true, Usage: "the exact target it acts on"},
			&cli.StringSliceFlag{Name: "gate", Required: true, Usage: "a gate the grant covers, repeatable"},
			&cli.StringSliceFlag{Name: "for", Required: true, Usage: "a role that may present it, repeatable"},
			&cli.DurationFlag{Name: "ttl", Required: true, Usage: "how long it stays valid, at most 12h"},
			&cli.IntFlag{Name: "uses", Value: 1, Usage: "how many gate checks it allows"},
			&cli.BoolFlag{Name: "json", Usage: "print the answer as JSON"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			spec := grantSpec{
				Action: cmd.String("action"), Target: cmd.String("target"),
				Gates: cmd.StringSlice("gate"), For: cmd.StringSlice("for"),
				TTLSeconds: int(cmd.Duration("ttl").Seconds()), Uses: int(cmd.Int("uses")),
			}
			reply, err := grantFrame(frame{Type: "grant_request", Grant: &spec})
			if err != nil {
				return err
			}
			if reply.Answer == nil {
				return fmt.Errorf("the daemon settled the grant without an answer")
			}
			writer := cmd.Root().Writer
			if cmd.Bool("json") {
				encoded, _ := json.MarshalIndent(reply.Answer, "", "  ")
				fmt.Fprintf(writer, "%s\n", encoded)
			} else if reply.Answer.GrantID != "" {
				fmt.Fprintln(writer, reply.Answer.GrantID)
			}
			if reply.Answer.GrantID == "" {
				return fmt.Errorf("no grant: the ask was %s %s", strings.ReplaceAll(reply.Answer.State, "_", " "), strings.Join(reply.Answer.Labels, ","))
			}
			return nil
		},
	}
}

func newGrantCheckCommand() *cli.Command {
	return &cli.Command{
		Name:      "check",
		Usage:     "ask whether a grant covers what this gate is about to do, and spend one use",
		ArgsUsage: "<grant id>",
		Description: "It prints allow or deny with the reason and exits 1 on a deny. The seat's role comes\n" +
			"from its token, so a relayed id is useless to a seat the grant does not name.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "action", Required: true, Usage: "the exact action about to run"},
			&cli.StringFlag{Name: "target", Required: true, Usage: "the exact target"},
			&cli.StringFlag{Name: "gate", Required: true, Usage: "the gate asking"},
			&cli.BoolFlag{Name: "json", Usage: "print the verdict as JSON"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return withExit(exitUsage, fmt.Errorf("aterm grant check needs one grant id"))
			}
			check := grantCheck{
				ID: cmd.Args().First(), Action: cmd.String("action"), Target: cmd.String("target"), Gate: cmd.String("gate"),
			}
			reply, err := grantFrame(frame{Type: "grant_check", Check: &check})
			if err != nil {
				return err
			}
			if reply.Verdict == nil {
				return fmt.Errorf("the daemon returned no verdict")
			}
			writer := cmd.Root().Writer
			if cmd.Bool("json") {
				encoded, _ := json.MarshalIndent(reply.Verdict, "", "  ")
				fmt.Fprintf(writer, "%s\n", encoded)
			} else if reply.Verdict.Allow {
				fmt.Fprintf(writer, "allow, %d uses left\n", reply.Verdict.Remaining)
			} else {
				fmt.Fprintf(writer, "deny: %s\n", reply.Verdict.Reason)
			}
			if !reply.Verdict.Allow {
				return fmt.Errorf("denied: %s", reply.Verdict.Reason)
			}
			return nil
		},
	}
}
