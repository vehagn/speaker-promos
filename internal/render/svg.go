// Package render draws promo cards as SVG.
package render

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/vehagn/speaker-promos/internal/layout"
)

// canvas accumulates SVG markup.
type canvas struct {
	b strings.Builder
}

func (c *canvas) writef(format string, args ...any) {
	fmt.Fprintf(&c.b, format, args...)
}

func (c *canvas) write(s string) { c.b.WriteString(s) }

func (c *canvas) String() string { return c.b.String() }

// escape encodes text for SVG character data.
//
// Promo text is third-party: talk titles and speaker bios come from a CMS and
// contain ampersands and angle brackets. Unescaped, those produce an SVG that
// no renderer will parse.
func escape(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// num formats a coordinate, trimming trailing zeros so the output stays
// readable and diffable.
func num(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// textBlock draws a laid-out block of text.
//
// anchor is an SVG text-anchor ("start", "middle", "end") and x is interpreted
// against it, so the same call handles both centred portrait cards and
// left-aligned landscape ones. y is the baseline of the first line.
//
// Lines are emitted as separate <text> elements rather than <tspan>s of one
// element because each line may itself be split into font runs; nesting runs
// inside per-line tspans inside one text element is valid but far harder to
// read in the output, and these files are meant to stay editable.
func (c *canvas) textBlock(block layout.Block, f *layout.Font, fallbackFamily string, x, y float64, anchor string, fill Paint, opacity float64) {
	for i, line := range block.Lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		c.textLine(line, f, fallbackFamily, x, y+float64(i)*block.LineHeight, block.Size, block.Tracking, anchor, fill, opacity)
	}
}

// textLine draws one line, splitting it into font runs where the primary face
// lacks glyphs.
func (c *canvas) textLine(line string, f *layout.Font, fallbackFamily string, x, y, size, tracking float64, anchor string, fill Paint, opacity float64) {
	// Alpha rides on fill-opacity rather than the element's `opacity`, so that
	// a translucent colour and a translucent style compose to one value.
	attrs := fmt.Sprintf(
		`x="%s" y="%s" font-family=%q font-size="%s" font-weight="%d" %s text-anchor=%q`,
		num(x), num(y), f.Family, num(size), f.Weight, fill.fillAttrs(opacity), anchor)
	if tracking != 0 {
		attrs += fmt.Sprintf(` letter-spacing="%s"`, num(tracking*size))
	}
	if f.Style != "" && f.Style != "normal" {
		attrs += fmt.Sprintf(` font-style=%q`, f.Style)
	}

	runs := layout.SplitRuns(f, line)
	if !layout.HasFallback(runs) {
		c.writef("  <text %s>%s</text>\n", attrs, escape(line))
		return
	}

	// Mixed line: the runs go in tspans so the emoji run can name a different
	// family. With text-anchor other than "start" the anchor applies to the
	// whole element, which is what keeps a centred title with an emoji in it
	// centred.
	c.writef("  <text %s>", attrs)
	for _, r := range runs {
		if r.Fallback {
			c.writef("<tspan font-family=%q>%s</tspan>", fallbackFamily, escape(r.Text))
		} else {
			c.writef("<tspan>%s</tspan>", escape(r.Text))
		}
	}
	c.write("</text>\n")
}

// fontFaceCSS builds the @font-face rules embedding the given faces.
//
// Embedding makes a card self-contained: it renders identically in a browser or
// resvg on a machine with no fonts installed. Inkscape ignores these rules —
// `promo fonts install` covers that case.
func fontFaceCSS(faces []embeddedFace) string {
	if len(faces) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("  <style>\n")
	for _, f := range faces {
		style := f.Style
		if style == "" {
			style = "normal"
		}
		b.WriteString("    @font-face {\n")
		fmt.Fprintf(&b, "      font-family: %q;\n", f.Family)
		fmt.Fprintf(&b, "      font-weight: %d;\n", f.Weight)
		fmt.Fprintf(&b, "      font-style: %s;\n", style)
		fmt.Fprintf(&b, "      src: url(\"data:font/ttf;base64,%s\") format(\"truetype\");\n",
			base64.StdEncoding.EncodeToString(f.Data))
		b.WriteString("    }\n")
	}
	b.WriteString("  </style>\n")
	return b.String()
}

// embeddedFace is a font to write into a card's <style> block.
type embeddedFace struct {
	Family string
	Weight int
	Style  string
	Data   []byte
}

// inlineSVG prepares an external SVG document (a conference logo) for
// embedding, scaled to fit a width x height box at (x, y).
//
// The logo is wrapped in a <svg> element with its own viewBox rather than
// having its markup spliced in: the source has its own coordinate system and
// may define ids that would collide with the card's. A nested <svg> establishes
// a new viewport, so `preserveAspectRatio` does the scaling and no coordinate
// maths is needed here.
func inlineSVG(doc string, x, y, width, height float64) string {
	inner, viewBox, ok := svgBody(doc)
	if !ok {
		return ""
	}
	return fmt.Sprintf(
		"  <svg x=%q y=%q width=%q height=%q viewBox=%q preserveAspectRatio=\"xMidYMid meet\">%s</svg>\n",
		num(x), num(y), num(width), num(height), viewBox, inner)
}
