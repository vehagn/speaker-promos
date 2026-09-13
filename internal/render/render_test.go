package render

import (
	"encoding/xml"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vehagn/speaker-promos/internal/cnd"
	"github.com/vehagn/speaker-promos/internal/theme"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// Photos are deliberately left out (Images is nil): the goldens must not depend
// on a CDN being reachable, and the no-photo path exercises the monogram
// fallback at the same time.
var newRenderer = sync.OnceValues(func() (*Renderer, error) {
	th, err := theme.Default()
	if err != nil {
		return nil, err
	}
	return New(th, nil)
})

func renderer(t *testing.T) *Renderer {
	t.Helper()
	r, err := newRenderer()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// testLogo is a stand-in for the conference wordmark. The real one is ~190 KB
// of paths; all the renderer needs from it is a viewBox to scale against.
const testLogo = `<svg width="2710" height="747" viewBox="0 0 2710 747" xmlns="http://www.w3.org/2000/svg"><rect width="2710" height="747" fill="#fff"/></svg>`

func testConference() cnd.Conference {
	return cnd.Conference{
		Title:      "Cloud Native Days Norway 2026",
		StartDate:  "2026-10-26",
		EndDate:    "2026-10-27",
		City:       "Bergen",
		Country:    "Norway",
		Domain:     "2026.cloudnativedays.no",
		LogoBright: testLogo,
	}
}

func testSession() cnd.Session {
	return cnd.Session{
		Date:      "2026-10-26",
		Day:       1,
		Track:     "Track 2: Platform Engineering, SRE & Operations",
		StartTime: "13:20",
		EndTime:   "13:45",
		Talk: cnd.Talk{
			ID:     "584db4de-d0ac-4d3a-9fc7-33d541b6c862",
			Title:  "Pods on Mars: Selvberget Kubernetes",
			Format: "presentation_25",
			Level:  "intermediate",
			Speakers: []cnd.Speaker{
				{ID: "a1", Name: "Sindre Vik", Slug: "sindre-vik", Title: "Platform Engineer at Fjordstack"},
			},
		},
	}
}

func TestGoldenCards(t *testing.T) {
	r := renderer(t)
	for _, size := range []string{"portrait", "landscape"} {
		t.Run(size, func(t *testing.T) {
			res, err := r.Card(testConference(), testSession(), size)
			if err != nil {
				t.Fatal(err)
			}
			// The embedded fonts are ~150 KB of base64 that would dominate the
			// golden file and obscure every real change, so they are elided.
			got := elideFontData(res.SVG)
			compareGolden(t, "card-"+size+".svg", got)
		})
	}
}

// elideFontData replaces base64 font payloads with their length, keeping the
// @font-face structure visible without the bulk.
func elideFontData(svg string) string {
	const marker = `url("data:font/ttf;base64,`
	var b strings.Builder
	rest := svg
	for {
		i := strings.Index(rest, marker)
		if i < 0 {
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(rest[:i+len(marker)])
		rest = rest[i+len(marker):]
		end := strings.Index(rest, `"`)
		if end < 0 {
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString("<")
		b.WriteString(itoa(end))
		b.WriteString(" base64 chars elided>")
		rest = rest[end:]
	}
}

func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run: go test ./internal/render -update)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from the golden file; run "+
			"`go test ./internal/render -update` and review the diff", name)
	}
}

// Every card must be well-formed XML. Talk titles come from a CMS and contain
// ampersands ("Platform Engineering, SRE & Operations"), which unescaped would
// produce a file no renderer will open.
func TestCardsAreWellFormedXML(t *testing.T) {
	r := renderer(t)
	s := testSession()
	s.Talk.Title = `Tom & Jerry's <script>alert("x")</script> "quoted" ampersand & more`
	s.Talk.Speakers[0].Name = `A & B <b>bold</b>`

	for _, size := range []string{"portrait", "landscape"} {
		res, err := r.Card(testConference(), s, size)
		if err != nil {
			t.Fatal(err)
		}
		dec := xml.NewDecoder(strings.NewReader(res.SVG))
		for {
			_, err := dec.Token()
			if err != nil {
				if err.Error() == "EOF" {
					break
				}
				t.Fatalf("%s is not well-formed XML: %v", size, err)
			}
		}
		// The markup in the title must have been escaped, not passed through.
		if strings.Contains(res.SVG, "<script>") || strings.Contains(res.SVG, "<b>bold") {
			t.Errorf("%s: markup from the title was not escaped", size)
		}
		if !strings.Contains(res.SVG, "&amp;") {
			t.Errorf("%s: expected an escaped ampersand", size)
		}
	}
}

// rgba() is CSS Color syntax that SVG presentation attributes do not accept:
// Inkscape and librsvg parse it as BLACK, which once turned the translucent
// talk panel into a solid black box. No card may contain it.
func TestNoCSSColorSyntaxInAttributes(t *testing.T) {
	r := renderer(t)
	for _, size := range []string{"portrait", "landscape"} {
		res, err := r.Card(testConference(), testSession(), size)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{`fill="rgba`, `stroke="rgba`, `fill="rgb(`, `stroke="rgb(`} {
			if strings.Contains(res.SVG, bad) {
				t.Errorf("%s contains %s", size, bad)
			}
		}
	}
}

