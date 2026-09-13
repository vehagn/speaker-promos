// Package web serves a local preview of every promo card beside its draft
// social copy, with the guessed fields editable in place.
//
// It exists because the CLI's review loop is bad: checking 36 cards means
// opening 36 files, the copy is in a terminal next to the card it belongs to
// only by coincidence, and the corrections the tool asks for have to be typed
// into YAML by hand with a re-run to see whether they helped. Here an edit
// re-renders the one card it affects and writes the manifest immediately.
package web

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vehagn/speaker-promos/internal/cache"
	"github.com/vehagn/speaker-promos/internal/cnd"
	"github.com/vehagn/speaker-promos/internal/export"
	"github.com/vehagn/speaker-promos/internal/manifest"
	"github.com/vehagn/speaker-promos/internal/post"
	"github.com/vehagn/speaker-promos/internal/raster"
	"github.com/vehagn/speaker-promos/internal/render"
	"github.com/vehagn/speaker-promos/internal/theme"
)

//go:embed templates/*.html static/*
var files embed.FS

// Options configures a Server.
type Options struct {
	Program  *cnd.Program
	Set      *manifest.Set
	Theme    *theme.Theme
	Images   *cache.Cache
	Loader   *cnd.Loader
	Size     string
	OutDir   string
	NoLinks  bool
	NoPhotos bool
	// Formats is what the Export button writes per talk. Empty means all of
	// svg, png and jpg.
	Formats []string
	// RasterWidth and JPEGQuality mirror the export flags; zero means default.
	RasterWidth int
	JPEGQuality int
	// Language is the default copy language; post.Auto detects it per talk.
	Language post.Language
}

// Server renders the preview site.
type Server struct {
	opts     Options
	renderer *render.Renderer
	tmpl     *template.Template

	converter    raster.Converter
	hasConverter bool

	// mu guards the manifest and everything derived from it. HTMX requests
	// interleave freely — a browser will happily have two edits in flight — and
	// every edit both mutates the set and rewrites the file.
	mu sync.Mutex
	// rev increments on every edit and is embedded in card URLs so the browser
	// refetches exactly the cards that changed.
	rev int64
	// linksMu guards links only. It is deliberately separate from mu: filling
	// this cache does network I/O, and doing that under the manifest lock
	// would block every other request for as long as the fetch takes.
	linksMu sync.Mutex
	// links caches scraped social handles for the process lifetime. Each
	// speaker page is ~3 MB; re-fetching on every re-render would make editing
	// unusable even against the on-disk cache.
	links map[string]cnd.Links
}

// New builds a Server.
func New(opts Options) (*Server, error) {
	if opts.Size == "" {
		opts.Size = "portrait"
	}
	if _, err := opts.Theme.Size(opts.Size); err != nil {
		return nil, err
	}
	renderer, err := render.New(opts.Theme, opts.Images)
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New("").Funcs(funcs).ParseFS(files, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}
	if len(opts.Formats) == 0 {
		opts.Formats = export.AllFormats
	}
	conv, _, hasConv := raster.Find()
	return &Server{
		opts:         opts,
		renderer:     renderer,
		tmpl:         tmpl,
		converter:    conv,
		hasConverter: hasConv,
		rev:          time.Now().Unix(),
		links:        map[string]cnd.Links{},
	}, nil
}

var funcs = template.FuncMap{
	"shortTrack": func(t string) string {
		if _, rest, ok := strings.Cut(t, ": "); ok {
			return rest
		}
		return t
	},
}

// Handler returns the mux serving the site.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /card/{id}", s.handleCard)
	mux.HandleFunc("GET /talk/{id}", s.handleTalkFragment)
	mux.HandleFunc("POST /talk/{id}", s.handleTalkUpdate)
	mux.HandleFunc("POST /speaker/{slug}", s.handleSpeakerUpdate)
	mux.HandleFunc("GET /download/{id}", s.handleDownload)
	mux.HandleFunc("POST /export", s.handleExport)
	mux.HandleFunc("POST /import", s.handleImport)
	mux.Handle("GET /static/", http.FileServerFS(files))
	return mux
}

// session finds a session by talk id.
func (s *Server) session(id string) (cnd.Session, bool) {
	for _, sess := range s.opts.Program.Sessions {
		if sess.Talk.ID == id {
			return sess, true
		}
	}
	return cnd.Session{}, false
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	size := s.sizeParam(r)

	s.mu.Lock()
	views := make([]talkView, 0, len(s.opts.Program.Sessions))
	for _, sess := range s.opts.Program.Sessions {
		views = append(views, s.buildViewLocked(sess, size))
	}
	rev := s.rev
	setPath := s.opts.Set.Path()
	s.mu.Unlock()

	data := struct {
		Conference cnd.Conference
		Talks      []talkView
		Size       string
		Sizes      []string
		Rev        int64
		Manifest   string
		OutDir     string
	}{
		Conference: s.opts.Program.Conference,
		Talks:      views,
		Size:       size,
		Sizes:      s.opts.Theme.Sizes(),
		Rev:        rev,
		Manifest:   setPath,
		OutDir:     s.opts.OutDir,
	}
	s.renderTemplate(w, "index.html", data)
}

