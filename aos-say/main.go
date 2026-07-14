// Command aos-say speaks short status messages locally or through a relay.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

const (
	sayBinary = "/usr/bin/say"
	relayEnv  = "AOS_SAY_RELAY"
)

var runCommand = func(command []string) error {
	cmd := exec.Command(command[0], command[1:]...)
	return cmd.Run()
}

type request struct {
	Text         string `json:"text"`
	Voice        string `json:"voice,omitempty"`
	Rate         int    `json:"rate,omitempty"`
	DryRun       bool   `json:"dry_run,omitempty"`
	Notification bool   `json:"notification,omitempty"`
}

func main() {
	os.Exit(run(os.Args[1:], runtime.GOOS, os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, goos string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "relay" {
		fs := flag.NewFlagSet("aos-say relay", flag.ContinueOnError)
		fs.SetOutput(stderr)
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if err := runRelay(stdin, stdout); err != nil {
			fmt.Fprintln(stderr, "aos-say relay:", err)
			return 1
		}
		return 0
	}

	fs := flag.NewFlagSet("aos-say", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var voice string
	var rate int
	var dryRun bool
	var notification bool
	fs.StringVar(&voice, "voice", "", "voice name passed to /usr/bin/say")
	fs.IntVar(&rate, "rate", 0, "speech rate passed to /usr/bin/say")
	fs.BoolVar(&dryRun, "dry-run", false, "print the command or relay request without speaking")
	fs.BoolVar(&notification, "notification", false, "show a desktop notification after speaking")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	text := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(text) == "" {
		fmt.Fprintln(stderr, "aos-say: missing text")
		return 2
	}

	req := request{
		Text:         text,
		Voice:        voice,
		Rate:         rate,
		DryRun:       dryRun,
		Notification: notification,
	}

	if goos == "darwin" {
		if err := runLocal(req, stdout); err != nil {
			fmt.Fprintln(stderr, "aos-say:", err)
			return 1
		}
		return 0
	}

	addr := os.Getenv(relayEnv)
	if addr == "" {
		fmt.Fprintf(stderr, "aos-say: %s is not set\n", relayEnv)
		return 1
	}
	if err := runRemote(req, addr, stdout); err != nil {
		fmt.Fprintln(stderr, "aos-say:", err)
		return 1
	}
	return 0
}

func runLocal(req request, stdout io.Writer) error {
	commands := buildCommands(req)
	if req.DryRun {
		printCommands(stdout, commands)
		return nil
	}
	return executeCommands(commands)
}

func runRemote(req request, addr string, stdout io.Writer) error {
	if req.DryRun {
		printCommands(stdout, buildCommands(req))
		return nil
	}
	conn, err := dialRelay(addr)
	if err != nil {
		return fmt.Errorf("dialing relay %q: %w", addr, err)
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return fmt.Errorf("sending request to relay: %w", err)
	}
	return nil
}

func runRelay(stdin io.Reader, stdout io.Writer) error {
	var req request
	if err := json.NewDecoder(stdin).Decode(&req); err != nil {
		return fmt.Errorf("reading request: %w", err)
	}
	commands := buildCommands(req)
	if req.DryRun {
		printCommands(stdout, commands)
		return nil
	}
	return executeCommands(commands)
}

func buildCommands(req request) [][]string {
	commands := [][]string{buildSayCommand(req)}
	if req.Notification {
		commands = append(commands, buildNotificationCommand(req.Text))
	}
	return commands
}

func buildSayCommand(req request) []string {
	args := []string{sayBinary}
	if req.Voice != "" {
		args = append(args, "-v", req.Voice)
	}
	if req.Rate > 0 {
		args = append(args, "-r", strconv.Itoa(req.Rate))
	}
	args = append(args, req.Text)
	return args
}

func buildNotificationCommand(text string) []string {
	return []string{
		"/usr/bin/osascript",
		"-e",
		"display notification \"" + escapeAppleScriptString(text) + "\" with title \"aos-say\"",
	}
}

func printCommands(dst io.Writer, commands [][]string) {
	for _, command := range commands {
		fmt.Fprintln(dst, strings.Join(command, " "))
	}
}

func executeCommands(commands [][]string) error {
	for _, command := range commands {
		if len(command) == 0 {
			continue
		}
		if err := runCommand(command); err != nil {
			return fmt.Errorf("running %s: %w", command[0], err)
		}
	}
	return nil
}

func escapeAppleScriptString(text string) string {
	text = strings.ReplaceAll(text, `\`, `\\`)
	text = strings.ReplaceAll(text, `"`, `\"`)
	return text
}

func dialRelay(addr string) (net.Conn, error) {
	network, target := relayTarget(addr)
	return net.Dial(network, target)
}

func relayTarget(addr string) (string, string) {
	switch {
	case strings.HasPrefix(addr, "unix://"):
		return "unix", strings.TrimPrefix(addr, "unix://")
	case strings.HasPrefix(addr, "unix:"):
		return "unix", strings.TrimPrefix(addr, "unix:")
	case strings.HasPrefix(addr, "tcp://"):
		return "tcp", strings.TrimPrefix(addr, "tcp://")
	case strings.HasPrefix(addr, "/"):
		return "unix", addr
	default:
		return "tcp", addr
	}
}
