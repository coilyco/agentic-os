package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"forgejo.coilysiren.me/coilyco-flight-deck/agentic-os/aterm/internal/soundspec"
)

// Rendered by `just aterm-sounds` and committed, so a launch needs no synth.

//go:embed sounds/*.wav
var soundSamples embed.FS

const (
	soundEnv        = "ATERM_SOUND"
	soundPlayBudget = 4 * time.Second
)

// soundPlayers is tried in order. The first one on PATH wins, and none of them
// being present is silence rather than a diagnostic.
var soundPlayers = map[string][][]string{
	"darwin": {{"afplay"}},
	"linux":  {{"paplay"}, {"aplay", "-q"}, {"play", "-q"}},
}

// playSoundMark plays the role's pair in the background. Sound is the only
// identity channel that reaches an operator looking elsewhere.
func playSoundMark(card sessionCard, audible bool) {
	if !audible {
		return
	}
	player := resolveSoundPlayer()
	if len(player) == 0 {
		return
	}
	paths := make([]string, 0, len(card.Figures))
	for _, figure := range card.Figures {
		path, err := cachedSample(figure.Timbre)
		if err != nil || path == "" {
			continue
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), soundPlayBudget)
		defer cancel()
		for _, path := range paths {
			// A failure here is never worth a line on top of the identity card.
			_ = exec.CommandContext(ctx, player[0], append(player[1:], path)...).Run()
		}
	}()
}

// hasSample answers whether the binary carries audio for a timbre, which is
// what a roster that grew a personality breaks.
func hasSample(timbre string) bool {
	name := soundspec.FileName(timbre)
	if name == "" {
		return false
	}
	_, err := soundSamples.ReadFile("sounds/" + name)
	return err == nil
}

func resolveSoundPlayer() []string {
	for _, candidate := range soundPlayers[runtime.GOOS] {
		if resolved, err := exec.LookPath(candidate[0]); err == nil {
			return append([]string{resolved}, candidate[1:]...)
		}
	}
	return nil
}

// cachedSample materializes the embedded sample once. A player needs a path,
// and rewriting it on every launch would be a write per window.
func cachedSample(timbre string) (string, error) {
	name := soundspec.FileName(timbre)
	if name == "" {
		return "", nil
	}
	body, err := soundSamples.ReadFile("sounds/" + name)
	if err != nil {
		// A timbre the shipped set does not carry is silent, which is the
		// honest answer until `just aterm-sounds` runs again.
		return "", nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	directory := filepath.Join(cache, "aterm", "sound")
	path := filepath.Join(directory, hex.EncodeToString(digest[:8])+"-"+name)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// soundAsked is the opt-in on its own, separable so it is testable off a
// terminal, which is where the suite runs.
func soundAsked(enabled bool) bool {
	return enabled || strings.TrimSpace(os.Getenv(soundEnv)) != ""
}

// soundWanted is off unless asked for, and still refused wherever it would play
// into a log, a recording, or CI. See docs/aterm.md.
func soundWanted(stdout *os.File, enabled bool) bool {
	return soundAsked(enabled) && characterDevices(stdout)
}
