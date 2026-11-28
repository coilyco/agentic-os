package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // registers the decoder a GIF upload needs
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// imageThumbFeature is how a client knows the daemon answers `image_thumb`: an image
// Kai sent, read back by its upload path so another device can show it (COI-3800).
const imageThumbFeature = "image-thumb"

const (
	// thumbSide is the longest side of a thumbnail, twice a 240px inline picture.
	thumbSide = 480
	// thumbMaxPixels refuses to decode a picture whose header claims more, since a few
	// kilobytes of PNG can declare a size that takes gigabytes to decode.
	thumbMaxPixels = 40_000_000
	// reasonImageGone is an upload path in the seat's directory that holds no file,
	// because the seat ended or the file was never there.
	reasonImageGone = "image_gone"
)

// imageThumb answers `image_thumb`: an upload's thumbnail, or with `full` its bytes.
// Only a file directly in the seat's own upload directory is ever read.
func (d *daemon) imageThumb(cl *client, message frame) error {
	if d.imageRoot == "" {
		return fmt.Errorf("unknown frame type %q", message.Type)
	}
	if err := d.typingRefusal(cl, "a process inside an aterm session cannot read a seat's sent images"); err != nil {
		return err
	}
	s, err := d.oneTarget(message.Session)
	if err != nil {
		return err
	}
	data, err := readUpload(filepath.Join(d.imageRoot, imageDirName(s)), message.Path)
	if err != nil {
		return err
	}
	reply := frame{Type: "image_thumb", ID: message.ID, Session: s.name, Path: message.Path, Data: data}
	if !message.Full {
		reply.Data, reply.Width, reply.Height = thumbnail(data)
	}
	d.logf("%s served a sent image of %d bytes", s.name, len(reply.Data))
	return cl.c.write(reply)
}

// readUpload reads path only when it sits directly in dir. It is compared as given, never
// resolved, so `..`, a nested name and a symlink are refused rather than followed.
func readUpload(dir, path string) ([]byte, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Dir(path) != dir {
		return nil, withReason(reasonBadPath, errors.New("that path is not an image this seat was sent"))
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, withReason(reasonImageGone, errors.New("the host no longer holds that image"))
	}
	if err != nil {
		return nil, withReason(reasonBadPath, errors.New("that path is not an image this seat was sent"))
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, withReason(reasonBadPath, errors.New("that path is not an image this seat was sent"))
	}
	data, err := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, withReason(reasonTooLarge, errors.New("that file is larger than any upload"))
	}
	if _, ok := sniffImage(data); !ok {
		return nil, withReason(reasonNotAnImage, errors.New("that file is not an image"))
	}
	return data, nil
}

// thumbnail fits a picture to thumbSide. What it cannot decode (WebP) goes back as it is,
// since an upload already fits a frame, and zero sizes say so.
func thumbnail(data []byte) ([]byte, int, int) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width*config.Height > thumbMaxPixels {
		return data, 0, 0
	}
	if config.Width <= thumbSide && config.Height <= thumbSide {
		return data, config.Width, config.Height
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data, 0, 0
	}
	small := shrink(source, thumbSide)
	var out bytes.Buffer
	// A PNG keeps its transparency and a screenshot's text edges, anything else is a photo.
	if format == "png" {
		err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&out, small)
	} else {
		err = jpeg.Encode(&out, small, &jpeg.Options{Quality: 82})
	}
	if err != nil {
		return data, 0, 0
	}
	bounds := small.Bounds()
	return out.Bytes(), bounds.Dx(), bounds.Dy()
}

// shrink averages each box of source pixels into one, so a screenshot's text thins
// rather than breaking up as nearest-neighbour would leave it.
func shrink(source image.Image, side int) *image.NRGBA {
	in := source.Bounds()
	width, height := in.Dx(), in.Dy()
	if width >= height {
		width, height = side, max(1, in.Dy()*side/in.Dx())
	} else {
		width, height = max(1, in.Dx()*side/in.Dy()), side
	}
	flat := image.NewNRGBA64(in)
	draw.Draw(flat, in, source, in.Min, draw.Src)
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		y0, y1 := in.Min.Y+y*in.Dy()/height, in.Min.Y+(y+1)*in.Dy()/height
		for x := range width {
			x0, x1 := in.Min.X+x*in.Dx()/width, in.Min.X+(x+1)*in.Dx()/width
			var r, g, b, a, n uint64
			for sy := y0; sy < max(y1, y0+1); sy++ {
				for sx := x0; sx < max(x1, x0+1); sx++ {
					c := flat.NRGBA64At(sx, sy)
					// Weighted by alpha, so a transparent pixel's colour does not bleed in.
					r += uint64(c.R) * uint64(c.A)
					g += uint64(c.G) * uint64(c.A)
					b += uint64(c.B) * uint64(c.A)
					a += uint64(c.A)
					n++
				}
			}
			if a == 0 {
				continue
			}
			out.SetNRGBA(x, y, color.NRGBA{R: uint8(r / a >> 8), G: uint8(g / a >> 8), B: uint8(b / a >> 8), A: uint8(a / n >> 8)})
		}
	}
	return out
}
