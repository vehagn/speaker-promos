package main

import (
	"errors"
	"fmt"
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
type converter struct {
	name string
	// args builds the command line for one conversion at a given pixel width.
	args func(in, out string, width int) []string
	// embedsFonts reports whether the tool honours base64 @font-face rules in
	// the SVG. Inkscape and librsvg do not, so a card exported with them needs
	// the fonts installed system-wide — see `promo fonts install`.
	embedsFonts bool
}

// converters are tried in order. resvg leads because it is the only one of the
// four that honours an embedded @font-face, so it reproduces the card exactly
// as a browser shows it; the others need `promo fonts install` first.
var converters = []converter{
	{
		name:        "resvg",
		embedsFonts: true,
		args: func(in, out string, width int) []string {
			return []string{"--width", strconv.Itoa(width), in, out}
		},
	},
	{
		name: "inkscape",
		args: func(in, out string, width int) []string {
			return []string{"--export-type=png", "--export-filename=" + out,
				"--export-width=" + strconv.Itoa(width), in}
		},
	},
	{
		name: "rsvg-convert",
		args: func(in, out string, width int) []string {
			return []string{"--width=" + strconv.Itoa(width), "--keep-aspect-ratio",
				"--output=" + out, in}
		},
	},
	{
		name: "magick",
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
func findConverter() (converter, string, bool) {
	for _, c := range converters {
		if path, err := exec.LookPath(c.name); err == nil {
			return c, path, true
		}
	}
	return converter{}, "", false
}

// noConverterMessage explains what to install, for a --png run with no tool.
const noConverterMessage = `note: --png needs an SVG rasteriser on PATH and none was found.
The SVGs above were still written. Install one of:
  brew install resvg          # honours the embedded fonts; closest to a browser
  brew install librsvg        # rsvg-convert
  brew install inkscape
  brew install imagemagick    # magick
`

// exportPNG rasterises an SVG that has already been written to disk, returning
// the path of the PNG.
func exportPNG(c converter, svgPath, pngPath string, width int) error {
	cmd := exec.Command(c.name, c.args(svgPath, pngPath, width)...)
	// Inkscape and magick are chatty on stderr even when they succeed, so their
	// output is only surfaced if the conversion actually fails.
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && len(out) > 0 {
			return fmt.Errorf("%s: %w: %s", c.name, err, trimOutput(out))
		}
		return fmt.Errorf("%s: %w", c.name, err)
	}
	// A tool can exit 0 and still write nothing, which is worth catching here
	// rather than leaving a missing file to be discovered later.
	if fi, statErr := os.Stat(pngPath); statErr != nil || fi.Size() == 0 {
		return fmt.Errorf("%s exited cleanly but wrote no PNG: %s", c.name, pngPath)
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
