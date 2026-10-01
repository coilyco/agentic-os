package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/urfave/cli/v3"
)

// detachKey is Ctrl-], telnet's escape, and only `aterm attach` reads it. A
// session window closing is its detach.
const detachKey = 0x1d

// inputPump owns stdin for the life of the process, so the read still pending
// when a session ends is the one that answers the hold prompt.
type inputPump struct{ chunks chan []byte }

func startPump(reader io.Reader) *inputPump {
	pump := &inputPump{chunks: make(chan []byte, 64)}
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := reader.Read(buffer)
			if n > 0 {
				pump.chunks <- append([]byte(nil), buffer[:n]...)
			}
			if err != nil {
				close(pump.chunks)
				return
			}
		}
	}()
	return pump
}

// waitLine returns at Enter or the end of input.
func (pump *inputPump) waitLine() {
	for chunk := range pump.chunks {
		for _, b := range chunk {
			if b == '\n' || b == '\r' {
				return
			}
		}
	}
}

type attachResult struct {
	code     int
	lost     bool
	detached bool
}

// attachLoop is one terminal on one session, returning when it exits, the
// daemon goes away, or at Ctrl-] with detach set. Next is the offset drawn.
func attachLoop(c *conn, session string, pump *inputPump, stdout *os.File, detach bool, next *int64) attachResult {
	fd := stdout.Fd()
	if state, err := term.MakeRaw(os.Stdin.Fd()); err == nil {
		defer func() { _ = term.Restore(os.Stdin.Fd(), state) }()
	}
	resize := func() {
		if width, height, err := term.GetSize(fd); err == nil {
			_ = c.write(frame{Type: "resize", Session: session, Rows: height, Cols: width})
		}
	}
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	resize()
	exited := make(chan int, 1)
	lost := make(chan struct{})
	go func() {
		defer close(lost)
		for {
			message, err := c.read()
			if err != nil {
				return
			}
			switch message.Type {
			case "output":
				if message.Session == session {
					_, _ = stdout.Write(message.Data)
					*next = message.Offset + int64(len(message.Data))
				}
			case "exit":
				if message.Session == session {
					exited <- message.Code
					return
				}
			}
		}
	}()
	for {
		select {
		case code := <-exited:
			return attachResult{code: code}
		case <-lost:
			select {
			case code := <-exited:
				return attachResult{code: code}
			default:
			}
			return attachResult{code: exitFailure, lost: true}
		case <-winch:
			resize()
		case chunk, ok := <-pump.chunks:
			if !ok {
				return attachResult{detached: true}
			}
			if detach {
				if index := indexByte(chunk, detachKey); index >= 0 {
					if index > 0 {
						_ = c.write(frame{Type: "input", Session: session, Data: chunk[:index]})
					}
					_ = c.write(frame{Type: "detach", Session: session})
					return attachResult{detached: true}
				}
			}
			if err := c.write(frame{Type: "input", Session: session, Data: chunk}); err != nil {
				return attachResult{code: exitFailure, lost: true}
			}
		}
	}
}

// reconnectWait is how long a terminal redials a daemon that went away, past
// the launchd agent's 10 second throttle.
var reconnectWait = 60 * time.Second

// attachReconnecting is attachLoop that redials a lost daemon, starting one, and
// attaches again from the offset it drew. See docs/aterm-daemon.md.
func attachReconnecting(c *conn, session string, pump *inputPump, stdout *os.File, detach bool, stderr io.Writer) attachResult {
	var next int64
	for {
		result := attachLoop(c, session, pump, stdout, detach, &next)
		if !result.lost {
			return result
		}
		_ = c.Close()
		fmt.Fprintf(stderr, "\r\naterm: lost the daemon, reconnecting to %s\r\n", session)
		again, reply, err := redialSession(session, next)
		if err != nil {
			fmt.Fprintf(stderr, "\r\naterm: could not reconnect: %v\r\n", err)
			return result
		}
		if reply.Type == "exited" {
			return attachResult{code: reply.Code}
		}
		c = again
	}
}

// redialSession attaches on whatever daemon answers, starting one. A refusal
// ends it at once, and an unreachable daemon is retried until reconnectWait.
func redialSession(session string, next int64) (*conn, frame, error) {
	deadline := time.Now().Add(reconnectWait)
	var last error
	for {
		c, err := dialDaemon(true)
		if err == nil {
			width, height, _ := term.GetSize(os.Stdout.Fd())
			reply, requestErr := c.request(frame{Type: "attach", Session: session, Replay: true, Offset: next, Rows: height, Cols: width})
			if requestErr == nil {
				return c, reply, nil
			}
			_ = c.Close()
			if reply.Type == "error" {
				return nil, frame{}, requestErr
			}
			err = requestErr
		}
		last = err
		if time.Now().After(deadline) {
			return nil, frame{}, fmt.Errorf("the daemon did not come back within %s: %w", reconnectWait, last)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func indexByte(chunk []byte, want byte) int {
	for index, b := range chunk {
		if b == want {
			return index
		}
	}
	return -1
}

func newAttachCommand() *cli.Command {
	return &cli.Command{
		Name:      "attach",
		Usage:     "attach this terminal to a live session the daemon holds, Ctrl-] to detach",
		ArgsUsage: "<session>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			name := cmd.Args().First()
			if name == "" {
				return withExit(exitUsage, fmt.Errorf("name a session. `aterm agents` lists them"))
			}
			if !term.IsTerminal(os.Stdin.Fd()) {
				return withExit(exitUsage, fmt.Errorf("attach needs a terminal on stdin"))
			}
			c, err := dialDaemon(false)
			if err != nil {
				return withExit(exitMissing, err)
			}
			defer c.Close()
			width, height, _ := term.GetSize(os.Stdout.Fd())
			if _, err := c.request(frame{Type: "attach", Session: name, Replay: true, Rows: height, Cols: width}); err != nil {
				return withExit(exitOffRoster, err)
			}
			result := attachReconnecting(c, name, startPump(os.Stdin), os.Stdout, true, cmd.Root().ErrWriter)
			switch {
			case result.detached:
				fmt.Fprintf(cmd.Root().ErrWriter, "\r\naterm: detached from %s, which keeps running\r\n", name)
				return nil
			case result.lost:
				return fmt.Errorf("lost the aterm daemon")
			case result.code != 0:
				return withExit(result.code, fmt.Errorf("%s exited %d", name, result.code))
			}
			return nil
		},
	}
}
