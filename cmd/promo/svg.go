package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vehagn/speaker-promos/internal/cache"
	"github.com/vehagn/speaker-promos/internal/render"
	"github.com/vehagn/speaker-promos/internal/theme"
)

func cmdSVG(args []string) error {
	fs := newFlagSet("svg")
	var common commonFlags
	common.register(fs)
	all := fs.Bool("all", false, "render every talk in the program")
	out := fs.String("out", "out", "directory to write into")
	sizes := fs.String("size", "portrait", "card sizes to render: portrait, landscape, or both")
	themePath := fs.String("theme", "", "theme YAML to merge over the built-in theme")
	noPhotos := fs.Bool("no-photos", false, "skip speaker photos (renders initials instead)")
	stripEmoji := fs.Bool("strip-emoji", false, "remove emoji rather than relying on a system emoji font")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	th, err := theme.Load(*themePath)
	if err != nil {
		return err
	}
	wanted, err := resolveSizes(th, *sizes)
	if err != nil {
		return err
	}

	program, err := common.load()
	if err != nil {
		return err
	}
	sessions, err := selectSessions(program, *all, fs.Args())
	if err != nil {
		return err
	}

	// Photos are cached for far longer than the program: a speaker's portrait
	// does not change between runs, and each one is a separate CDN fetch.
	var images *cache.Cache
	if !*noPhotos {
		images = cache.New(30 * 24 * time.Hour)
		images.Disabled = common.noCache
	}
	renderer, err := render.New(th, images)
	if err != nil {
		return err
	}
	renderer.StripEmoji = *stripEmoji

	if err := os.MkdirAll(*out, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", *out, err)
	}

	var withEmoji, truncated []string
	for _, s := range sessions {
		for _, size := range wanted {
			res, err := renderer.Card(program.Conference, s, size)
			if err != nil {
				return fmt.Errorf("rendering %q: %w", s.Talk.Title, err)
			}
			name := s.FileStem()
			if len(wanted) > 1 {
				name += "-" + size
			}
			path := filepath.Join(*out, name+".svg")
			if err := os.WriteFile(path, []byte(res.SVG), 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", path, err)
			}
			if res.EmojiFallback {
				withEmoji = append(withEmoji, s.Talk.Title)
			}
			if len(res.Overflow) > 0 {
				truncated = append(truncated, fmt.Sprintf("%s (%s)", s.Talk.Title, strings.Join(res.Overflow, ", ")))
			}
			fmt.Printf("%s  %s\n", path, humanBytes(len(res.SVG)))
		}
	}
	fmt.Printf("\n%d cards written to %s\n", len(sessions)*len(wanted), *out)

	// Both of these are visible in the output rather than silent because they
	// are things the user would otherwise only notice after posting.
	if len(withEmoji) > 0 && !*stripEmoji {
		fmt.Fprintf(os.Stderr, "\nnote: %d card(s) contain emoji, which render in browsers but not in\n"+
			"Inkscape or librsvg — they leave a gap there instead. Re-run with --strip-emoji\n"+
			"to remove them:\n", len(dedupe(withEmoji)))
		for _, t := range dedupe(withEmoji) {
			fmt.Fprintf(os.Stderr, "  %s\n", t)
		}
	}
	if len(truncated) > 0 {
		fmt.Fprintf(os.Stderr, "\nwarning: text was truncated to fit on %d card(s); "+
			"raise maxLines or lower minSize in a --theme override:\n", len(dedupe(truncated)))
		for _, t := range dedupe(truncated) {
			fmt.Fprintf(os.Stderr, "  %s\n", t)
		}
	}
	return nil
}

// resolveSizes expands the --size flag against the theme's defined sizes.
func resolveSizes(th *theme.Theme, spec string) ([]string, error) {
	if spec == "both" || spec == "all" {
		return th.Sizes(), nil
	}
	var out []string
	for _, name := range strings.Split(spec, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, err := th.Size(name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no card size selected")
	}
	return out, nil
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// dedupe removes repeats while preserving order; a talk rendered at two sizes
// would otherwise be reported twice.
func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
