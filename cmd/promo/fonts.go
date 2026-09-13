package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/vehagn/speaker-promos/assets"
)

func cmdFonts(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: promo fonts install | promo fonts list")
	}
	switch args[0] {
	case "list":
		names, err := assets.FontNames()
		if err != nil {
			return err
		}
		for _, n := range names {
			fmt.Println(n)
		}
		return nil
	case "install":
		return installFonts()
	default:
		return fmt.Errorf("unknown fonts subcommand %q (want install or list)", args[0])
	}
}

// userFontDir is where a per-user font install goes on this platform.
func userFontDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Fonts"), nil
	case "windows":
		return filepath.Join(home, "AppData", "Local", "Microsoft", "Windows", "Fonts"), nil
	default:
		return filepath.Join(home, ".local", "share", "fonts"), nil
	}
}

// installFonts copies the bundled fonts into the user font directory.
//
// Rendered cards embed their fonts, so browsers and resvg need nothing
// installed. Inkscape is the reason this exists: it ignores base64 @font-face
// rules, so a card opened there for hand-tweaking falls back to a default
// typeface unless the real font is available to the system.
func installFonts() error {
	dir, err := userFontDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	names, err := assets.FontNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		data, err := assets.Font(name)
		if err != nil {
			return err
		}
		dest := filepath.Join(dir, name)
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", dest, err)
		}
		fmt.Println("installed", dest)
	}
	fmt.Printf("\n%d fonts installed (SIL Open Font License 1.1).\n", len(names))
	fmt.Println("Restart Inkscape to pick them up.")
	return nil
}
