package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const (
	// Bump when the plate recipe changes, so a cached plate from the old one
	// stops being reused rather than outliving it.
	creatureRecipe = "aterm.creature.v2"
	noCreatureEnv  = "ATERM_NO_CREATURE"
	// The creature's share of the window width, and how far its own box sits
	// from the top right it is anchored to. Both are fractions of the plate.
	creatureWidthShare = 0.60
	creatureInset      = 0.08
	// How much of the art survives the tint, chosen against the identity card
	// rather than in the abstract. See docs/aterm-creature.md.
	creaturePresence = 0.20
	// cscaled covers the window, so the plate carries the aspect a full-screen
	// window most often has and loses a band top and bottom on anything wider.
	creaturePlateAspect = 10.0 / 16.0
	// Where a fill starts losing alpha, and how much of it goes. The dark
	// linework stays whole at any depth. See docs/aterm-creature.md.
	creatureGlareKnee  = 0.62
	creatureGlareDepth = 0.30
	// A path kitty reads as a glob is a path it may not open. See docs/aterm-creature.md.
	globMetacharacters = "*?[]{}"
	// The creature has the whole window to itself until a pane takes part of
	// it. See docs/aterm-pane.md.
	creatureWholeWindow = 1.0
)

// creaturePlate is the background image one window draws behind its session,
// sized so kitty leaves the creature at creatureWidthShare of the window.
type creaturePlate struct {
	Path string `json:"path"`
	Tint string `json:"tint"`
}

// bakeCreaturePlate returns the plate, or a zero value on anything that would
// stop one. The background is decoration, so a window opens either way.
func bakeCreaturePlate(role string, presence, occupancy float64) creaturePlate {
	art := creatureArt(roleIcon(role))
	if art == nil {
		return creaturePlate{}
	}
	occupancy = clampOccupancy(occupancy)
	path, err := creaturePlatePath(role, art, occupancy)
	if err != nil {
		return creaturePlate{}
	}
	if strings.ContainsAny(path, globMetacharacters) {
		return creaturePlate{}
	}
	if _, err := os.Stat(path); err != nil {
		if err := writeCreaturePlate(path, art, occupancy); err != nil {
			return creaturePlate{}
		}
	}
	return creaturePlate{Path: path, Tint: fmt.Sprintf("%.2f", 1-presence)}
}

// creatureArt reads the largest PNG out of an icns container, which is the one
// committed copy of this art. See docs/aterm-creature.md.
func creatureArt(icns []byte) []byte {
	const header = 8
	if len(icns) < header || string(icns[:4]) != "icns" {
		return nil
	}
	total := int(binary.BigEndian.Uint32(icns[4:header]))
	if total > len(icns) {
		total = len(icns)
	}
	var best []byte
	var bestWidth uint32
	for offset := header; offset+header <= total; {
		size := int(binary.BigEndian.Uint32(icns[offset+4 : offset+header]))
		// A chunk that does not advance, or runs past the container, ends the
		// walk rather than looping or reading someone else's bytes.
		if size < header || offset+size > total {
			break
		}
		entry := icns[offset+header : offset+size]
		if width, ok := pngWidth(entry); ok && width > bestWidth {
			best, bestWidth = entry, width
		}
		offset += size
	}
	return best
}

func pngWidth(entry []byte) (uint32, bool) {
	const ihdr = 24
	if len(entry) < ihdr+4 || string(entry[:8]) != "\x89PNG\r\n\x1a\n" {
		return 0, false
	}
	return binary.BigEndian.Uint32(entry[16:20]), true
}

// creaturePlatePath names the plate after what produced it, so re-drawn art or
// a new recipe lands on a new file rather than a stale hit.
func creaturePlatePath(role string, art []byte, occupancy float64) (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve the cache directory: %w", err)
	}
	// A plate re-cut to a new share is the same bytes in, so the geometry is
	// hashed beside the art or the name cannot tell the two apart.
	recipe := fmt.Sprintf(
		"%s\x00%g\x00%g\x00%g\x00%g\x00%g\x00%g\x00",
		creatureRecipe, creatureWidthShare, creatureInset, creaturePlateAspect,
		creatureGlareKnee, creatureGlareDepth, clampOccupancy(occupancy),
	)
	sum := sha256.Sum256(append([]byte(recipe), art...))
	name := fmt.Sprintf("%s-%s.png", role, hex.EncodeToString(sum[:6]))
	return filepath.Join(root, "aterm", "creature", name), nil
}

