package lang

import (
	"testing"
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
