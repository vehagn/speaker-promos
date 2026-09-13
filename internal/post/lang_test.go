package post

import (
	"strings"
	"testing"

	"github.com/vehagn/speaker-promos/internal/cnd"
)

func TestParseLanguage(t *testing.T) {
	for in, want := range map[string]Language{
		"":          Auto,
		"auto":      Auto,
		"en":        English,
		"English":   English,
		"no":        Norwegian,
		"nb":        Norwegian,
		"norsk":     Norwegian,
		"Norwegian": Norwegian,
		"bokmål":    Norwegian,
	} {
		got, err := ParseLanguage(in)
		if err != nil {
			t.Errorf("ParseLanguage(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseLanguage(%q) = %q, want %q", in, got, want)
		}
	}
	// A typo must be an error, not a silent fall back to English in every post.
	for _, bad := range []string{"swedish", "sv", "klingon", "norweigan"} {
		if _, err := ParseLanguage(bad); err == nil {
			t.Errorf("ParseLanguage(%q) should have failed", bad)
		}
	}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name, title, abstract string
		want                  Language
	}{
		{
			"norwegian abstract",
			"Kan skyen kjøre på en brødrister?",
			"Plattformer bygges best når teamet forstår hele stacken. Vi ser på hvordan " +
				"åpen kildekode og tydelige grensesnitt gjør det mulig å kjøre tjenestene selv.",
			Norwegian,
		},
		{
			"english abstract",
			"Scaling Scheduling: the Boring Way",
			"This talk walks through what actually broke, why the obvious fix made it " +
				"worse, and the handful of decisions that turned out to matter.",
			English,
		},
		{
			// The case that motivated detection reading the body rather than
			// the title: a Norwegian title on an English talk.
			"norwegian title, english body",
			"Friheten i Koden: Digital Sovereignty for the Nordic Age",
			"We look at what sovereignty means when the platform you depend on is not " +
				"yours, and what you can do about it with the tools that already exist.",
			English,
		},
		{
			"english title, norwegian body",
			"Platform Engineering in Practice",
			"Vi går gjennom hvordan vi bygde plattformen vår, hva som ikke fungerte, " +
				"og hva vi ville gjort annerledes hvis vi skulle gjøre det på nytt.",
			Norwegian,
		},
		{
			// Nothing to go on but the letters.
			"norwegian letters only",
			"Størrelse og mål",
			"",
			Norwegian,
		},
		{"empty", "", "", English},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Detect(c.title, c.abstract); got != c.want {
				t.Errorf("Detect = %q, want %q", got, c.want)
			}
		})
	}
}

// "men" is a word in both languages, and including it as a Norwegian marker
// scored English prose as Norwegian.
func TestDetectIsNotFooledBySharedWords(t *testing.T) {
	english := "Men and women build platforms. The men in the room agreed that this " +
		"was the way to do it, and that we should have done it sooner."
	if got := Detect("Platform work", english); got != English {
		t.Errorf("Detect = %q, want English for %q", got, english)
	}
}

func TestResolveHonoursAnExplicitLanguage(t *testing.T) {
	norwegian := "Vi ser på hvordan det ikke fungerte og hva vi gjorde med det."
	if got := English.Resolve("x", norwegian); got != English {
		t.Errorf("an explicit English must not be overridden by detection: %q", got)
	}
	if got := Norwegian.Resolve("x", "This is plainly English text with the and with."); got != Norwegian {
		t.Errorf("an explicit Norwegian must not be overridden: %q", got)
	}
	if got := Auto.Resolve("x", norwegian); got != Norwegian {
		t.Errorf("Auto should detect: %q", got)
	}
}

func norwegianInput() Input {
	return Input{
		Conference: cnd.Conference{
			Title: "Cloud Native Days Norway 2026", StartDate: "2026-10-26",
			EndDate: "2026-10-27", Domain: "2026.cloudnativedays.no",
		},
		Session: cnd.Session{
			Date: "2026-10-26", Day: 1, StartTime: "09:00", EndTime: "11:00",
			Track: "Track 1: Full Day Workshops",
			Talk: cnd.Talk{
				Title:    "Kan skyen kjøre på en brødrister?",
				Abstract: "Vi ser på hvordan det ikke fungerte og hva vi gjorde med det.",
				Format:   "workshop_120",
			},
		},
		Speakers: []Speaker{{
			Speaker: cnd.Speaker{Name: "Dario Haaland", Slug: "dario-haaland"},
			Role:    Role{Employer: "Bysten Labs"},
		}},
	}
}

