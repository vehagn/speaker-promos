// Package theme holds the look of a promo card: colours, fonts and the handful
// of geometric knobs the renderer needs.
//
// What is themeable and what is not is a deliberate split. Colours, fonts,
// paddings, corner radii, photo sizes and per-element type scales live here and
// can be overridden from a YAML file. The *arrangement* of a card — what sits
// above what — stays in internal/render. Exposing every coordinate would make
// the config enormous and easy to put into an invalid state, while giving very
// little that a user actually wants to change.
package theme

import (
	"embed"
	"fmt"
	"maps"
	"os"
	"slices"

	"gopkg.in/yaml.v3"
)

//go:embed default.yaml
var builtin embed.FS

// Palette is the card's colour set. Values are any valid SVG paint string, so
// "#1D4ED8" and "rgba(255,255,255,0.2)" are both fine.
type Palette struct {
	GradientFrom string `yaml:"gradientFrom"`
	GradientTo   string `yaml:"gradientTo"`
	Text         string `yaml:"text"`
	TextMuted    string `yaml:"textMuted"`
	Accent       string `yaml:"accent"`
	CardFill     string `yaml:"cardFill"`
	CardStroke   string `yaml:"cardStroke"`
	PhotoStroke  string `yaml:"photoStroke"`
}

// Face is one embeddable font face.
type Face struct {
	// Family is the CSS family name written into the SVG.
	Family string `yaml:"family"`
	// File names a font in the bundled assets/fonts directory, or an absolute
	// path to a TTF on disk for a custom theme.
	File string `yaml:"file"`
	// Weight and Style are emitted in the @font-face rule so that a renderer
	// picks the right face when several share a family name.
	Weight int    `yaml:"weight"`
	Style  string `yaml:"style"`
}

// TextStyle describes one text element on the card.
//
// MinSize/MaxSize form an autofit range: the renderer starts at MaxSize and
// steps down until the text wraps within MaxLines. Setting them equal disables
// autofit.
type TextStyle struct {
	// Face selects a key from Theme.Fonts.
	Face     string  `yaml:"face"`
	MinSize  float64 `yaml:"minSize"`
	MaxSize  float64 `yaml:"maxSize"`
	MaxLines int     `yaml:"maxLines"`
	// LineHeight is a multiple of the font size.
	LineHeight float64 `yaml:"lineHeight"`
	// Tracking is letter-spacing as a fraction of the font size.
	Tracking float64 `yaml:"tracking"`
	// Uppercase renders the text in capitals (used for eyebrow and meta lines).
	Uppercase bool `yaml:"uppercase"`
	// Color is a palette key ("text", "textMuted", "accent") or a literal paint.
	Color   string  `yaml:"color"`
	Opacity float64 `yaml:"opacity"`
}

// Geometry is the frame and spacing of one card size.
type Geometry struct {
	Width  int `yaml:"width"`
	Height int `yaml:"height"`
	// Pad is the outer margin kept clear of content.
	Pad int `yaml:"pad"`
	// Gap is the default vertical space between stacked blocks.
	Gap int `yaml:"gap"`
	// Radius is the corner radius of the photo and the talk card.
	Radius int `yaml:"radius"`
	// LogoWidth is the rendered width of the conference wordmark.
	LogoWidth int `yaml:"logoWidth"`
	// PhotoSize is the edge length of a single speaker's photo. Cards with
	// several speakers shrink it; see render.
	PhotoSize int `yaml:"photoSize"`
	// Text maps element names to their type styles. The renderer looks up
	// "eyebrow", "name", "role", "talk", "meta" and "footer".
	Text map[string]TextStyle `yaml:"text"`
}

// Theme is a complete card design.
type Theme struct {
	Name    string `yaml:"name"`
	Palette Palette
	// Fonts maps a face key ("heading", "body") to the face to embed.
	Fonts map[string]Face `yaml:"fonts"`
	// EmojiFallback is the CSS family list appended for runs the text font
	// cannot render. Emoji are common in talk titles and no text font covers
	// them, so these runs are split out and handed to a system colour font.
	EmojiFallback string `yaml:"emojiFallback"`
	// Geometry maps a size name ("portrait", "landscape") to its frame.
	Geometry map[string]Geometry `yaml:"geometry"`
}

// Default returns the built-in Cloud Native Days 2026 theme.
func Default() (*Theme, error) {
	b, err := builtin.ReadFile("default.yaml")
	if err != nil {
		return nil, err
	}
	return parse(b, "built-in theme")
}

// Load reads a theme from a YAML file, filling anything it omits from the
// built-in theme. Overriding just a colour therefore needs a three-line file
// rather than a full copy.
func Load(path string) (*Theme, error) {
	base, err := Default()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return base, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading theme: %w", err)
	}
	// Decode the overlay ONTO a copy of the default so that scalar fields it
	// omits keep their built-in values — that is what makes a three-line theme
	// file work.
	//
	// The two map fields are cleared first, and this is load-bearing rather
	// than tidiness: a struct copy shares its maps with the original, and
	// unmarshalling into a non-nil map MUTATES it. Left populated, YAML would
	// overwrite the default's own geometry with the overlay's partial entries,
	// and the merge below would then have nothing intact left to merge against.
	// Nilled, YAML allocates fresh maps holding only what the file declares.
	merged := *base
	merged.Fonts = nil
	merged.Geometry = nil
	if err := yaml.Unmarshal(b, &merged); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	merged.Fonts = mergeFaces(base.Fonts, merged.Fonts)
	merged.Geometry = mergeGeometry(base.Geometry, merged.Geometry)

	// Only the merged result has to be coherent; the overlay on its own is
	// legitimately partial.
	if err := merged.validate(path); err != nil {
		return nil, err
	}
	return &merged, nil
}

