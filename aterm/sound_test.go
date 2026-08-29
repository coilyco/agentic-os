package main

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"

	"forgejo.coilysiren.me/coilyco-flight-deck/agentic-os/aterm/internal/soundspec"
)

// Every sample the binary carries has to be a real file, or a launch plays
// nothing and says nothing about why.
func TestEmbeddedSamplesAreUsableWAVs(t *testing.T) {
	entries, err := soundSamples.ReadDir("sounds")
	if err != nil {
		t.Fatalf("read the embedded samples: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no samples are embedded")
	}
	for _, entry := range entries {
		body, err := soundSamples.ReadFile("sounds/" + entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if len(body) < 44 || string(body[0:4]) != "RIFF" {
			t.Fatalf("%s is not a WAV", entry.Name())
		}
		if binary.LittleEndian.Uint32(body[40:44]) == 0 {
			t.Fatalf("%s carries no audio", entry.Name())
		}
	}
}

// The fixtures are real overlay captures, so a timbre they ship with no sample
// means `just aterm-sounds` was not run after the vocabulary moved.
func TestTheFixtureTimbresAllHaveASample(t *testing.T) {
	for _, name := range []string{"platform-claude-overlay.json", "director-codex-overlay.json"} {
		role, seat := "platform", "claude"
		if strings.HasPrefix(name, "director") {
			role, seat = "director", "codex"
		}
		document, err := parseOverlay(fixture(t, name), role, seat, "acting")
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, personality := range document.Personalities {
			file := soundspec.FileName(personality.SoundMark.Timbre)
			if file == "" {
				t.Fatalf("%s ships an unusable timbre %q", personality.Name, personality.SoundMark.Timbre)
			}
			if _, err := soundSamples.ReadFile("sounds/" + file); err != nil {
				t.Fatalf("%s ships timbre %q with no sample: run `just aterm-sounds`",
					personality.Name, personality.SoundMark.Timbre)
			}
		}
	}
}

func TestTheCardCarriesTheSoundMark(t *testing.T) {
	document := platformOverlay(t)
	card := buildSessionCard(document, launchPlan{})
	if len(card.Figures) != len(document.Personalities) {
		t.Fatalf("figures = %d", len(card.Figures))
	}
	for index, figure := range card.Figures {
		want := document.Personalities[index].SoundMark
		if figure.Timbre != want.Timbre || figure.Contour != want.Contour || figure.Pulse != want.Pulse {
			t.Fatalf("figure %d = %+v, want %+v", index, figure, want)
		}
	}
}

// Silence is the default, so the interesting cases are the two ways in and the
// one refusal that outranks both.
func TestASessionIsSilentUnlessAsked(t *testing.T) {
	if soundAsked(false) {
		t.Fatal("a launch nobody asked to hear should stay silent")
	}
	if !soundAsked(true) {
		t.Fatal("--sound should ask for the mark")
	}
	t.Setenv(soundEnv, "1")
	if !soundAsked(false) {
		t.Fatalf("%s should ask for the mark", soundEnv)
	}
	// A recording or a CI runner still gets nothing, opted in or not.
	if soundWanted(nil, true) {
		t.Fatal("a nil stdout is not a terminal")
	}
}

// Silence must cost nothing: no player resolved, no sample written.
func TestSilencePlaysAndCachesNothing(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	playSoundMark(buildSessionCard(platformOverlay(t), launchPlan{}), false)
	entries, err := os.ReadDir(t.TempDir())
	if err == nil && len(entries) != 0 {
		t.Fatalf("a silenced launch wrote %d file(s)", len(entries))
	}
}

func TestSessionStageDefaultsToSilent(t *testing.T) {
	quiet, err := parseSessionArgs([]string{"--no-motion", "--", "true"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if quiet.Audible || quiet.Motion {
		t.Fatalf("options = %+v, want both channels off", quiet)
	}
	loud, err := parseSessionArgs([]string{"--sound", "--", "true"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !loud.Audible {
		t.Fatalf("options = %+v, want the mark on", loud)
	}
}
