package main

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/vehagn/speaker-promos/internal/cache"
	"github.com/vehagn/speaker-promos/internal/cnd"
	"github.com/vehagn/speaker-promos/internal/theme"
	"github.com/vehagn/speaker-promos/internal/web"
)

func cmdServe(args []string) error {
	fs := newFlagSet("serve")
	var common commonFlags
	common.register(fs)
	var manifestPath manifestFlag
	manifestPath.register(fs)
	addr := fs.String("addr", "localhost:8787", "address to listen on")
	size := fs.String("size", "portrait", "card size to preview")
	themePath := fs.String("theme", "", "theme YAML to merge over the built-in theme")
	out := fs.String("out", "out", "directory the Export button writes into")
	noPhotos := fs.Bool("no-photos", false, "skip speaker photos (renders initials instead)")
	noLinks := fs.Bool("no-links", false, "skip fetching speaker pages for social handles")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	th, err := theme.Load(*themePath)
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

	var images *cache.Cache
	if !*noPhotos {
		images = cache.New(30 * 24 * time.Hour)
		images.Disabled = common.noCache
	}

	server, err := web.New(web.Options{
		Program: program,
		Set:     set,
		Theme:   th,
		Images:  images,
		Loader:  loader,
		Size:    *size,
		OutDir:  *out,
		NoLinks: *noLinks,
	})
	if err != nil {
		return err
	}

	// Bind before announcing, so the printed URL is never a lie and the
	// "address in use" case fails here rather than after the banner.
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", *addr, err)
	}

	// Fetching social handles is the slow part — 49 speaker pages at ~3 MB —
	// so it happens here with a progress line rather than inside the first page
	// load, where it looked like a hung browser.
	if !*noLinks {
		fmt.Print("fetching speaker profiles… ")
		server.Warm(6, func(done, total int) {
			if done == total {
				fmt.Printf("%d/%d\n", done, total)
			}
		})
	}
	fmt.Printf("%s — %d talks\n", program.Conference.Title, len(program.Sessions))
	fmt.Printf("overrides: %s\n", set.Path())
	fmt.Printf("\n  http://%s\n\n", ln.Addr())
	fmt.Println("Edits save immediately. Ctrl-C to stop.")

	srv := &http.Server{
		Handler: server.Handler(),
		// Cards are ~700 KB each and a page asks for many at once, so the
		// write timeout has to be generous even on localhost.
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      2 * time.Minute,
	}
	return srv.Serve(ln)
}
