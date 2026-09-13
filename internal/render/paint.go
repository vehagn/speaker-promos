package render

import (
	"fmt"
	"strconv"
	"strings"
)

// Paint is a colour split into the two attributes SVG 1.1 actually wants: an
// opaque colour and a separate alpha.
//
// This exists because `fill="rgba(255,255,255,0.13)"` is CSS Color syntax that
// SVG presentation attributes do not accept. Inkscape and librsvg fail to parse
// it and fall back to BLACK — which turned the translucent talk panel into a
// solid black box while looking fine in a browser. Themes may still be written
// with rgba(), since that is what anyone copying values from CSS will reach
// for; it is normalised here instead.
type Paint struct {
	// Color is an opaque SVG colour: "#RRGGBB", a named colour, or "none".
	Color string
	// Opacity is the alpha extracted from the input, 0..1.
	Opacity float64
}

// none is the paint for "no fill".
var none = Paint{Color: "none", Opacity: 1}

// parsePaint normalises a theme colour string.
//
// Unrecognised input is passed through with full opacity rather than replaced:
// SVG has many valid colour forms (named colours, hsl()) and silently blanking
// one a user typed deliberately would be worse than letting the renderer
// decide.
func parsePaint(s string) Paint {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "":
		return none
	case "none", "transparent":
		return none
	}

	if rest, ok := cutFold(s, "rgba("); ok {
		if p, ok := parseRGBFunc(rest, true); ok {
			return p
		}
	}
	if rest, ok := cutFold(s, "rgb("); ok {
		if p, ok := parseRGBFunc(rest, false); ok {
			return p
		}
	}
	if strings.HasPrefix(s, "#") {
		return parseHex(s)
	}
	return Paint{Color: s, Opacity: 1}
}

func cutFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return "", false
}

// parseRGBFunc parses the inside of an rgb()/rgba() call.
func parseRGBFunc(body string, withAlpha bool) (Paint, bool) {
	body = strings.TrimSuffix(strings.TrimSpace(body), ")")
	// Both comma and space separated forms are legal CSS.
	fields := strings.FieldsFunc(body, func(r rune) bool {
		return r == ',' || r == ' ' || r == '/' || r == '\t'
	})
	want := 3
	if withAlpha {
		want = 4
	}
	if len(fields) < want {
		return Paint{}, false
	}

	var ch [3]int
	for i := range ch {
		v, err := parseChannel(fields[i])
		if err != nil {
			return Paint{}, false
		}
		ch[i] = v
	}
	opacity := 1.0
	if withAlpha {
		a, err := strconv.ParseFloat(strings.TrimSuffix(fields[3], "%"), 64)
		if err != nil {
			return Paint{}, false
		}
		if strings.HasSuffix(fields[3], "%") {
			a /= 100
		}
		opacity = clamp01(a)
	}
	return Paint{Color: fmt.Sprintf("#%02X%02X%02X", ch[0], ch[1], ch[2]), Opacity: opacity}, true
}

// parseChannel reads a colour channel given as 0-255 or as a percentage.
func parseChannel(s string) (int, error) {
	s = strings.TrimSpace(s)
	if pct, ok := strings.CutSuffix(s, "%"); ok {
		v, err := strconv.ParseFloat(pct, 64)
		if err != nil {
			return 0, err
		}
		return clampChannel(v / 100 * 255), nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return clampChannel(v), nil
}

// parseHex handles #RGB, #RGBA, #RRGGBB and #RRGGBBAA.
func parseHex(s string) Paint {
	h := s[1:]
	expand := func(r byte) string { return string([]byte{r, r}) }
	switch len(h) {
	case 3:
		return Paint{Color: "#" + expand(h[0]) + expand(h[1]) + expand(h[2]), Opacity: 1}
	case 4:
		a, err := strconv.ParseUint(expand(h[3]), 16, 8)
		if err != nil {
			return Paint{Color: s, Opacity: 1}
		}
		return Paint{
			Color:   "#" + expand(h[0]) + expand(h[1]) + expand(h[2]),
			Opacity: float64(a) / 255,
		}
	case 8:
		a, err := strconv.ParseUint(h[6:8], 16, 8)
		if err != nil {
			return Paint{Color: s, Opacity: 1}
		}
		return Paint{Color: "#" + h[:6], Opacity: float64(a) / 255}
	default:
		return Paint{Color: s, Opacity: 1}
	}
}

// fillAttrs renders a paint as fill attributes, scaled by an extra opacity
// factor from the caller (a theme text style's own opacity).
func (p Paint) fillAttrs(extra float64) string {
	return p.attrs("fill", extra)
}

// strokeAttrs renders a paint as stroke attributes.
func (p Paint) strokeAttrs(extra float64) string {
	return p.attrs("stroke", extra)
}

func (p Paint) attrs(prop string, extra float64) string {
	if extra <= 0 {
		extra = 1
	}
	opacity := clamp01(p.Opacity * extra)
	out := fmt.Sprintf("%s=%q", prop, p.Color)
	if p.Color == "none" {
		return out
	}
	if opacity < 1 {
		out += fmt.Sprintf(" %s-opacity=%q", prop, num(opacity))
	}
	return out
}

func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}

func clampChannel(v float64) int {
	i := int(v + 0.5)
	switch {
	case i < 0:
		return 0
	case i > 255:
		return 255
	default:
		return i
	}
}
