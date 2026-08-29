package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"
)

// leafValues flattens JSON to path/value pairs, so one comparison catches both
// a field the struct dropped and a value it mangled.
func leafValues(value any, prefix string, into map[string]any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			leafValues(nested, path, into)
		}
	case []any:
		for index, nested := range typed {
			leafValues(nested, fmt.Sprintf("%s[%d]", prefix, index), into)
		}
	default:
		into[prefix] = typed
	}
}

func leaves(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	flat := map[string]any{}
	leafValues(document, "", flat)
	return flat
}

// Decoding into a narrower struct is silent, so every shipped leaf has to
// survive the round trip with the value it went in with.
func TestOverlayDecodesEveryShippedField(t *testing.T) {
	cases := map[string][2]string{
		"platform-claude-overlay.json": {"platform", "claude"},
		"platform-codex-overlay.json":  {"platform", "codex"},
		"director-codex-overlay.json":  {"director", "codex"},
		// frontend is the only seat on the scattered arrangement.
		"frontend-claude-overlay.json": {"frontend", "claude"},
	}
	for name, selection := range cases {
		t.Run(name, func(t *testing.T) {
			raw := fixture(t, name)
			document, err := parseOverlay(raw, selection[0], selection[1], "acting")
			if err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			encoded, err := json.Marshal(document)
			if err != nil {
				t.Fatalf("re-encode %s: %v", name, err)
			}
			shipped := leaves(t, raw)
			decoded := leaves(t, encoded)
			discarded := []string{}
			for path, want := range shipped {
				got, ok := decoded[path]
				if !ok {
					discarded = append(discarded, path+" (dropped)")
					continue
				}
				if got != want {
					discarded = append(discarded, fmt.Sprintf("%s (%v, want %v)", path, got, want))
				}
			}
			sort.Strings(discarded)
			for _, entry := range discarded {
				t.Errorf("overlay field %s", entry)
			}
		})
	}
}

// The identity card and the sound mark are built from this vocabulary, so an
// empty leaf is a contract break rather than a blank field.
func TestOverlayPersonalitiesCarryTheFullSensoryVocabulary(t *testing.T) {
	document := platformOverlay(t)
	if len(document.Personalities) == 0 {
		t.Fatal("the platform overlay ships no personalities")
	}
	for _, personality := range document.Personalities {
		fields := map[string]string{
			"name":               personality.Name,
			"color":              personality.Color,
			"motif":              personality.Motif,
			"emblem.emoji":       personality.Emblem.Emoji,
			"geometry":           personality.Geometry,
			"body.archetype":     personality.Body.Archetype,
			"body.attachment":    personality.Body.Attachment,
			"sound_mark.timbre":  personality.SoundMark.Timbre,
			"sound_mark.contour": personality.SoundMark.Contour,
			"sound_mark.pulse":   personality.SoundMark.Pulse,
		}
		for field, value := range fields {
			if value == "" {
				t.Errorf("personality %s has an empty %s", personality.Name, field)
			}
		}
		if len(personality.Emblem.Names) == 0 {
			t.Errorf("personality %s ships no emblem names", personality.Name)
		}
		if !hexColorPattern.MatchString(personality.Color) {
			t.Errorf("personality %s color %q is not #RRGGBB", personality.Name, personality.Color)
		}
	}
	if document.Seat.Key == "" || document.Seat.Tier == "" {
		t.Errorf("seat key %q tier %q, want both", document.Seat.Key, document.Seat.Tier)
	}
	if document.Purpose == "" {
		t.Error("the overlay purpose is empty")
	}
}
