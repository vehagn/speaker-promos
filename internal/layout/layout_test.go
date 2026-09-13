package layout

import (
	"strings"
	"sync"
	"testing"

	"github.com/vehagn/speaker-promos/internal/theme"
)

var loadSet = sync.OnceValues(func() (*FontSet, error) {
	th, err := theme.Default()
	if err != nil {
		return nil, err
	}
	return NewFontSet(th)
})

func heading(t *testing.T) *Font {
	t.Helper()
	fs, err := loadSet()
	if err != nil {
		t.Fatal(err)
	}
	f, err := fs.Face("heading")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestMeasureScalesWithSize(t *testing.T) {
	f := heading(t)
	a := f.Measure("Kubernetes", 40, 0)
	b := f.Measure("Kubernetes", 80, 0)
	if a <= 0 {
		t.Fatalf("width at 40px = %v", a)
	}
	// Advances are a pure fraction of the em, so doubling the size must double
	// the width.
	if ratio := b / a; ratio < 1.99 || ratio > 2.01 {
		t.Errorf("width ratio 80px/40px = %v, want 2", ratio)
	}
	// A sanity bound: proportional text averages well under an em per glyph.
	if perGlyph := a / 10 / 40; perGlyph < 0.3 || perGlyph > 0.9 {
		t.Errorf("mean advance = %v em, implausible", perGlyph)
	}
}

func TestMeasureIsProportionalNotMonospace(t *testing.T) {
	f := heading(t)
	// The whole point of using real metrics: "iiii" must not measure the same
	// as "WWWW".
	narrow := f.Measure("iiii", 60, 0)
	wide := f.Measure("WWWW", 60, 0)
	if wide <= narrow*1.5 {
		t.Errorf("iiii=%v WWWW=%v — metrics look monospaced", narrow, wide)
	}
}

func TestMeasureCountsTracking(t *testing.T) {
	f := heading(t)
	plain := f.Measure("SPEAKER", 30, 0)
	tracked := f.Measure("SPEAKER", 30, 0.2)
	// 7 glyphs x 0.2 x 30px = 42px of added space, trailing one included.
	if diff := tracked - plain; diff < 41 || diff > 43 {
		t.Errorf("tracking added %v px, want ~42", diff)
	}
}

func TestMeasureHandlesNorwegianLetters(t *testing.T) {
	f := heading(t)
	for _, s := range []string{"Håvard", "Øystein", "Æsj", "på", "gjøre"} {
		if w := f.Measure(s, 40, 0); w <= 0 {
			t.Errorf("Measure(%q) = %v", s, w)
		}
		for _, r := range s {
			if !f.Has(r) {
				t.Errorf("font lacks %q from %q", r, s)
			}
		}
	}
}

func TestWrapRespectsMaxWidth(t *testing.T) {
	f := heading(t)
	text := "Selvberget Kubernetes for Preppers and Other Curious People"
	const maxWidth = 500
	lines := Wrap(f, text, 48, 0, maxWidth)

	if len(lines) < 2 {
		t.Fatalf("expected several lines, got %v", lines)
	}
	for _, l := range lines {
		if w := f.Measure(l, 48, 0); w > maxWidth {
			t.Errorf("line %q is %v px, over the %v limit", l, w, maxWidth)
		}
	}
	// No words may be lost or duplicated.
	if got, want := strings.Join(strings.Fields(strings.Join(lines, " ")), " "), text; got != want {
		t.Errorf("wrap changed the text:\n got %q\nwant %q", got, want)
	}
}

func TestWrapBreaksAnOverlongWord(t *testing.T) {
	f := heading(t)
	// A single token wider than the line must be split, not allowed to overflow.
	word := "Kubernetesoperatørimplementasjonsdetaljer"
	lines := Wrap(f, word, 60, 0, 300)
	if len(lines) < 2 {
		t.Fatalf("long word was not broken: %v", lines)
	}
	for _, l := range lines {
		if w := f.Measure(l, 60, 0); w > 300 {
			t.Errorf("piece %q is %v px, over 300", l, w)
		}
	}
	if joined := strings.Join(lines, ""); joined != word {
		t.Errorf("broken word lost characters: %q", joined)
	}
}

func TestWrapHonoursHardNewlines(t *testing.T) {
	f := heading(t)
	lines := Wrap(f, "first\nsecond", 40, 0, 10_000)
	if len(lines) != 2 || lines[0] != "first" || lines[1] != "second" {
		t.Errorf("hard newline not honoured: %v", lines)
	}
}

func TestFitPicksLargestSizeThatFits(t *testing.T) {
	f := heading(t)
	short, long := "Talos", "Simplifying Sovereignty: When the Registry Is Gone — Lessons from Lysaker Works"

	s := Fit(f, short, 46, 82, 0, 1.1, 900, 3)
	if s.Size != 82 {
		t.Errorf("short title size = %v, want the maximum 82", s.Size)
	}
	if s.Overflow {
		t.Error("short title reported overflow")
	}

	l := Fit(f, long, 46, 82, 0, 1.1, 900, 3)
	if l.Size >= 82 {
		t.Errorf("long title size = %v, want it scaled down", l.Size)
	}
	if l.Size < 46 {
		t.Errorf("long title size = %v, below the minimum", l.Size)
	}
	if len(l.Lines) > 3 {
		t.Errorf("long title used %d lines, max 3", len(l.Lines))
	}
	for _, line := range l.Lines {
		if w := f.Measure(line, l.Size, 0); w > 900 {
			t.Errorf("line %q is %v px wide", line, w)
		}
	}
	if l.LineHeight != l.Size*1.1 {
		t.Errorf("LineHeight = %v, want size x 1.1", l.LineHeight)
	}
	if h := l.Height(); h != float64(len(l.Lines))*l.LineHeight {
		t.Errorf("Height = %v", h)
	}
}

// When even the smallest size needs too many lines, the text is truncated with
// an ellipsis and flagged — a clipped or overlapping block looks broken, a
// shortened one still reads.
func TestFitTruncatesWhenImpossible(t *testing.T) {
	f := heading(t)
	text := strings.Repeat("altfor lang tekst som aldri kommer til å passe ", 20)

	b := Fit(f, text, 40, 60, 0, 1.1, 400, 2)
	if !b.Overflow {
		t.Error("want Overflow set")
	}
	if len(b.Lines) != 2 {
		t.Fatalf("lines = %d, want exactly the 2 allowed", len(b.Lines))
	}
	last := b.Lines[1]
	if !strings.HasSuffix(last, "…") {
		t.Errorf("last line %q should end with an ellipsis", last)
	}
	if w := f.Measure(last, b.Size, 0); w > 400 {
		t.Errorf("ellipsised line is %v px, over 400", w)
	}
}

func TestFitSingleSizeDisablesAutofit(t *testing.T) {
	f := heading(t)
	b := Fit(f, "26–27 OCTOBER 2026", 34, 34, 0.08, 1.2, 1200, 1)
	if b.Size != 34 {
		t.Errorf("Size = %v, want the fixed 34", b.Size)
	}
	if len(b.Lines) != 1 {
		t.Errorf("lines = %v", b.Lines)
	}
}

func TestSplitRunsSeparatesEmoji(t *testing.T) {
	f := heading(t)
	// The real 2026 title that motivated run splitting.
	runs := SplitRuns(f, "Kan 🇳🇴 skyen kjøre på en brødrister?")

	if !HasFallback(runs) {
		t.Fatal("flag emoji was not routed to the fallback family")
	}
	// Reassembly must be lossless — a promo must not drop characters.
	var sb strings.Builder
	for _, r := range runs {
		sb.WriteString(r.Text)
	}
	if got, want := sb.String(), "Kan 🇳🇴 skyen kjøre på en brødrister?"; got != want {
		t.Errorf("runs do not reassemble:\n got %q\nwant %q", got, want)
	}

	// The flag's two regional indicators must stay in ONE fallback run, or the
	// renderer cannot compose them into a flag.
	var fallbacks []string
	for _, r := range runs {
		if r.Fallback {
			fallbacks = append(fallbacks, r.Text)
		}
	}
	if len(fallbacks) != 1 {
		t.Fatalf("fallback runs = %q, want the flag as a single run", fallbacks)
	}
	if !strings.Contains(fallbacks[0], "🇳🇴") {
		t.Errorf("fallback run = %q, want the whole flag", fallbacks[0])
	}
	// Norwegian letters are in the font and must NOT be pushed to fallback.
	for _, r := range runs {
		if r.Fallback && strings.ContainsAny(r.Text, "åøæÅØÆ") {
			t.Errorf("Norwegian letters wrongly sent to fallback: %q", r.Text)
		}
	}
}

func TestSplitRunsPlainTextIsOneRun(t *testing.T) {
	f := heading(t)
	runs := SplitRuns(f, "Pods on Mars")
	if len(runs) != 1 || runs[0].Fallback {
		t.Errorf("runs = %+v, want one primary run", runs)
	}
	if HasFallback(runs) {
		t.Error("HasFallback true for plain text")
	}
	if SplitRuns(f, "") != nil {
		t.Error("empty line should produce no runs")
	}
}

func TestSplitRunsKeepsSpacesWithText(t *testing.T) {
	f := heading(t)
	// A space must never become its own run; that would multiply elements and
	// risk inconsistent spacing.
	runs := SplitRuns(f, "a b c")
	if len(runs) != 1 {
		t.Errorf("runs = %+v, want one", runs)
	}
}

func TestSplitRunsZWJSequenceStaysTogether(t *testing.T) {
	f := heading(t)
	// A ZWJ emoji sequence is several code points; splitting it would break the
	// composed glyph.
	runs := SplitRuns(f, "hei 👩‍💻 der")
	var fb int
	for _, r := range runs {
		if r.Fallback {
			fb++
			if !strings.Contains(r.Text, "‍") {
				t.Errorf("ZWJ lost from fallback run %q", r.Text)
			}
		}
	}
	if fb != 1 {
		t.Errorf("fallback runs = %d, want 1", fb)
	}
}

func TestFontSetFacesAndRaw(t *testing.T) {
	fs, err := loadSet()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"heading", "body", "light"} {
		if _, err := fs.Face(key); err != nil {
			t.Errorf("Face(%q): %v", key, err)
		}
		if b, ok := fs.Raw(key); !ok || len(b) < 10_000 {
			t.Errorf("Raw(%q) = %d bytes, ok=%v", key, len(b), ok)
		}
	}
	// An empty key defaults to body rather than erroring, so a theme style may
	// omit `face`.
	if _, err := fs.Face(""); err != nil {
		t.Errorf("Face(\"\"): %v", err)
	}
	if _, err := fs.Face("nope"); err == nil {
		t.Error("want error for an undeclared face")
	}
}

