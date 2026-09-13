// Package raster turns a rendered SVG card into PNG and JPEG.
package raster

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"strconv"
)

// converter rasterises an SVG file to PNG using an external tool.
//
// Rendering SVG properly means shipping a rasteriser, and the only pure-Go one
// worth having is a large dependency for a feature most runs will not use. The
// tools below are already installed on a machine that does graphics work, so
// PNG export shells out and says so plainly when nothing is available.
type Converter struct {
	Name string
	// args builds the command line for one conversion at a given pixel width.
	args func(in, out string, width int) []string
	// embedsFonts reports whether the tool honours base64 @font-face rules in
	// the SVG. Inkscape and librsvg do not, so a card exported with them needs
	// the fonts installed system-wide — see `promo fonts install`.
	EmbedsFonts bool
}

// converters are tried in order. resvg leads because it is the only one of the
// four that honours an embedded @font-face, so it reproduces the card exactly
// as a browser shows it; the others need `promo fonts install` first.
var converters = []Converter{
	{
		Name:        "resvg",
		EmbedsFonts: true,
		args: func(in, out string, width int) []string {
			return []string{"--width", strconv.Itoa(width), in, out}
		},
	},
	{
		Name: "inkscape",
		args: func(in, out string, width int) []string {
			return []string{"--export-type=png", "--export-filename=" + out,
				"--export-width=" + strconv.Itoa(width), in}
		},
	},
	{
		Name: "rsvg-convert",
		args: func(in, out string, width int) []string {
			return []string{"--width=" + strconv.Itoa(width), "--keep-aspect-ratio",
				"--output=" + out, in}
		},
	},
	{
		Name: "magick",
		args: func(in, out string, width int) []string {
			// -density before the input raises the rasterisation resolution;
			// magick otherwise renders at 72 dpi and upscales, which turns
			// crisp text into mush.
			return []string{"-background", "none", "-density", "192", in,
				"-resize", strconv.Itoa(width), out}
		},
	},
}

// findConverter returns the first available rasteriser, or a nil converter when
// none is installed.
func Find() (Converter, string, bool) {
	for _, c := range converters {
		if path, err := exec.LookPath(c.Name); err == nil {
			return c, path, true
		}
	}
	return Converter{}, "", false
}

// noConverterMessage explains what to install, for a --png run with no tool.
const NoConverterMessage = `note: PNG and JPEG need an SVG rasteriser on PATH and none was found.
The SVGs, copy and manifests were still written. Install one of:
  brew install resvg          # honours the embedded fonts; closest to a browser
  brew install librsvg        # rsvg-convert
  brew install inkscape
  brew install imagemagick    # magick
`

// exportPNG rasterises an SVG that has already been written to disk, returning
// the path of the PNG.
func (c Converter) PNG(svgPath, pngPath string, width int) error {
	cmd := exec.Command(c.Name, c.args(svgPath, pngPath, width)...)
	// Inkscape and magick are chatty on stderr even when they succeed, so their
	// output is only surfaced if the conversion actually fails.
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(out) > 0 {
			return fmt.Errorf("%s: %w: %s", c.Name, err, trimOutput(out))
		}
		return fmt.Errorf("%s: %w", c.Name, err)
	}
	// A tool can exit 0 and still write nothing, which is worth catching here
	// rather than leaving a missing file to be discovered later.
	if fi, statErr := os.Stat(pngPath); statErr != nil || fi.Size() == 0 {
		return fmt.Errorf("%s exited cleanly but wrote no PNG: %s", c.Name, pngPath)
	}
	return nil
}

// trimOutput reduces tool output to its first few lines for an error message.
func trimOutput(b []byte) string {
	const max = 400
	s := string(b)
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// JPEG re-encodes an existing PNG as JPEG.
//
// It goes through the PNG rather than asking the rasteriser for a JPEG
// directly, because only one of the four tools can write JPEG at all. Go's
// image/jpeg is in the standard library, so this way JPEG is available whenever
// PNG is, with no extra dependency and no second choice of tool to explain.
//
// The card is composited onto white first. JPEG has no alpha channel, and a
// card whose theme leaves any transparency would otherwise come out with black
// where the background should be.
func JPEG(pngPath, jpegPath string, quality int) error {
	in, err := os.Open(pngPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", pngPath, err)
	}
	defer in.Close()

	src, err := png.Decode(in)
	if err != nil {
		return fmt.Errorf("decoding %s: %w", pngPath, err)
	}

	bounds := src.Bounds()
	flat := image.NewRGBA(bounds)
	draw.Draw(flat, bounds, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, bounds, src, bounds.Min, draw.Over)

	out, err := os.Create(jpegPath)
	if err != nil {
		return fmt.Errorf("writing %s: %w", jpegPath, err)
	}
	defer out.Close()
	if quality <= 0 {
		quality = 88
	}
	if err := jpeg.Encode(out, flat, &jpeg.Options{Quality: quality}); err != nil {
		return fmt.Errorf("encoding %s: %w", jpegPath, err)
	}
	return out.Close()
}