// The whole point: no English scaffolding around a Norwegian abstract.
func TestNorwegianDraftHasNoEnglishScaffolding(t *testing.T) {
	in := norwegianInput()
	for _, d := range []Draft{LinkedIn(in), Bluesky(in)} {
		text := d.Text
		for _, want := range []string{
			"holder workshop på Cloud Native Days Norway 2026",
			"«Kan skyen kjøre på en brødrister?»", // angle quotes, per Norwegian typography
			"mandag 26. oktober",                  // localised weekday and month, with the ordinal point
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s draft missing %q:\n%s", d.Platform, want, text)
			}
		}
		for _, forbidden := range []string{
			"is speaking", "are speaking", "is running", "are running",
			" at Cloud Native", "Monday", "October", "“", "”",
		} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s draft contains English %q:\n%s", d.Platform, forbidden, text)
			}
		}
	}
}

func TestNorwegianJoinsNamesWithOg(t *testing.T) {
	in := norwegianInput()
	in.Speakers = append(in.Speakers, Speaker{
		Speaker: cnd.Speaker{Name: "Solveig Ulriksen"}, Role: Role{Employer: "Skyvakt"},
	})
	got := hook(in)
	if !strings.Contains(got, "og Solveig Ulriksen") {
		t.Errorf("hook = %q, want the names joined with \"og\"", got)
	}
	// Norwegian has no verb-number agreement, so the verb does not change.
	if !strings.Contains(got, "holder workshop") {
		t.Errorf("hook = %q", got)
	}
}

func TestEnglishIsUnchanged(t *testing.T) {
	in := norwegianInput()
	in.Language = English
	got := hook(in)
	if !strings.Contains(got, "is running a workshop at Cloud Native Days Norway 2026") {
		t.Errorf("hook = %q", got)
	}
	if !strings.Contains(quoteTitle("Pods on Mars", English), "“Pods on Mars”") {
		t.Error("English should keep curly quotes")
	}
}

func TestNorwegianTalkDraftStaysInsideTheBlueskyLimit(t *testing.T) {
	in := norwegianInput()
	in.Session.Talk.Title = strings.Repeat("Veldig lang tittel om plattformer ", 5)
	in.Session.Talk.Abstract = strings.Repeat("Vi ser på hvordan det ikke fungerte. ", 30)
	d := Bluesky(in)
	if d.Runes() > BlueskyLimit {
		t.Errorf("draft is %d runes:\n%s", d.Runes(), d.Text)
	}
}

// A title that already carries quotation marks is not double-quoted, in either
// language's style.
func TestQuoteTitleRespectsExistingQuotes(t *testing.T) {
	for _, title := range []string{"«Allerede sitert»", "“Already quoted”", `"Quoted"`} {
		if got := quoteTitle(title, Norwegian); got != title {
			t.Errorf("quoteTitle(%q) = %q", title, got)
		}
	}
}

// An unrecognised Language value falls back to DETECTION rather than to a
// fixed language, so a garbage value still produces coherent copy rather than
// an English frame around a Norwegian abstract — which is the bug this whole
// feature exists to fix.
func TestUnknownLanguageFallsBackToDetection(t *testing.T) {
	in := norwegianInput()
	in.Language = Language("sv")
	if got := hook(in); !strings.Contains(got, "holder workshop") {
		t.Errorf("hook = %q, want the detected Norwegian", got)
	}

	// And the phrase table itself still answers for an unknown value, so
	// nothing can render blank.
	w := Language("sv").words()
	if w.at == "" || w.and == "" || len(w.months) != 12 {
		t.Errorf("words() for an unknown language is incomplete: %+v", w)
	}
}
