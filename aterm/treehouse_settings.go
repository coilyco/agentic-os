package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"
)

// A reader of the snapshot agent-compose publishes, not an import of its private writer.
// A golden file the real writer made holds this reader to the writer's format and hash.

const (
	configSnapshotFormat = "treehouse.config.snapshot.v1"
	configSnapshotFile   = "snapshot.json"
	configLastGoodFile   = "snapshot.last-good.json"
	configDirEnv         = "AGENT_COMPOSE_CONFIG_DIR"
	// settingAllUsers is the settings user whose values apply to everyone.
	settingAllUsers = "*"
	// settingPushQuestionText puts the first questionChars of an ask in the broker event.
	settingPushQuestionText = "push.question_text"
	questionChars           = 140
)

// settingsReader gives a per-user setting's value, statting the snapshot on each read so
// an edit applies at the next one. A bad snapshot never replaces what is held.
type settingsReader struct {
	dir     string
	mu      sync.Mutex
	info    os.FileInfo
	values  map[string]map[string]bool
	loaded  bool
	lastErr error
}

func newSettingsReader(dir string) *settingsReader { return &settingsReader{dir: dir} }

// configDir is the snapshot directory: the flag, else AGENT_COMPOSE_CONFIG_DIR, else
// ~/.agent-compose/config. The word off gives none.
func configDir(flag string) string {
	value := strings.TrimSpace(flag)
	if value == "" {
		value = strings.TrimSpace(os.Getenv(configDirEnv))
	}
	if value == "off" {
		return ""
	}
	if value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".agent-compose", "config")
}

// Bool is the user's value, else the everyone value, else def.
func (r *settingsReader) Bool(user, key string, def bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refresh()
	if value, set := r.values[user][key]; set {
		return value
	}
	if value, set := r.values[settingAllUsers][key]; set {
		return value
	}
	return def
}

// Err is why the newest snapshot was refused, nil when it loaded.
func (r *settingsReader) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastErr
}

func (r *settingsReader) refresh() {
	info, err := os.Stat(filepath.Join(r.dir, configSnapshotFile))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			r.lastErr = err
		}
		return
	}
	if r.info != nil && os.SameFile(r.info, info) && r.info.Size() == info.Size() && r.info.ModTime().Equal(info.ModTime()) {
		return
	}
	r.info = info
	values, err := readSnapshot(filepath.Join(r.dir, configSnapshotFile))
	if err == nil {
		r.values, r.loaded, r.lastErr = values, true, nil
		return
	}
	r.lastErr = err
	if r.loaded {
		return
	}
	if values, lastGoodErr := readSnapshot(filepath.Join(r.dir, configLastGoodFile)); lastGoodErr == nil {
		r.values, r.loaded = values, true
	}
}

// readSnapshot checks one snapshot's format and hash and returns its settings.
// The hash is sha256 of the config as compact JSON, which is how the writer computes it.
func readSnapshot(path string) (map[string]map[string]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file struct {
		Format string          `json:"format"`
		SHA256 string          `json:"sha256"`
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("decode the snapshot: %w", err)
	}
	if file.Format != configSnapshotFormat {
		return nil, fmt.Errorf("snapshot format %q is not %q", file.Format, configSnapshotFormat)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, file.Config); err != nil {
		return nil, fmt.Errorf("decode the snapshot config: %w", err)
	}
	sum := sha256.Sum256(compact.Bytes())
	if hex.EncodeToString(sum[:]) != file.SHA256 {
		return nil, errors.New("snapshot hash does not match its config, so the file is truncated or was edited by hand")
	}
	var config struct {
		Settings map[string]map[string]bool `json:"settings"`
	}
	if err := json.Unmarshal(file.Config, &config); err != nil {
		return nil, fmt.Errorf("decode the snapshot settings: %w", err)
	}
	return config.Settings, nil
}

// firstChars is the first n characters of s, whole characters and never a split rune.
func firstChars(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n])
}