func TestOnlyUsedFacesAreEmbedded(t *testing.T) {
	r := renderer(t)
	res, err := r.Card(testConference(), testSession(), "portrait")
	if err != nil {
		t.Fatal(err)
	}
	// The default theme declares three weights; the portrait card uses heading
	// (700) and body (500) but never light (400).
	if n := strings.Count(res.SVG, "@font-face"); n != 2 {
		t.Errorf("@font-face rules = %d, want 2", n)
	}
	if strings.Contains(res.SVG, "font-weight: 400;") {
		t.Error("embedded an unused face")
	}
}

func TestEmojiIsReportedAndStrippable(t *testing.T) {
	r := renderer(t)
	s := testSession()
	s.Talk.Title = "Kan 🇳🇴 skyen kjøre på en brødrister?"

	res, err := r.Card(testConference(), s, "portrait")
	if err != nil {
		t.Fatal(err)
	}
	if !res.EmojiFallback {
		t.Error("want EmojiFallback set for a title containing a flag")
	}
	if !strings.Contains(res.SVG, "🇳🇴") {
		t.Error("the emoji should still be present by default")
	}
	if !strings.Contains(res.SVG, r.Theme.EmojiFallback) {
		t.Error("the emoji run should name the fallback family")
	}

	// StripEmoji removes it and stops reporting.
	th, _ := theme.Default()
	stripped, err := New(th, nil)
	if err != nil {
		t.Fatal(err)
	}
	stripped.StripEmoji = true
	res2, err := stripped.Card(testConference(), s, "portrait")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res2.SVG, "🇳🇴") {
		t.Error("--strip-emoji left the emoji in")
	}
	if res2.EmojiFallback {
		t.Error("EmojiFallback should be false once emoji are stripped")
	}
	// Stripping must not eat the surrounding words or leave a double space.
	if !strings.Contains(res2.SVG, "datasenter") {
		t.Error("stripping removed neighbouring text")
	}
	if strings.Contains(svgText(t, res2.SVG), "Et  datasenter") {
		t.Error("stripping left a double space")
	}
}

// A speaker with no photo must still get a recognisable card rather than an
// empty square; five of the 2026 speakers have no image or no title.
func TestMissingPhotoFallsBackToMonogram(t *testing.T) {
	r := renderer(t)
	res, err := r.Card(testConference(), testSession(), "portrait")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.SVG, ">SV<") {
		t.Error("want the speaker's initials as a monogram")
	}
	if strings.Contains(res.SVG, "<image") {
		t.Error("no photo was available, so no <image> should be emitted")
	}
}

func TestMissingTitleOmitsRoleLine(t *testing.T) {
	r := renderer(t)
	s := testSession()
	s.Talk.Speakers[0].Title = ""
	res, err := r.Card(testConference(), s, "portrait")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(svgText(t, res.SVG), "Platform Engineer") {
		t.Error("role line should be gone")
	}
	// The card must still be complete.
	text := svgText(t, res.SVG)
	for _, want := range []string{"Sindre Vik", "Pods on Mars", "2026.cloudnativedays.no"} {
		if !strings.Contains(text, want) {
			t.Errorf("card missing %q; text was %q", want, text)
		}
	}
}

func TestMissingLogoFallsBackToConferenceName(t *testing.T) {
	r := renderer(t)
	conf := testConference()
	conf.LogoBright = ""
	res, err := r.Card(conf, testSession(), "portrait")
	if err != nil {
		t.Fatal(err)
	}
	// A card must never be unattributed.
	if got := svgText(t, res.SVG); !strings.Contains(got, "Cloud Native Days Norway 2026") {
		t.Errorf("want the conference name when there is no logo, got text: %q", got)
	}
}

func TestInitials(t *testing.T) {
	for in, want := range map[string]string{
		"Sindre Vik":                "SV",
		"Dario Haaland":             "DH",
		"solveig":                   "S",
		"":                          "",
		"Øyvind Riise":              "ØR",
		"  spaced   name   here   ": "SN",
	} {
		if got := Initials(in); got != want {
			t.Errorf("Initials(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEscape(t *testing.T) {
	got := escape(`a & b < c > d " e ' f`)
	want := `a &amp; b &lt; c &gt; d &quot; e &apos; f`
	if got != want {
		t.Errorf("escape =\n %q\nwant %q", got, want)
	}
	// Norwegian letters and emoji must pass through untouched.
	if got := escape("Håvard 🇳🇴"); got != "Håvard 🇳🇴" {
		t.Errorf("escape mangled non-ASCII: %q", got)
	}
}

// svgText returns a card's visible text with lines joined by spaces.
//
// Assertions must not depend on where autofit happened to wrap: each line is a
// separate <text> element, so a search for "Cloud Native Days Norway 2026" in
// the raw markup fails the moment the title wraps. The <style> block is skipped
// so embedded font CSS does not pollute the result.
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
			if s := strings.TrimSpace(string(e)); s != "" {
				parts = append(parts, s)
			}
		}
	}
	return strings.Join(parts, " ")
}
