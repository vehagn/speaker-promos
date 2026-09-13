// Package assets bundles the fonts a promo card is rendered with.
//
// They are embedded rather than loaded from the system so that a card renders
// identically on any machine, and so that `promo` works with no setup. The
// fonts are SIL Open Font License 1.1; see fonts/OFL.txt.
package assets

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed fonts
var files embed.FS

// Fonts is the bundled font directory, containing static Space Grotesk
// instances and their licence.
func Fonts() fs.FS {
	sub, err := fs.Sub(files, "fonts")
	if err != nil {
		panic(err) // The embedded path is a compile-time constant.
	}
	return sub
}

// Font returns one bundled font by filename.
func Font(name string) ([]byte, error) {
	b, err := fs.ReadFile(Fonts(), name)
	if err != nil {
		return nil, fmt.Errorf("bundled font %q: %w", name, err)
	}
	return b, nil
}

// FontNames lists the bundled font files.
func FontNames() ([]string, error) {
	entries, err := fs.ReadDir(Fonts(), ".")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if name := e.Name(); !e.IsDir() && len(name) > 4 && name[len(name)-4:] == ".ttf" {
			out = append(out, name)
		}
	}
	return out, nil
}