func (s *Server) handleTalkFragment(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	view := s.buildViewLocked(sess, s.sizeParam(r))
	s.mu.Unlock()
	s.renderTemplate(w, "talk.html", view)
}

func (s *Server) handleTalkUpdate(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.session(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	spec := manifest.TalkSpec{
		DisplayTitle: strings.TrimSpace(r.FormValue("displayTitle")),
		Hidden:       r.FormValue("hidden") != "",
		Language:     strings.TrimSpace(r.FormValue("language")),
	}
	if _, err := post.ParseLanguage(spec.Language); err != nil {
		s.fail(w, err)
		return
	}

	s.mu.Lock()
	err := s.opts.Set.SetTalk(sess.Talk.ID, spec)
	if err == nil {
		s.rev++
	}
	view := s.buildViewLocked(sess, s.sizeParam(r))
	s.mu.Unlock()

	if err != nil {
		s.fail(w, err)
		return
	}
	s.renderTemplate(w, "talk.html", view)
}

func (s *Server) handleSpeakerUpdate(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	// The row to swap back is passed explicitly: a speaker can appear on
	// several talks, and the one being edited is the one whose form was
	// submitted.
	sess, ok := s.session(r.FormValue("talk"))
	if !ok {
		http.NotFound(w, r)
		return
	}

	spec := manifest.SpeakerSpec{
		Name:     strings.TrimSpace(r.FormValue("name")),
		Employer: strings.TrimSpace(r.FormValue("employer")),
		Job:      strings.TrimSpace(r.FormValue("job")),
		Title:    strings.TrimSpace(r.FormValue("title")),
		Image:    strings.TrimSpace(r.FormValue("image")),
		Links: manifest.Links{
			LinkedIn: strings.TrimSpace(r.FormValue("linkedin")),
			Bluesky:  strings.TrimPrefix(strings.TrimSpace(r.FormValue("bluesky")), "@"),
			X:        strings.TrimPrefix(strings.TrimSpace(r.FormValue("x")), "@"),
		},
	}

	s.mu.Lock()
	err := s.opts.Set.SetSpeaker(slug, spec)
	if err == nil {
		s.rev++
	}
	view := s.buildViewLocked(sess, s.sizeParam(r))
	s.mu.Unlock()

	if err != nil {
		s.fail(w, err)
		return
	}
	s.renderTemplate(w, "talk.html", view)
}

func (s *Server) handleCard(w http.ResponseWriter, r *http.Request) {
	svg, _, ok := s.card(r.PathValue("id"), s.sizeParam(r))
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	// Cards are addressed with a revision, so a given URL's bytes never change
	// and may be cached hard. An edit bumps the revision and therefore the URL.
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Write([]byte(svg))
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	size := s.sizeParam(r)
	svg, sess, ok := s.card(id, size)
	if !ok {
		http.NotFound(w, r)
		return
	}
	name := sess.FileStem() + ".svg"
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Write([]byte(svg))
}

// card renders one talk's SVG with overrides applied.
func (s *Server) card(id, size string) (string, cnd.Session, bool) {
	sess, ok := s.session(id)
	if !ok {
		return "", cnd.Session{}, false
	}
	s.mu.Lock()
	rewritten := s.opts.Set.Rewrite(sess)
	s.mu.Unlock()

	res, err := s.renderer.Card(s.opts.Program.Conference, rewritten, size)
	if err != nil {
		return "", cnd.Session{}, false
	}
	return res.SVG, rewritten, true
}

// sizeParam resolves the requested card size, falling back to the configured
// default rather than erroring on a stale bookmark.
func (s *Server) sizeParam(r *http.Request) string {
	if v := r.FormValue("size"); v != "" {
		if _, err := s.opts.Theme.Size(v); err == nil {
			return v
		}
	}
	return s.opts.Size
}

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data any) {
	var buf strings.Builder
	if err := s.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		// Buffering first means a template failure produces an error page
		// rather than half a page followed by an error.
		s.fail(w, fmt.Errorf("rendering %s: %w", name, err))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(buf.String()))
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, `<p class="error">%s</p>`, template.HTMLEscapeString(err.Error()))
}

func itoa(i int64) string { return strconv.FormatInt(i, 10) }

// speakerLinks returns a speaker's scraped handles, fetching once per process.
func (s *Server) speakerLinks(sp cnd.Speaker) cnd.Links {
	if s.opts.NoLinks || sp.Slug == "" {
		return cnd.Links{}
	}
	s.linksMu.Lock()
	l, ok := s.links[sp.Slug]
	s.linksMu.Unlock()
	if ok {
		return l
	}

	// A failure here is not worth surfacing: handles only suggest mentions, and
	// the edit form lets them be filled in by hand.
	l, _ = s.opts.Loader.SpeakerLinks(sp)

	s.linksMu.Lock()
	s.links[sp.Slug] = l
	s.linksMu.Unlock()
	return l
}

