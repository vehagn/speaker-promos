package web

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vehagn/speaker-promos/internal/cnd"
	"github.com/vehagn/speaker-promos/internal/manifest"
	"github.com/vehagn/speaker-promos/internal/theme"
)

const talkID = "09b41694-27be-495d-abe3-1899bd725ad8"

// testLogo stands in for the ~190 KB conference wordmark; all the renderer
// needs is a viewBox to scale against.
const testLogo = `<svg width="2710" height="747" viewBox="0 0 2710 747" xmlns="http://www.w3.org/2000/svg"><rect width="2710" height="747" fill="#fff"/></svg>`

func testProgram() *cnd.Program {
	return &cnd.Program{
		Conference: cnd.Conference{
			Title:      "Cloud Native Days Norway 2026",
			StartDate:  "2026-10-26",
			EndDate:    "2026-10-27",
			City:       "Bergen",
			Country:    "Norway",
			Domain:     "2026.cloudnativedays.no",
			LogoBright: testLogo,
		},
		Sessions: []cnd.Session{
			{
				Date: "2026-10-26", Day: 1, Track: "Track 1: Full Day Workshops",
				StartTime: "09:00", EndTime: "11:00",
				Talk: cnd.Talk{
					ID: talkID, Title: "Nok nett", Format: "workshop_120", Level: "intermediate",
					Abstract: "Et foredrag om åpen kildekode og norsk suverenitet.",
					Speakers: []cnd.Speaker{{
						ID: "sp-1", Name: "Dario Haaland", Slug: "dario-haaland", Title: "Bysten Labs",
					}},
				},
			},
			{
				Date: "2026-10-27", Day: 2, Track: "Track 2: Platform Engineering",
				StartTime: "13:20", EndTime: "13:45",
				Talk: cnd.Talk{
					ID: "talk-2", Title: "Pods on Mars", Format: "presentation_25",
					Speakers: []cnd.Speaker{{ID: "sp-2", Name: "Someone Else", Slug: "someone-else"}},
				},
			},
		},
	}
}