func writeCreaturePlate(path string, art []byte, occupancy float64) error {
	source, err := png.Decode(bytes.NewReader(art))
	if err != nil {
		return fmt.Errorf("decode the creature art: %w", err)
	}
	plate := composeCreaturePlate(source, occupancy)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("make the plate directory: %w", err)
	}
	// A half-written plate is a window kitty opens blank, so the file only ever
	// appears complete under its own name.
	staged, err := os.CreateTemp(filepath.Dir(path), ".plate-*.png")
	if err != nil {
		return fmt.Errorf("stage the plate: %w", err)
	}
	defer func() { _ = os.Remove(staged.Name()) }()
	if err := png.Encode(staged, plate); err != nil {
		_ = staged.Close()
		return fmt.Errorf("encode the plate: %w", err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("close the plate: %w", err)
	}
	return os.Rename(staged.Name(), path)
}

// composeCreaturePlate places the art once and never resamples it, at occupancy
// of the window's width. See docs/aterm-creature.md and docs/aterm-pane.md.
func composeCreaturePlate(source image.Image, occupancy float64) image.Image {
	art := source.Bounds()
	occupancy = clampOccupancy(occupancy)
	// region is the part of the canvas the creature is anchored to the right
	// of. At full occupancy it is the whole canvas, which is the launch plate.
	region := float64(art.Dx()) / creatureWidthShare
	width := int(region / occupancy)
	height := int(float64(width) * creaturePlateAspect)
	if height < art.Dy() {
		height = art.Dy()
	}
	inset := image.Point{
		X: int(region * creatureInset),
		Y: int(float64(height) * creatureInset),
	}
	// At this share the art is nearly as tall as the plate, so the top inset
	// takes whatever vertical room is left. See docs/aterm-creature.md.
	origin := image.Point{
		X: max(0, int(region)-art.Dx()-inset.X),
		Y: min(inset.Y, max(0, height-art.Dy())),
	}
	plate := image.NewNRGBA(image.Rect(0, 0, width, height))
	drawTamed(plate, source, origin)
	return plate
}

// drawTamed copies the art with its brightest fills faded, keeping the linework
// that makes the creature read. See docs/aterm-creature.md.
func drawTamed(plate *image.NRGBA, source image.Image, origin image.Point) {
	art := source.Bounds()
	for y := art.Min.Y; y < art.Max.Y; y++ {
		for x := art.Min.X; x < art.Max.X; x++ {
			red, green, blue, alpha := source.At(x, y).RGBA()
			if alpha == 0 {
				continue
			}
			// At() is premultiplied and the plate is not, so every channel is
			// divided back out before anything reads it as a color.
			straight := [3]uint8{
				uint8(red * 255 / alpha),
				uint8(green * 255 / alpha),
				uint8(blue * 255 / alpha),
			}
			faded := float64(alpha) / 0xffff * (1 - glareFade(straight))
			plate.SetNRGBA(origin.X+x-art.Min.X, origin.Y+y-art.Min.Y, color.NRGBA{
				R: straight[0], G: straight[1], B: straight[2],
				A: uint8(math.Round(faded * 255)),
			})
		}
	}
}

// glareFade is the share of a pixel's alpha the rolloff takes, smooth so a fill
// never gains an edge the artist did not draw.
func glareFade(straight [3]uint8) float64 {
	// The square root approximates perceived lightness, which is what decides
	// whether a fill competes with text rather than its physical luminance.
	level := math.Sqrt(relativeLuminance(straight))
	if level <= creatureGlareKnee {
		return 0
	}
	step := (level - creatureGlareKnee) / (1 - creatureGlareKnee)
	return creatureGlareDepth * step * step * (3 - 2*step)
}

// clampOccupancy refuses a share the geometry is undefined for: zero divides,
// and above one would push the creature off the canvas it anchors to.
func clampOccupancy(occupancy float64) float64 {
	if occupancy <= 0 || occupancy > creatureWholeWindow {
		return creatureWholeWindow
	}
	return occupancy
}

// creatureWanted is off wherever the window is being recorded or read by
// something other than a person, matching how the card reads its own motion.
func creatureWanted(disabled bool) bool {
	return !disabled && strings.TrimSpace(os.Getenv(noCreatureEnv)) == ""
}
