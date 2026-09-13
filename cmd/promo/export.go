package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/vehagn/speaker-promos/internal/cache"
	"github.com/vehagn/speaker-promos/internal/cnd"
	"github.com/vehagn/speaker-promos/internal/export"
	"github.com/vehagn/speaker-promos/internal/lang"
	"github.com/vehagn/speaker-promos/internal/raster"
	"github.com/vehagn/speaker-promos/internal/render"
	"github.com/vehagn/speaker-promos/internal/theme"
)

func cmdExport(args []string) error {
	fs := newFlagSet("export")
	var common commonFlags
	common.register(fs)
	var manifestPath manifestFlag
	manifestPath.register(fs)
	all := fs.Bool("all", false, "export every talk in the program")
	out := fs.String("out", "out", "directory to write the per-talk folders into")
	sizes := fs.String("size", "portrait", "card sizes: portrait, landscape, or both")
	formats := fs.String("formats", "svg,png,jpg", "card formats: any of svg, png, jpg")
	themePath := fs.String("theme", "", "theme YAML to merge over the built-in theme")
	noPhotos := fs.Bool("no-photos", false, "skip speaker photos (renders initials instead)")
	noLinks := fs.Bool("no-links", false, "skip fetching speaker pages for social handles")
	stripEmoji := fs.Bool("strip-emoji", false, "remove emoji rather than relying on a system emoji font")
	width := fs.Int("width", 0, "raster width in pixels (default: the card's own width)")
	quality := fs.Int("jpeg-quality", 88, "JPEG quality, 1-100")
	language := fs.String("language", "auto", "copy language: auto, en or no")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	th, err := theme.Load(*themePath)
	if err != nil {
		return err
	}
	wantSizes, err := resolveSizes(th, *sizes)
	if err != nil {
		return err
	}
	wantFormats, err := export.ParseFormats(*formats)
	if err != nil {
		return err
	}
	copyLang, err := lang.ParseLanguage(*language)
	if err != nil {
		return err
	}

	set, err := manifestPath.load()
	if err != nil {
		return err
	}
	loader := cnd.NewLoader(common.domain, common.ttl)
	loader.Cache.Disabled = common.noCache
	program, err := loader.Load()
	if err != nil {
		return err
	}
	sessions, err := selectSessions(program, set, *all, fs.Args())
	if err != nil {
		return err
	}

	// Photos are cached far longer than the program: a portrait does not change
	// between runs and each one is a separate CDN fetch.
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

	conv, path, hasConv := raster.Find()
	needsRaster := len(wantFormats) > 1 || wantFormats[0] != export.FormatSVG
	if needsRaster && hasConv {
		fmt.Printf("rasterising with %s (%s)\n", conv.Name, path)
		if !conv.EmbedsFonts {
			fmt.Printf("note: %s ignores the cards' embedded fonts — run `promo fonts install` first\n", conv.Name)
		}
	}

	exporter := &export.Exporter{
		Renderer:     renderer,
		Set:          set,
		Program:      program,
		Language:     copyLang,
		Formats:      wantFormats,
		Sizes:        wantSizes,
		RasterWidth:  *width,
		JPEGQuality:  *quality,
		LinksFor:     speakerLinkFetcher(loader, *noLinks),
		Converter:    conv,
		HasConverter: hasConv,
	}

	var warned []string
	files := 0
	for _, s := range sessions {
		res, err := exporter.Write(*out, s)
		if err != nil {
			return err
		}
		files += len(res.Files)
		fmt.Printf("%s/  %s\n", res.Dir, strings.Join(res.Files, " "))
		for _, w := range res.Warnings {
			warned = append(warned, res.Dir+": "+w)
		}
	}

	fmt.Printf("\n%d talks, %d files under %s/\n", len(sessions), files, *out)
	if needsRaster && !hasConv {
		fmt.Fprint(os.Stderr, "\n"+raster.NoConverterMessage)
	}
	if len(warned) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d warning(s):\n", len(warned))
		for _, w := range warned {
			fmt.Fprintln(os.Stderr, "  "+w)
		}
	}
	return nil
}

// speakerLinkFetcher returns a per-speaker link lookup that fetches at most
// once per speaker, since a speaker on two talks would otherwise be fetched
// twice and each page is around 3 MB.
func speakerLinkFetcher(loader *cnd.Loader, skip bool) func(cnd.Speaker) cnd.Links {
	if skip {
		return nil
	}
	seen := map[string]cnd.Links{}
	return func(sp cnd.Speaker) cnd.Links {
		if sp.Slug == "" {
			return cnd.Links{}
		}
		if l, ok := seen[sp.Slug]; ok {
			return l
		}
		// Handles only suggest mentions, so a failure is not worth aborting an
		// export over; the copy simply goes out without them.
		l, _ := loader.SpeakerLinks(sp)
		seen[sp.Slug] = l
		return l
	}
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
