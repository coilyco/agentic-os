package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// stageFeature is how a client knows the daemon answers a stage frame. See
// docs/aterm-pane.md.
const stageFeature = "stage"

const (
	maxStageCommand = 4096
	maxStageNote    = 256
	// stageShellWait bounds how long a freshly opened shell gets to reach its
	// prompt, since text typed ahead of an rc file can be read by the rc file.
	stageShellWait  = 5 * time.Second
	stageShellQuiet = 250 * time.Millisecond
)

// The three standing answers a stage gives besides an error.
const (
	stageStaged   = "staged"
	stageRefused  = "refused"
	stageNoClient = "no_client"
)

// checkStagedText is the one gate between a seat and Kai's shell: it refuses control,
// format and line-separator runes and invalid UTF-8, and names the rune.
func checkStagedText(what, text string, limit int) error {
	if len(text) > limit {
		return fmt.Errorf("%s is at most %d bytes, and this one is %d", what, limit, len(text))
	}
	if !utf8.ValidString(text) {
		return fmt.Errorf("%s is UTF-8 text", what)
	}
	for _, r := range text {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return fmt.Errorf("%s may not carry %U: a staged line holds no control, format or line-separator character, "+
				"so it can never run itself", what, r)
		}
	}
	return nil
}

func checkStagedCommand(command string) error {
	if strings.TrimSpace(command) == "" {
		return errors.New("the command is empty")
	}
	return checkStagedText("the command", command, maxStageCommand)
}

func checkStageNote(note string) error {
	return checkStagedText("the note", note, maxStageNote)
}

// pairedTerminal is the live shell labelled with the seat's session name, which is the
// client's pairing. The oldest wins.
func (d *daemon) pairedTerminal(seat string) *ptySession {
	d.mu.Lock()
	defer d.mu.Unlock()
	var paired []*ptySession
	for _, s := range d.terminals {
		if s.label == seat {
			paired = append(paired, s)
		}
	}
	sort.Slice(paired, func(i, j int) bool {
		if !paired[i].started.Equal(paired[j].started) {
			return paired[i].started.Before(paired[j].started)
		}
		return paired[i].name < paired[j].name
	})
	if len(paired) == 0 {
		return nil
	}
	return paired[0]
}

func (d *daemon) hasSubscribers() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.subscribers) > 0
}

// stageRefusal answers a stage the daemon will not carry out.
func stageRefusal(message frame, reason string) frame {
	return frame{Type: "stage_result", ID: message.ID, State: stageRefused, Text: reason}
}

// stage prefills a command into the terminal beside the calling seat, found from its
// token alone, and never submits it. See docs/aterm-pane.md.
func (d *daemon) stage(cl *client, message frame) error {
	sender := d.byToken(message.Token)
	if sender == nil {
		return withExit(exitUsage, errors.New("aterm stage speaks for a session aterm launched, and this token names no live one"))
	}
	if message.Target != "" || message.Session != "" || message.Role != "" || message.Identity != "" ||
		message.Seat != "" || message.Label != "" || len(message.Argv) > 0 || len(message.Env) > 0 {
		return cl.c.write(stageRefusal(message, "stage takes a command and a note and never names a terminal or another seat: "+
			"it stages beside the session its token belongs to"))
	}
	if err := checkStagedCommand(message.Body); err != nil {
		return cl.c.write(stageRefusal(message, err.Error()))
	}
	if err := checkStageNote(message.Text); err != nil {
		return cl.c.write(stageRefusal(message, err.Error()))
	}

	// One stage at a time, so two calls from a seat open one shell between them.
	d.stageMu.Lock()
	defer d.stageMu.Unlock()
	s, opened := d.pairedTerminal(sender.name), false
	if s == nil {
		if !d.hasSubscribers() {
			return cl.c.write(frame{Type: "stage_result", ID: message.ID, State: stageNoClient,
				Text: "no aterm client is attached to this daemon, so nothing would show the staged line. " +
					"Nothing was opened or typed"})
		}
		var err error
		if s, err = d.spawnTerminal(frame{Kind: kindTerminal, Label: sender.name}); err != nil {
			return cl.c.write(stageRefusal(message, "the terminal beside "+sender.name+" did not open: "+err.Error()))
		}
		opened = true
		d.pushSessions()
		if !s.awaitPrompt(stageShellWait, stageShellQuiet) {
			return cl.c.write(stageRefusal(message, s.name+" opened but its shell had not settled at a prompt after "+
				stageShellWait.String()+", so nothing was typed. It stays open, and a second stage will use it"))
		}
	}

	viewers, err := s.stageInput(message.Body)
	if err != nil {
		return cl.c.write(stageRefusal(message, err.Error()))
	}
	d.logf("staged %d bytes in %s beside %s", len(message.Body), s.name, sender.name)
	d.broadcast(frame{Type: "staged", Session: s.name, Label: sender.name, Body: message.Body, Text: message.Text})
	reply := frame{Type: "stage_result", ID: message.ID, State: stageStaged, Session: s.name, Opened: opened, Viewers: viewers}
	if viewers == 0 {
		reply.Text = "no client is viewing " + s.name + " yet, so the line waits there for Enter"
	}
	return cl.c.write(reply)
}

// stageInput types the command with no Enter and counts it as an unsent line, so a
// second stage or Kai's own draft refuses. It returns the viewer count.
func (s *ptySession) stageInput(command string) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.Lock()
	held := s.draft > 0
	viewers := len(s.clients)
	s.mu.Unlock()
	if held {
		return viewers, fmt.Errorf("%s already holds an unsent line, so a staged command would run together with it. "+
			"Enter, Ctrl-C or Ctrl-U there clears it", s.name)
	}
	// Defence in depth: the daemon's own gate ran in stage, and this is the last
	// place the bytes are still a string.
	if err := checkStagedCommand(command); err != nil {
		return viewers, err
	}
	if err := s.writePTY([]byte(command)); err != nil {
		return viewers, err
	}
	s.mu.Lock()
	s.trackDraft([]byte(command))
	s.mu.Unlock()
	return viewers, nil
}

// awaitPrompt waits for a shell to print something and then go quiet.
func (s *ptySession) awaitPrompt(limit, quiet time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		spoke := !s.lastOutput.IsZero()
		settled := spoke && time.Since(s.lastOutput) >= quiet
		s.mu.Unlock()
		if settled {
			return true
		}
		select {
		case <-s.done:
			return false
		case <-time.After(25 * time.Millisecond):
		}
	}
	return false
}
