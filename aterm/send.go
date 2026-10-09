package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

const agentsFormat = "aterm.agents.v1"

func newDaemonCommand() *cli.Command {
	return &cli.Command{
		Name:  "daemon",
		Usage: "run the host daemon that routes every session, in the foreground",
		Description: "A session starts the daemon on its own, so this verb is for a service\n" +
			"manager or for watching one. It exits after five idle minutes, or never with --idle 0.",
		Commands: []*cli.Command{newDaemonLaunchdCommand()},
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "socket", Value: daemonSocket(), Usage: "unix socket to serve"},
			&cli.StringFlag{
				Name:    "websocket",
				Value:   defaultDaemonWS,
				Usage:   "loopback address for browser clients, empty for none",
				Sources: cli.EnvVars(daemonWSEnv),
			},
			&cli.StringFlag{
				Name:    "tailnet-port",
				Value:   defaultTailnetPort,
				Usage:   "HTTPS port on this node's tailnet address, empty for none",
				Sources: cli.EnvVars(daemonTailnetPortEnv),
			},
			&cli.StringSliceFlag{
				Name:    "peer-port",
				Usage:   "name=port of a peer whose daemon is not on --tailnet-port, so host discovery dials it there",
				Sources: cli.EnvVars(daemonPeerPortsEnv),
			},
			&cli.StringSliceFlag{
				Name:    "allow-tags",
				Value:   defaultAllowTags,
				Usage:   "tailnet tags whose devices may attach, besides this node owner's own",
				Sources: cli.EnvVars(daemonAllowTagsEnv),
			},
			&cli.StringSliceFlag{
				Name:    "allow-origins",
				Value:   defaultAllowOrigins,
				Usage:   "hosted client pages that may open a session socket on the tailnet",
				Sources: cli.EnvVars(daemonAllowOrigins),
			},
			&cli.StringFlag{Name: "client-dir", Value: defaultClientDir(), Usage: "built aterm client served at /"},
			&cli.DurationFlag{Name: "idle", Value: daemonIdle, Usage: "exit after this long with no session and no client, 0 for never"},
			&cli.BoolFlag{Name: "end-sessions", Usage: "end every session when the daemon stops, instead of leaving them for the next daemon"},
			&cli.StringFlag{
				Name:    "sentry-dsn",
				Usage:   "Sentry DSN for the cron check-in each interval and for panic capture, empty for none",
				Sources: cli.EnvVars(sentryDSNEnv),
			},
			&cli.StringFlag{
				Name:    "agent-proxy",
				Usage:   "Agent Proxy base URL, read for the context of seats that run through it, empty for none",
				Sources: cli.EnvVars(agentProxyEnv),
			},
			&cli.StringFlag{
				Name:    "vapid-key",
				Usage:   "VAPID private key that turns on Web Push to a closed browser, empty for none",
				Sources: cli.EnvVars(vapidKeyEnv),
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			peerPorts, err := parsePeerPorts(cmd.StringSlice("peer-port"))
			if err != nil {
				return withExit(exitUsage, err)
			}
			return runDaemon(daemonOptions{
				PeerPorts:    peerPorts,
				Socket:       cmd.String("socket"),
				Websocket:    cmd.String("websocket"),
				TailnetPort:  cmd.String("tailnet-port"),
				AllowTags:    cmd.StringSlice("allow-tags"),
				AllowOrigins: cmd.StringSlice("allow-origins"),
				ClientDir:    cmd.String("client-dir"),
				Idle:         cmd.Duration("idle"),
				EndSessions:  cmd.Bool("end-sessions"),
				AgentProxy:   cmd.String("agent-proxy"),
				SentryDSN:    cmd.String("sentry-dsn"),
				VAPIDKey:     cmd.String("vapid-key"),
			}, cmd.Root().ErrWriter)
		},
	}
}