// Photos and link scraping are disabled: the tests must not touch the network,
// and that also exercises the monogram fallback.
func newTestServer(t *testing.T) (*Server, http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "promos.yaml")

	th, err := theme.Default()
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(Options{
		Program: testProgram(),
		Set:     manifest.New(manifestPath),
		Theme:   th,
		Size:    "portrait",
		OutDir:  filepath.Join(dir, "out"),
		NoLinks: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv, srv.Handler(), manifestPath
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec
}

func postForm(t *testing.T, h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestIndexListsEveryTalk(t *testing.T) {
	_, h, _ := newTestServer(t)
	rec := get(t, h, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d\n%s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if n := strings.Count(body, `class="row `); n != 2 {
		t.Errorf("rendered %d rows, want 2", n)
	}
	for _, want := range []string{
		"Nok nett", "Pods on Mars", "Dario Haaland",
		"Cloud Native Days Norway 2026", "26–27 October 2026",
		`src="/card/` + talkID, "htmx.min.js",
		// The website's "Track N: " prefix is noise once the row is labelled.
		"Full Day Workshops",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing %q", want)
		}
	}
	if strings.Contains(body, "Track 1:") {
		t.Error("track prefix should be stripped")
	}
}

func TestCardEndpointServesSVG(t *testing.T) {
	_, h, _ := newTestServer(t)
	rec := get(t, h, "/card/"+talkID+"?size=portrait")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/svg+xml") {
		t.Errorf("Content-Type = %q", ct)
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "<svg") || !strings.Contains(body, "@font-face") {
		t.Errorf("body does not look like a self-contained card: %.80q", body)
	}
	// Landscape is a different size, so a different document.
	land := get(t, h, "/card/"+talkID+"?size=landscape")
	if land.Code != http.StatusOK {
		t.Fatalf("landscape status = %d", land.Code)
	}
	if !strings.Contains(land.Body.String(), `width="1200"`) {
		t.Error("landscape card is not 1200 wide")
	}
}

func TestUnknownTalkIs404(t *testing.T) {
	_, h, _ := newTestServer(t)
	for _, path := range []string{"/card/nope", "/talk/nope", "/download/nope"} {
		if rec := get(t, h, path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
	// A POST naming an unknown talk must not create an override for it.
	if rec := postForm(t, h, "/speaker/whoever", url.Values{"talk": {"nope"}}); rec.Code != http.StatusNotFound {
		t.Errorf("POST with unknown talk = %d, want 404", rec.Code)
	}
}

// An invalid size must fall back rather than error: it is usually a stale
// bookmark, and failing the whole page over it would be worse.
func TestInvalidSizeFallsBack(t *testing.T) {
	_, h, _ := newTestServer(t)
	rec := get(t, h, "/card/"+talkID+"?size=square")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `width="1400"`) {
		t.Error("want the configured portrait default")
	}
}

// The whole point of the server: an edit persists and changes both the card and
// the copy next to it.
func TestSpeakerEditPersistsAndRerenders(t *testing.T) {
	_, h, manifestPath := newTestServer(t)

	before := get(t, h, "/card/"+talkID).Body.String()
	if !strings.Contains(before, "Bysten Labs") {
		t.Fatal("expected the upstream title on the card first")
	}

	rec := postForm(t, h, "/speaker/dario-haaland", url.Values{
		"talk":     {talkID},
		"employer": {"Bysten Labs AS"},
		"job":      {"Infrastructure Engineer"},
		"bluesky":  {"@dario.example"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d\n%s", rec.Code, rec.Body)
	}

	// The manifest is written immediately — there is no save button.
	saved, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"kind: SpeakerOverride", "name: dario-haaland",
		"employer: Bysten Labs AS", "job: Infrastructure Engineer", "bluesky: dario.example"} {
		if !strings.Contains(string(saved), want) {
			t.Errorf("manifest missing %q:\n%s", want, saved)
		}
	}
	// A leading @ is stripped so the post does not double it.
	if strings.Contains(string(saved), "bluesky: '@") {
		t.Error("the @ prefix should have been stripped")
	}

	// The returned fragment shows the new values and the new copy.
	frag := rec.Body.String()
	if !strings.Contains(frag, `value="Bysten Labs AS"`) {
		t.Error("fragment does not show the saved employer")
	}
	if !strings.Contains(frag, "@dario.example") {
		t.Error("fragment copy does not mention the new handle")
	}
	// The employer is no longer a guess, so the warning must be gone.
	if strings.Contains(frag, "guessed from") {
		t.Error("fragment still flags the employer as guessed")
	}

	// And the card itself now carries the corrected role line.
	after := get(t, h, "/card/"+talkID).Body.String()
	if !strings.Contains(after, "Infrastructure Engineer at Bysten Labs AS") {
		t.Error("card was not re-rendered with the override")
	}
}

// Card URLs carry a revision so an HTMX swap does not re-insert a src the
// browser will serve from cache, which would make an edit look like a no-op.
func TestCardURLRevisionChangesAfterAnEdit(t *testing.T) {
	_, h, _ := newTestServer(t)
	first := extractCardURL(t, get(t, h, "/talk/"+talkID).Body.String())

	postForm(t, h, "/speaker/dario-haaland", url.Values{
		"talk": {talkID}, "employer": {"New Corp"},
	})
	second := extractCardURL(t, get(t, h, "/talk/"+talkID).Body.String())

	if first == second {
		t.Errorf("card URL unchanged after an edit: %s", first)
	}
}

func extractCardURL(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, `src="/card/`)
	if i < 0 {
		t.Fatal("no card URL in fragment")
	}
	rest := body[i+5:]
	return rest[:strings.IndexByte(rest, '"')]
}

func TestTalkOverrideAndHidden(t *testing.T) {
	_, h, manifestPath := newTestServer(t)

	rec := postForm(t, h, "/talk/"+talkID, url.Values{
		"displayTitle": {"Shorter Title"},
		"hidden":       {"1"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d\n%s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `class="row hidden"`) {
		t.Error("fragment should mark the row as hidden")
	}

	saved, _ := os.ReadFile(manifestPath)
	for _, want := range []string{"kind: TalkOverride", "displayTitle: Shorter Title", "hidden: true"} {
		if !strings.Contains(string(saved), want) {
			t.Errorf("manifest missing %q:\n%s", want, saved)
		}
	}

	// The display title reaches the card.
	if !strings.Contains(get(t, h, "/card/"+talkID).Body.String(), "Shorter Title") {
		t.Error("card does not use the display title")
	}

	// Unchecking clears it again.
	postForm(t, h, "/talk/"+talkID, url.Values{"displayTitle": {"Shorter Title"}})
	saved, _ = os.ReadFile(manifestPath)
	if strings.Contains(string(saved), "hidden: true") {
		t.Errorf("hidden was not cleared:\n%s", saved)
	}
}

func TestExportWritesABundlePerTalk(t *testing.T) {
	srv, h, _ := newTestServer(t)

	rec := postForm(t, h, "/export", url.Values{"size": {"portrait"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d\n%s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "wrote 2 talks") {
		t.Errorf("export said %q", rec.Body.String())
	}

	// One folder per talk, named the same as the CLI names it.
	dirs, _ := filepath.Glob(filepath.Join(srv.opts.OutDir, "*"))
	if len(dirs) != 2 {
		t.Fatalf("wrote %d folders, want 2: %v", len(dirs), dirs)
	}
	var bundle string
	for _, d := range dirs {
		if strings.Contains(filepath.Base(d), "dario-haaland") {
			bundle = d
		}
	}
	if bundle == "" {
		t.Fatalf("no folder named after the speaker: %v", dirs)
	}

	// The card, both drafts and the editable manifest. PNG and JPEG depend on
	// a rasteriser being installed, so they are not required here.
	for _, name := range []string{"portrait.svg", "linkedin.txt", "bluesky.txt", "promo.yaml"} {
		fi, err := os.Stat(filepath.Join(bundle, name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if fi.Size() == 0 {
			t.Errorf("%s is empty", name)
		}
	}

	// The copy file must hold the post body alone, so it pastes verbatim.
	body, err := os.ReadFile(filepath.Join(bundle, "bluesky.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "check before posting") {
		t.Error("bluesky.txt should not carry the review notes")
	}
	if !strings.Contains(string(body), "Dario Haaland") {
		t.Errorf("bluesky.txt = %q", body)
	}

	// The manifest is pre-filled with what the card actually used, so it can be
	// edited and fed back.
	yml, err := os.ReadFile(filepath.Join(bundle, "promo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"kind: SpeakerOverride", "name: dario-haaland", "Bysten Labs"} {
		if !strings.Contains(string(yml), want) {
			t.Errorf("promo.yaml missing %q:\n%s", want, yml)
		}
	}

	// Hiding a talk excludes it, matching `promo export --all`.
	postForm(t, h, "/talk/talk-2", url.Values{"hidden": {"1"}})
	os.RemoveAll(srv.opts.OutDir)
	rec = postForm(t, h, "/export", url.Values{})
	if !strings.Contains(rec.Body.String(), "wrote 1 talks") || !strings.Contains(rec.Body.String(), "1 hidden") {
		t.Errorf("export said %q", rec.Body.String())
	}
}

func TestDownloadUsesTheCLIFilename(t *testing.T) {
	_, h, _ := newTestServer(t)
	rec := get(t, h, "/download/"+talkID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	cd := rec.Header().Get("Content-Disposition")
	// Same stem as `promo export` writes, so a browser download and a CLI export
	// land on the same name.
	if !strings.Contains(cd, "d1-0900-dario-haaland") || !strings.Contains(cd, ".svg") {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

// Talk titles and speaker names come from a CMS, so the templates must escape
// them. html/template does this by default; the test pins that nothing is
// emitted through a raw-HTML path.
func TestTemplatesEscapeUpstreamText(t *testing.T) {
	dir := t.TempDir()
	th, _ := theme.Default()
	program := testProgram()
	program.Sessions[0].Talk.Title = `<script>alert("x")</script> & co`
	program.Sessions[0].Talk.Speakers[0].Name = `<b>bold</b>`

	srv, err := New(Options{
		Program: program, Set: manifest.New(filepath.Join(dir, "m.yaml")),
		Theme: th, Size: "portrait", OutDir: dir, NoLinks: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	body := get(t, srv.Handler(), "/").Body.String()
	if strings.Contains(body, "<script>alert") || strings.Contains(body, "<b>bold</b>") {
		t.Error("upstream markup was not escaped")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("expected the escaped form to be present")
	}
}

func TestStaticAssetsAreServed(t *testing.T) {
	_, h, _ := newTestServer(t)
	rec := get(t, h, "/static/htmx.min.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "htmx") {
		t.Error("does not look like htmx")
	}
}

// A browser will have several edits in flight at once. Run with -race.
func TestConcurrentEditsAreSerialised(t *testing.T) {
	_, h, manifestPath := newTestServer(t)

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			postForm(t, h, "/speaker/dario-haaland", url.Values{
				"talk": {talkID}, "employer": {"Corp"},
			})
			postForm(t, h, "/talk/talk-2", url.Values{"displayTitle": {"T"}})
			get(t, h, "/")
			get(t, h, "/card/"+talkID)
		}(i)
	}
	wg.Wait()

	// The manifest must still be valid and loadable after the pile-up.
	set, err := manifest.Load(manifestPath)
	if err != nil {
		t.Fatalf("manifest is corrupt after concurrent edits: %v", err)
	}
	if sp, ok := set.Speaker("dario-haaland"); !ok || sp.Employer != "Corp" {
		t.Errorf("speaker override = %+v, ok=%v", sp, ok)
	}
}

// Regression: the draft copy was built from the ORIGINAL speakers while the
// card used the rewritten ones, so a corrected name appeared on the card but
// not in the draft sitting beside it.
func TestCorrectedNameReachesBothCardAndCopy(t *testing.T) {
	_, h, manifestPath := newTestServer(t)

	rec := postForm(t, h, "/speaker/dario-haaland", url.Values{
		"talk": {talkID},
		"name": {"Dárió Håaland"},
		// A role line that does not fit "<job> at <employer>" at all.
		"title": {"Maintainer, Co-Chair CNCF TAG Infrastructure"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d\n%s", rec.Code, rec.Body)
	}
	frag := rec.Body.String()

	// The draft copy names the corrected speaker.
	if !strings.Contains(frag, "Dárió Håaland") {
		t.Errorf("fragment does not use the corrected name:\n%s", frag)
	}
	if strings.Contains(svgText(t, get(t, h, "/card/"+talkID).Body.String()), "Dario Haaland") {
		t.Error("the card still shows the uncorrected name")
	}

	// The form keeps the CMS value as its placeholder, since that is what an
	// override is being compared against.
	if !strings.Contains(frag, `placeholder="Dario Haaland"`) {
		t.Error("the form should still show the original name as a placeholder")
	}

	// The card's role line is the verbatim title.
	card := svgText(t, get(t, h, "/card/"+talkID).Body.String())
	if !strings.Contains(card, "Maintainer, Co-Chair CNCF TAG Infrastructure") {
		t.Errorf("card role line = %q", card)
	}

	saved, _ := os.ReadFile(manifestPath)
	for _, want := range []string{"name: Dárió Håaland", "title: Maintainer,"} {
		if !strings.Contains(string(saved), want) {
			t.Errorf("manifest missing %q:\n%s", want, saved)
		}
	}
}

// svgText returns a card's visible text with lines joined by spaces, so an
// assertion does not depend on where autofit happened to wrap. The <style>
// block is skipped so embedded font CSS does not pollute the result.
func svgText(t *testing.T, doc string) string {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(doc))
	var parts []string
	inStyle := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch e := tok.(type) {
		case xml.StartElement:
			if e.Name.Local == "style" {
				inStyle = true
			}
		case xml.EndElement:
			if e.Name.Local == "style" {
				inStyle = false
			}
		case xml.CharData:
			if inStyle {
				continue
			}
			if v := strings.TrimSpace(string(e)); v != "" {
				parts = append(parts, v)
			}
		}
	}
	return strings.Join(parts, " ")
}