// DefaultYAML returns the built-in theme's source, for `promo theme dump`.
func DefaultYAML() ([]byte, error) { return builtin.ReadFile("default.yaml") }

func parse(b []byte, source string) (*Theme, error) {
	var t Theme
	if err := yaml.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", source, err)
	}
	if err := t.validate(source); err != nil {
		return nil, err
	}
	return &t, nil
}

func mergeFaces(base, overlay map[string]Face) map[string]Face {
	out := maps.Clone(base)
	if out == nil {
		out = map[string]Face{}
	}
	for k, v := range overlay {
		merged := out[k]
		if v.Family != "" {
			merged.Family = v.Family
		}
		if v.File != "" {
			merged.File = v.File
		}
		if v.Weight != 0 {
			merged.Weight = v.Weight
		}
		if v.Style != "" {
			merged.Style = v.Style
		}
		out[k] = merged
	}
	return out
}

func mergeGeometry(base, overlay map[string]Geometry) map[string]Geometry {
	out := maps.Clone(base)
	if out == nil {
		out = map[string]Geometry{}
	}
	for name, og := range overlay {
		bg, ok := out[name]
		if !ok {
			out[name] = og
			continue
		}
		// Re-decoding the overlay onto the base geometry would be cleaner, but
		// the overlay has already been parsed, so copy non-zero fields.
		if og.Width != 0 {
			bg.Width = og.Width
		}
		if og.Height != 0 {
			bg.Height = og.Height
		}
		if og.Pad != 0 {
			bg.Pad = og.Pad
		}
		if og.Gap != 0 {
			bg.Gap = og.Gap
		}
		if og.Radius != 0 {
			bg.Radius = og.Radius
		}
		if og.LogoWidth != 0 {
			bg.LogoWidth = og.LogoWidth
		}
		if og.PhotoSize != 0 {
			bg.PhotoSize = og.PhotoSize
		}
		bg.Text = mergeText(bg.Text, og.Text)
		out[name] = bg
	}
	return out
}

func mergeText(base, overlay map[string]TextStyle) map[string]TextStyle {
	out := maps.Clone(base)
	if out == nil {
		out = map[string]TextStyle{}
	}
	for k, v := range overlay {
		merged := out[k]
		if v.Face != "" {
			merged.Face = v.Face
		}
		if v.MinSize != 0 {
			merged.MinSize = v.MinSize
		}
		if v.MaxSize != 0 {
			merged.MaxSize = v.MaxSize
		}
		if v.MaxLines != 0 {
			merged.MaxLines = v.MaxLines
		}
		if v.LineHeight != 0 {
			merged.LineHeight = v.LineHeight
		}
		if v.Tracking != 0 {
			merged.Tracking = v.Tracking
		}
		if v.Color != "" {
			merged.Color = v.Color
		}
		if v.Opacity != 0 {
			merged.Opacity = v.Opacity
		}
		merged.Uppercase = v.Uppercase || merged.Uppercase
		out[k] = merged
	}
	return out
}

// Sizes returns the card sizes a theme defines, sorted for stable output.
func (t *Theme) Sizes() []string {
	return slices.Sorted(maps.Keys(t.Geometry))
}

// Size returns one card geometry by name.
func (t *Theme) Size(name string) (Geometry, error) {
	g, ok := t.Geometry[name]
	if !ok {
		return Geometry{}, fmt.Errorf("theme %q has no size %q (has %v)", t.Name, name, t.Sizes())
	}
	return g, nil
}

// Paint resolves a TextStyle colour, which may name a palette entry.
func (t *Theme) Paint(color string) string {
	switch color {
	case "", "text":
		return t.Palette.Text
	case "textMuted":
		return t.Palette.TextMuted
	case "accent":
		return t.Palette.Accent
	default:
		return color
	}
}

func (t *Theme) validate(source string) error {
	if len(t.Fonts) == 0 {
		return fmt.Errorf("%s: no fonts defined", source)
	}
	for key, f := range t.Fonts {
		if f.Family == "" || f.File == "" {
			return fmt.Errorf("%s: font %q needs both family and file", source, key)
		}
	}
	for name, g := range t.Geometry {
		if g.Width <= 0 || g.Height <= 0 {
			return fmt.Errorf("%s: size %q has a non-positive dimension", source, name)
		}
		for element, st := range g.Text {
			if st.Face != "" {
				if _, ok := t.Fonts[st.Face]; !ok {
					return fmt.Errorf("%s: size %q element %q uses undefined face %q",
						source, name, element, st.Face)
				}
			}
			if st.MaxSize > 0 && st.MinSize > st.MaxSize {
				return fmt.Errorf("%s: size %q element %q has minSize > maxSize",
					source, name, element)
			}
		}
	}
	return nil
}