func newSendCommand() *cli.Command {
	return &cli.Command{
		Name:          "send",
		ShellComplete: completeSessionName,
		Usage:         "type a message into another live session, stamped with this session's seat",
		ArgsUsage:     "<role|seat|session> <message...>",
		Description: "The daemon stamps `[from <role> <identity>]` from this session's token,\n" +
			"so the sender cannot choose that line. A message of `-` reads stdin.\n" +
			"A message over 256 KiB is refused, so send a file's path instead.\n" +
			"It waits while Kai is typing in the target and lands after her draft.",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "launch", Usage: "when no session answers to a role, open one and deliver into it"},
			&cli.BoolFlag{Name: "new", Usage: "open a new instance of the role even when one is live, and deliver into it"},
			&cli.BoolFlag{Name: "notify-idle", Usage: "type one line into this session when the target next goes idle after the message lands"},
			&cli.DurationFlag{Name: "wait", Usage: "wait up to this long for delivered or failed, 3s when absent, and exit 1 on failed"},
			&cli.BoolFlag{Name: "json", Usage: "print the message state as JSON"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			args := cmd.Args().Slice()
			if len(args) < 2 {
				return withExit(exitUsage, fmt.Errorf("aterm send needs a target and a message. `aterm agents` lists targets"))
			}
			body := strings.Join(args[1:], " ")
			if body == "-" {
				// One byte past the limit is enough to know it is over.
				raw, err := io.ReadAll(io.LimitReader(os.Stdin, maxSendBody+1))
				if err != nil {
					return err
				}
				body = string(raw)
			}
			wait := cmd.Duration("wait")
			state, err := sendMessage(args[0], body, sendOptions{Launch: cmd.Bool("launch"), Fresh: cmd.Bool("new"),
				NotifyIdle: cmd.Bool("notify-idle"), Wait: wait})
			if err != nil {
				return err
			}
			if cmd.Bool("json") {
				encoded, _ := json.MarshalIndent(state, "", "  ")
				_, err = fmt.Fprintf(cmd.Root().Writer, "%s\n", encoded)
			} else {
				_, err = fmt.Fprintln(cmd.Root().Writer, describeMessage(state))
			}
			if err == nil && wait > 0 && state.State == "failed" {
				err = fmt.Errorf("the message failed: %s", state.Reason)
			}
			return err
		},
	}
}

// sendOptions are the choices a send takes beyond its target and body.
type sendOptions struct {
	Launch, Fresh, NotifyIdle bool
	Wait                      time.Duration
}

// sendMessage is the one path both front doors take, the CLI and the MCP
// tool, so the two cannot drift.
func sendMessage(target, body string, opts sendOptions) (peerMessage, error) {
	if len(body) > maxSendBody {
		return peerMessage{}, errSendTooLong()
	}
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
	if opts.Fresh && !slices.Contains(c.features, sendNewFeature) {
		return peerMessage{}, fmt.Errorf("the running aterm daemon predates new-instance sends and would deliver " +
			"to the live session. It restarts on the upgraded binary after five idle minutes")
	}
	if opts.Wait > 0 && !slices.Contains(c.features, sendWaitFeature) {
		return peerMessage{}, fmt.Errorf("the running aterm daemon predates send --wait and would answer after 3 seconds. " +
			"It restarts on the upgraded binary after five idle minutes")
	}
	if opts.NotifyIdle && !slices.Contains(c.features, sendIdleFeature) {
		return peerMessage{}, fmt.Errorf("the running aterm daemon predates send --notify-idle and would send no notice. " +
			"It restarts on the upgraded binary after five idle minutes")
	}
	reply, err := c.request(frame{Type: "send", Token: token, Target: target, Body: body, Launch: opts.Launch, New: opts.Fresh,
		NotifyIdle: opts.NotifyIdle, Wait: int(math.Ceil(opts.Wait.Seconds()))})
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
	if state.State != "launching" {
		return state, nil
	}
	// Subscribe before launching, and read the watch during it: an unread watch
	// fills with sessions frames and the daemon drops it (COI-2300).
	watch, err := dialDaemon(false)
	if err == nil {
		defer watch.Close()
		err = watch.write(frame{Type: "subscribe", ID: randomID(6), Channel: "sessions"})
	}
	var adopted chan peerMessage
	if err == nil {
		adopted = make(chan peerMessage, 1)
		go func() { adopted <- awaitSession(watch, state, launchWait) }()
	}
	if launchErr := openRole(target, ""); launchErr != nil {
		return state, fmt.Errorf("the message waits for %s, but opening it failed: %w", target, launchErr)
	}
	if adopted == nil {
		return state, nil
	}
	select {
	case state = <-adopted:
	case <-time.After(adoptWait):
	}
	return state, nil
}

// openRole is launchRole, replaceable so a test can open the role itself.
var openRole = launchRole

// adoptWait bounds how long a launching send waits to learn which session
// took it. The daemon keeps holding the message for launchWait after this.
const adoptWait = 45 * time.Second

