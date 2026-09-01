package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const sessionCommand = "_session"

var (
	sessionNoticeStyle  = lipgloss.NewStyle().Faint(true)
	sessionFailureStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
)

// runSession is the inner half of the launch. Holding the window on a non-zero
// exit is why this exists rather than `-e`. See docs/aterm.md.
func runSession(options sessionOptions, stdin io.Reader, stdout, stderr io.Writer) int {
	argv := options.Argv
	if len(argv) == 0 {
		fmt.Fprintln(stderr, "aterm: "+sessionCommand+" needs a command after `--`")
		return 2
	}
	// The card is the only moment aterm owns the window by itself, so it is
	// drawn here rather than by the launcher the operator typed in.
	if options.Card.Format != "" {
		playSoundMark(options.Card, options.Audible)
		playCard(stdout, options.Card, options.Motion)
	}
	command := exec.Command(argv[0], argv[1:]...)
	// The card is already resolved here, so the session carries it rather than
	// re-resolving it later. `aterm card` re-renders from this.
	if options.CardPayload != "" {
		command.Env = append(os.Environ(), cardEnv+"="+options.CardPayload)
	}
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	err := command.Run()
	if err == nil {
		if options.Hold {
			holdWindow(stdin, stdout, sessionNoticeStyle.Render("Session ended. Press Enter to close."))
		}
		return 0
	}
	code := 1
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
		if code < 0 {
			code = 1
		}
	default:
		fmt.Fprintf(stderr, "\naterm: start %s: %v\n", argv[0], err)
	}
	holdWindow(stdin, stdout, sessionFailureStyle.Render(
		fmt.Sprintf("Session failed (exit %d). Press Enter to close.", code),
	))
	return code
}

// holdWindow keeps the pane readable after the child is gone. A non-interactive
// stdin will never answer, so it prints and returns instead.
func holdWindow(stdin io.Reader, stdout io.Writer, notice string) {
	fmt.Fprintf(stdout, "\n%s\n", notice)
	file, ok := stdin.(*os.File)
	if !ok {
		return
	}
	info, err := file.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return
	}
	reader := bufio.NewReader(file)
	_, _ = reader.ReadString('\n')
}

type sessionOptions struct {
	Hold    bool
	Motion  bool
	Audible bool
	Card    sessionCard
	// CardPayload is the encoded card exactly as it arrived, so the session can
	// pass it on without re-encoding what it decoded.
	CardPayload string
	Argv        []string
}

// parseSessionArgs hand-parses because everything after the first `--` belongs
// to the child verbatim, including the child's own `--`.
func parseSessionArgs(argv []string) (sessionOptions, error) {
	options := sessionOptions{Motion: true}
	for index := 0; index < len(argv); index++ {
		switch value := argv[index]; value {
		case "--hold":
			options.Hold = true
		case "--no-motion":
			options.Motion = false
		case "--sound":
			options.Audible = true
		case "--card":
			if index+1 >= len(argv) {
				return sessionOptions{}, fmt.Errorf("%s --card needs a value", sessionCommand)
			}
			index++
			card, err := decodeSessionCard(argv[index])
			if err != nil {
				return sessionOptions{}, err
			}
			options.Card = card
			options.CardPayload = strings.TrimSpace(argv[index])
		case "--":
			options.Argv = argv[index+1:]
			return options, nil
		default:
			return sessionOptions{}, fmt.Errorf(
				"%s has unsupported option %q",
				sessionCommand,
				strings.TrimSpace(value),
			)
		}
	}
	return sessionOptions{}, fmt.Errorf("%s needs a command after `--`", sessionCommand)
}