func TestFitStyleUsesThemeValues(t *testing.T) {
	fs, err := loadSet()
	if err != nil {
		t.Fatal(err)
	}
	st := theme.TextStyle{Face: "heading", MinSize: 30, MaxSize: 60, MaxLines: 2, LineHeight: 1.2}
	b, f, err := fs.FitStyle(st, "Crossplane in Practice", 600)
	if err != nil {
		t.Fatal(err)
	}
	if f == nil || b.Size < 30 || b.Size > 60 {
		t.Errorf("block = %+v", b)
	}
	if b.LineHeight != b.Size*1.2 {
		t.Errorf("LineHeight = %v", b.LineHeight)
	}

	// A style with no lineHeight must still produce a usable one.
	b2, _, err := fs.FitStyle(theme.TextStyle{Face: "body", MaxSize: 20, MaxLines: 1}, "x", 600)
	if err != nil {
		t.Fatal(err)
	}
	if b2.LineHeight <= 0 {
		t.Errorf("LineHeight = %v, want a default", b2.LineHeight)
	}
}

// Every talk title in the real program must lay out without overflowing at the
// theme's configured sizes; this is the check that the type scale is actually
// viable for the 2026 data rather than just for short examples.
func TestProgramTitlesFitTheDefaultTheme(t *testing.T) {
	// These mirror the anonymised program fixture's title distribution: the
	// longest one it contains, the emoji title, Norwegian letters, a
	// typographic apostrophe and an em dash, and a short title that must NOT be
	// scaled down. Between them they cover every glyph class and both ends of
	// the length range that autofit has to cope with.
	titles := []string{
		"Kan 🇳🇴 skyen kjøre på en brødrister?",
		"Automating Runtimes: for Teams That Sleep at Night — Lessons from Grønnfjell Tech",
		"Simplifying Sovereignty: When the Registry Is Gone — Lessons from Lysaker Works",
		"Nettverket er ikke dødt — det er bare ikke der du la det",
		"Hvorfor ble det så vanskelig",
		"Plattform uten panikk",
		"Drift uten dramatikk, for folk flest",
	}
	fs, err := loadSet()
	if err != nil {
		t.Fatal(err)
	}
	th, _ := theme.Default()
	for _, size := range []string{"portrait", "landscape"} {
		g, err := th.Size(size)
		if err != nil {
			t.Fatal(err)
		}
		st := g.Text["talk"]
		maxWidth := float64(g.Width - 2*g.Pad)
		for _, title := range titles {
			b, f, err := fs.FitStyle(st, title, maxWidth)
			if err != nil {
				t.Fatal(err)
			}
			if b.Overflow {
				t.Errorf("%s: %q overflows even at %vpx", size, title, st.MinSize)
			}
			for _, line := range b.Lines {
				if w := f.Measure(line, b.Size, b.Tracking); w > maxWidth+0.5 {
					t.Errorf("%s: line %q is %v px, over %v", size, line, w, maxWidth)
				}
			}
		}
	}
}