// Warm pre-fetches every speaker's social links.
//
// Without this the first page load fetches 49 speaker pages of ~3 MB each,
// serially, before rendering anything — about twenty seconds of apparently
// hung browser. Doing it up front makes the cost visible and bounded, and the
// on-disk cache means it only happens once per machine. Progress goes to
// `progress` so the caller can print it.
func (s *Server) Warm(parallel int, progress func(done, total int)) {
	if s.opts.NoLinks {
		return
	}
	speakers := s.opts.Program.Speakers()
	if parallel < 1 {
		parallel = 1
	}

	var wg sync.WaitGroup
	// A small semaphore rather than one goroutine per speaker: these are
	// requests to someone else's website, not a benchmark.
	sem := make(chan struct{}, parallel)
	var mu sync.Mutex
	done := 0

	for _, sp := range speakers {
		wg.Add(1)
		go func(sp cnd.Speaker) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			s.speakerLinks(sp)

			mu.Lock()
			done++
			n := done
			mu.Unlock()
			if progress != nil {
				progress(n, len(speakers))
			}
		}(sp)
	}
	wg.Wait()
}

// buildViewLocked assembles a talk's view. Callers must hold s.mu.
func (s *Server) buildViewLocked(sess cnd.Session, size string) talkView {
	set := s.opts.Set
	rewritten := set.Rewrite(sess)
	overrides := set.Overrides()

	view := talkView{
		Session: rewritten,
		Size:    size,
		CardURL: fmt.Sprintf("/card/%s?size=%s&rev=%s", sess.Talk.ID, size, itoa(s.rev)),
		Hidden:  set.Hidden(sess.Talk.ID),
	}
	view.Talk, _ = set.Talk(sess.Talk.ID)

	lang := set.LanguageFor(sess.Talk.ID)
	if lang == post.Auto {
		lang = s.opts.Language
	}
	in := post.Input{Conference: s.opts.Program.Conference, Session: rewritten, Language: lang}
	for i, sp := range sess.Talk.Speakers {
		links := overrides.LinksFor(sp, s.speakerLinks(sp))
		role := overrides.RoleFor(sp)
		override, _ := set.Speaker(sp.Slug)

		// Two versions of the same speaker, used for different things.
		//
		// The FORM shows the original: its placeholders are what the CMS says,
		// which is what an override is being compared against.
		//
		// Everything that describes the OUTPUT — the photo check and the draft
		// copy — uses the rewritten speaker, because that is who the card and
		// the post are about. Building the copy from the original left a
		// corrected name on the card but not in the draft beside it, which is
		// exactly the drift this view model exists to prevent. Rewrite
		// preserves order and length, so the indices line up.
		rendered := sp
		if i < len(rewritten.Talk.Speakers) {
			rendered = rewritten.Talk.Speakers[i]
		}

		view.Speakers = append(view.Speakers, speakerView{
			Speaker:  sp,
			Role:     role,
			Links:    links,
			Override: override,
			Guessed:  role.Guessed,
			HasPhoto: s.renderer.HasPhoto(rendered),
		})
		in.Speakers = append(in.Speakers, post.Speaker{Speaker: rendered, Role: role, Links: links})
	}

	view.Language = in.Language.Resolve(rewritten.Talk.Title, rewritten.Talk.Abstract)
	view.Detected = post.Detect(rewritten.Talk.Title, rewritten.Talk.Abstract)

	view.Drafts = []draftView{
		{Draft: post.LinkedIn(in)},
		{Draft: post.Bluesky(in), Limit: post.BlueskyLimit},
	}
	for i := range view.Drafts {
		d := &view.Drafts[i]
		d.Over = d.Limit > 0 && d.Draft.Runes() > d.Limit
	}

	if res, err := s.renderer.Card(s.opts.Program.Conference, rewritten, size); err == nil {
		if res.EmojiFallback {
			view.Warnings = append(view.Warnings,
				"contains emoji: renders in browsers, but Inkscape and librsvg leave a gap")
		}
		for _, el := range res.Overflow {
			view.Warnings = append(view.Warnings, "text truncated to fit: "+el)
		}
	}
	return view
}

// exporter builds the bundle writer for the given card sizes.
//
// It is constructed per request rather than held on the Server because the
// requested size comes from the query string, and because the manifest it reads
// changes with every edit. Callers must hold s.mu.
func (s *Server) exporter(sizes []string) *export.Exporter {
	return &export.Exporter{
		Renderer:     s.renderer,
		Set:          s.opts.Set,
		Program:      s.opts.Program,
		Language:     s.opts.Language,
		Formats:      s.opts.Formats,
		Sizes:        sizes,
		RasterWidth:  s.opts.RasterWidth,
		JPEGQuality:  s.opts.JPEGQuality,
		LinksFor:     s.speakerLinks,
		Converter:    s.converter,
		HasConverter: s.hasConverter,
	}
}
