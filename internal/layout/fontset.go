package layout

import (
	"fmt"

	"github.com/vehagn/speaker-promos/internal/theme"
)

// FontSet is a theme's faces, parsed once and shared across every card in a
// run. Parsing and metric caching are per-face, so reusing a set across 36
// talks is what keeps a bulk render fast.
type FontSet struct {
	faces map[string]*Font
	// raw keeps the original bytes so the renderer can embed the faces it used.
	raw map[string][]byte
}

// NewFontSet parses every face a theme declares.
func NewFontSet(t *theme.Theme) (*FontSet, error) {
	fs := &FontSet{
		faces: make(map[string]*Font, len(t.Fonts)),
		raw:   make(map[string][]byte, len(t.Fonts)),
	}
	for key, face := range t.Fonts {
		data, err := face.Data()
		if err != nil {
			return nil, err
		}
		f, err := ParseFont(data, face.Family, face.Weight, face.Style)
		if err != nil {
			return nil, err
		}
		fs.faces[key] = f
		fs.raw[key] = data
	}
	return fs, nil
}

// Face returns a parsed font by theme key.
func (fs *FontSet) Face(key string) (*Font, error) {
	if key == "" {
		key = "body"
	}
	f, ok := fs.faces[key]
	if !ok {
		return nil, fmt.Errorf("theme declares no font %q", key)
	}
	return f, nil
}

// Raw returns the font file bytes for a theme key, for embedding.
func (fs *FontSet) Raw(key string) ([]byte, bool) {
	b, ok := fs.raw[key]
	return b, ok
}

// FitStyle lays text out according to a theme text style.
func (fs *FontSet) FitStyle(st theme.TextStyle, text string, maxWidth float64) (Block, *Font, error) {
	f, err := fs.Face(st.Face)
	if err != nil {
		return Block{}, nil, err
	}
	lineHeight := st.LineHeight
	if lineHeight <= 0 {
		lineHeight = 1.15
	}
	block := Fit(f, text, st.MinSize, st.MaxSize, st.Tracking, lineHeight, maxWidth, st.MaxLines)
	return block, f, nil
}