// awaitSession reads the sessions channel until the daemon hands the message
// to a session, so the caller learns the new instance's name.
func awaitSession(watch *conn, state peerMessage, limit time.Duration) peerMessage {
	_ = watch.raw.SetReadDeadline(time.Now().Add(limit))
	for {
		event, err := watch.read()
		if err != nil {
			return state
		}
		if event.Type != "message" || event.Message == nil || event.Message.ID != state.ID {
			continue
		}
		state = *event.Message
		if state.Session != "" || state.State == "failed" {
			return state
		}
	}
}

// launchRole opens the role through this binary, so the roster check is the
// ordinary one. Headless, since nobody is at a launch the daemon starts.
func launchRole(role, seat string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	output, err := exec.Command(self, launchRoleArgs(role, seat)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func launchRoleArgs(role, seat string) []string {
	args := []string{"--headless", role}
	if seat != "" {
		args = append(args, seat)
	}
	return args
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
	if view.State != "" {
		parts = append(parts, agentStateWord(view))
	} else if !view.Ready {
		parts = append(parts, "starting")
	}
	if len(view.Degraded) > 0 {
		parts = append(parts, "degraded: "+strings.Join(view.Degraded, " "))
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

// agentStateWord is the screen state for a line, with how long a quiet session
// has been quiet, since "idle for a minute" and "idle for an hour" differ.
func agentStateWord(view sessionView) string {
	switch view.State {
	case statePrompt:
		return "waiting on a prompt"
	case stateIdle:
		return fmt.Sprintf("idle %s", (time.Duration(view.QuietSeconds) * time.Second).String())
	}
	return view.State
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

// askChoice puts a question to whoever is at a client and waits for the pick.
// Both front doors take it, the CLI and the MCP tool.
func askChoice(ask choiceAsk) (choiceAnswer, error) {
	token := strings.TrimSpace(os.Getenv(sessionTokenEnv))
	if token == "" {
		return choiceAnswer{}, withExit(exitUsage, fmt.Errorf(
			"%s is unset: aterm ask speaks for a session aterm launched, so run it from inside one", sessionTokenEnv))
	}
	c, err := dialDaemon(false)
	if err != nil {
		return choiceAnswer{}, withExit(exitMissing, err)
	}
	defer c.Close()
	reply, err := c.request(frame{Type: "ask", Token: token, Ask: &ask})
	if err != nil {
		if reply.Code != 0 {
			return choiceAnswer{}, withExit(reply.Code, err)
		}
		return choiceAnswer{}, err
	}
	if reply.Answer == nil {
		return choiceAnswer{}, fmt.Errorf("the daemon settled the ask without an answer")
	}
	return *reply.Answer, nil
}

func newAskCommand() *cli.Command {
	return &cli.Command{
		Name:      "ask",
		Usage:     "put a multiple-choice question to Kai's client and print the pick",
		ArgsUsage: "<question> <option[::description]>...",
		Description: "The daemon stamps the asking seat from this session's token and shows the\n" +
			"question on every attached client. It exits 1 when the ask is cancelled or times out.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "header", Usage: "a short label above the question"},
			&cli.BoolFlag{Name: "other", Usage: "allow a free-text answer"},
			&cli.BoolFlag{Name: "multi", Usage: "allow more than one pick"},
			&cli.BoolFlag{Name: "json", Usage: "print the answer as JSON"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			args := cmd.Args().Slice()
			if len(args) < 1 {
				return withExit(exitUsage, fmt.Errorf("aterm ask needs a question"))
			}
			ask := choiceAsk{Question: args[0], Header: cmd.String("header"), AllowOther: cmd.Bool("other"), Multi: cmd.Bool("multi")}
			for _, raw := range args[1:] {
				label, description, _ := strings.Cut(raw, "::")
				ask.Options = append(ask.Options, choiceOption{Label: label, Description: description})
			}
			answer, err := askChoice(ask)
			if err != nil {
				return err
			}
			writer := cmd.Root().Writer
			if cmd.Bool("json") {
				encoded, _ := json.MarshalIndent(answer, "", "  ")
				fmt.Fprintf(writer, "%s\n", encoded)
			} else {
				for _, label := range answer.Labels {
					fmt.Fprintln(writer, label)
				}
				if answer.Text != "" {
					fmt.Fprintln(writer, answer.Text)
				}
			}
			if answer.State != "answered" {
				return fmt.Errorf("the ask was %s: %s", strings.ReplaceAll(answer.State, "_", " "), answer.Reason)
			}
			return nil
		},
	}
}
