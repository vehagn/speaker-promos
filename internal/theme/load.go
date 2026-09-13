package theme

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vehagn/speaker-promos/assets"
)

// Data returns the raw font bytes for a face.
//
// A bare filename resolves against the bundled fonts, so the built-in theme
// needs nothing on disk. A path containing a separator is read from the
// filesystem, which is how a custom theme brings its own typeface.
func (f Face) Data() ([]byte, error) {
	if filepath.Base(f.File) != f.File {
		b, err := os.ReadFile(f.File)
		if err != nil {
			return nil, fmt.Errorf("theme font %q: %w", f.File, err)
		}
		return b, nil
	}
	return assets.Font(f.File)
}
