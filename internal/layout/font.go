// Package layout measures and wraps text using real font metrics.
//
// Promo cards are generated from data of wildly varying length — talk titles in
// the 2026 program run from 24 to 96 characters — so text has to be wrapped and
// scaled to fit rather than positioned at fixed coordinates. Doing that with
// guessed character widths produces lines that overflow or stop short, which is
// exactly the kind of thing that makes generated artwork look generated.
package layout

import (
	"fmt"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// unitsPerEmScale is the fixed.Int26_6 scale used when querying glyph advances.
// Advances are requested at this pseudo-size and then divided by it, giving a
// ratio of the em that can be multiplied by any real font size. Using a large
// value keeps rounding error well below a pixel.
const unitsPerEmScale = 1 << 10

// Font is a parsed font face that can measure text.
type Font struct {
	Family string
	Weight int
	Style  string

	font *sfnt.Font
	buf  sfnt.Buffer
	// advances caches the per-rune advance as a fraction of the em. Measuring
	// dominates autofit — five candidate sizes over a dozen text elements
	// re-measures the same runes repeatedly — and glyph lookup is the expensive
	// part.
	advances map[rune]float64
	// missing records runes with no glyph in this font, so callers can split
	// them into a fallback run.
	missing map[rune]bool
}

// ParseFont reads a TTF/OTF and prepares it for measurement.
func ParseFont(data []byte, family string, weight int, style string) (*Font, error) {
	f, err := sfnt.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parsing font %s: %w", family, err)
	}
	if style == "" {
		style = "normal"
	}
	if weight == 0 {
		weight = 400
	}
	return &Font{
		Family:   family,
		Weight:   weight,
		Style:    style,
		font:     f,
		advances: make(map[rune]float64),
		missing:  make(map[rune]bool),
	}, nil
}

// Has reports whether the font can render a rune.
func (f *Font) Has(r rune) bool {
	if _, ok := f.advances[r]; ok {
		return true
	}
	if f.missing[r] {
		return false
	}
	idx, err := f.font.GlyphIndex(&f.buf, r)
	return err == nil && idx != 0
}

// advance returns a rune's advance width as a fraction of the em.
//
// Runes the font cannot render are charged a 1.0 em advance. That is a
// deliberate approximation for emoji, which are typically about one em wide in
// the colour fonts that end up rendering them; it keeps a title containing a
// flag from wrapping wildly wrong without pulling in a second font stack just
// to measure it.
func (f *Font) advance(r rune) float64 {
	if a, ok := f.advances[r]; ok {
		return a
	}
	if f.missing[r] {
		return 1.0
	}

	idx, err := f.font.GlyphIndex(&f.buf, r)
	if err != nil || idx == 0 {
		f.missing[r] = true
		return 1.0
	}
	adv, err := f.font.GlyphAdvance(&f.buf, idx, fixed.I(unitsPerEmScale), 0)
	if err != nil {
		f.missing[r] = true
		return 1.0
	}
	ratio := float64(adv) / float64(fixed.I(unitsPerEmScale))
	f.advances[r] = ratio
	return ratio
}

// Measure returns the rendered width of s at the given font size, including the
// extra width that letter-spacing adds.
//
// Tracking is a fraction of the font size and, matching SVG's letter-spacing, is
// applied after every glyph including the last. Ignoring that trailing space
// would let a tracked line overflow its box by one space's worth.
func (f *Font) Measure(s string, size, tracking float64) float64 {
	var total float64
	for _, r := range s {
		total += f.advance(r)
	}
	n := len([]rune(s))
	return total*size + float64(n)*tracking*size
}
