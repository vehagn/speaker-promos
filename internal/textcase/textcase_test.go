package textcase

import (
	"testing"

	"github.com/vehagn/speaker-promos/internal/lang"
)

func TestTitleEnglish(t *testing.T) {
	for in, want := range map[string]string{
		"agentic cloud ops":                   "Agentic Cloud Ops",
		"shift left with reliability testing": "Shift Left with Reliability Testing",
		"the legend of config":                "The Legend of Config",
		"a perspective on platforms":          "A Perspective on Platforms",
		"git outta here: the gitless gitops":  "Git Outta Here: The Gitless GitOps",
		"from pipelines to control planes":    "From Pipelines to Control Planes",
		"platform obesity, not complexity":    "Platform Obesity, Not Complexity",
		"  spaced   out  ":                    "Spaced Out",
		"ALREADY LOUD":                        "ALREADY LOUD",
		"Already Fine":                        "Already Fine",
	} {
		if got := Title(in, lang.English); got != want {
			t.Errorf("Title(%q) = %q, want %q", in, got, want)
		}
	}
}

// A minor word is only lowercase in the middle: a title may legitimately begin
// or end with one.
func TestTitleKeepsMinorWordsAtTheEdges(t *testing.T) {
	for in, want := range map[string]string{
		"the boring way":       "The Boring Way",
		"what we are here for": "What We Are Here For",
		"in and out":           "In and Out",
	} {
		if got := Title(in, lang.English); got != want {
			t.Errorf("Title(%q) = %q, want %q", in, got, want)
		}
	}
}

// The one thing a title fixer must never do is recase a name someone chose.
func TestTitlePreservesAcronymsAndCamelCase(t *testing.T) {
	for _, in := range []string{
		"How OpenTelemetry Trace Context Helps",
		"Verifying Cluster State with Kyverno",
		"Introduction to k6 and YAML",
		"GitOps at Scale with ArgoCD",
		"Running AI Workloads on Kubernetes",
		"Migrating from EC2 to k8s",
	} {
		if got := Title(in, lang.English); got != in {
			t.Errorf("Title(%q) = %q, want it untouched", in, got)
		}
	}
	// And it fixes the lowercase words around them.
	// Around them: lowercase words are fixed, the known acronym is expanded to
	// the spelling this audience uses, and anything with a digit is left alone.
	got := Title("running ai workloads with OpenTelemetry on k8s", lang.English)
	want := "Running AI Workloads with OpenTelemetry on k8s"
	if got != want {
		t.Errorf("Title = %q, want %q", got, want)
	}
}

// Norwegian uses sentence case. Title-casing a Norwegian title is an error, not
// a correction, and a third of the program is Norwegian.
func TestTitleNorwegianUsesSentenceCase(t *testing.T) {
	for in, want := range map[string]string{
		"akkurat nok nett: et datasenter på en laptop": "Akkurat nok nett: Et datasenter på en laptop",
		"praktisk AI-drevet Kubernetes-drift":          "Praktisk AI-drevet Kubernetes-drift",
		"drift uten dramatikk":                         "Drift uten dramatikk",
		"hvorfor ble det så vanskelig?":                "Hvorfor ble det så vanskelig?",
	} {
		if got := Title(in, lang.Norwegian); got != want {
			t.Errorf("Title(%q, no) = %q, want %q", in, got, want)
		}
	}
	// It never lowercases, because a proper noun is indistinguishable from an
	// over-capitalised word without a dictionary.
	if got := Title("Vi Ser På Kubernetes", lang.Norwegian); got != "Vi Ser På Kubernetes" {
		t.Errorf("sentence case lowercased something: %q", got)
	}
}

// Auto detects from the title itself, which is what the button passes.
func TestTitleAutoDetects(t *testing.T) {
	if got := Title("vi ser på hvordan det ikke fungerte og hva vi gjorde", lang.Auto); got !=
		"Vi ser på hvordan det ikke fungerte og hva vi gjorde" {
		t.Errorf("Norwegian not detected: %q", got)
	}
	if got := Title("this is the way we did it and what we learned", lang.Auto); got !=
		"This Is the Way We Did It and What We Learned" {
		t.Errorf("English not detected: %q", got)
	}
}

func TestName(t *testing.T) {
	for in, want := range map[string]string{
		"leffen":            "Leffen",
		"lars":              "Lars",
		"jan ivar beddari":  "Jan Ivar Beddari",
		"Dario Haaland":     "Dario Haaland",
		"ÅSE NORDBØ":        "ÅSE NORDBØ", // already capitalised; left alone
		"øydis kind refsum": "Øydis Kind Refsum",
		"  josvaz  ":        "Josvaz",
		"":                  "",
	} {
		if got := Name(in); got != want {
			t.Errorf("Name(%q) = %q, want %q", in, got, want)
		}
	}
}

// Particles stay lowercase mid-name — capitalising them is a change for the
// worse — but lead a name normally.
func TestNameKeepsParticlesLowercase(t *testing.T) {
	for in, want := range map[string]string{
		"jeroen van erp":    "Jeroen van Erp",
		"andrés de la cruz": "Andrés de la Cruz",
		"ludwig von mises":  "Ludwig von Mises",
		"van morrison":      "Van Morrison", // leading: capitalised
	} {
		if got := Name(in); got != want {
			t.Errorf("Name(%q) = %q, want %q", in, got, want)
		}
	}
}

// A name someone deliberately cased must survive.
func TestNamePreservesChosenCasing(t *testing.T) {
	for _, in := range []string{"McDonald", "O'Brien", "Anne-Marie DuPont", "eBay Team"} {
		if got := Name(in); got != in {
			t.Errorf("Name(%q) = %q, want it untouched", in, got)
		}
	}
}

func TestUpperFirstSkipsLeadingPunctuation(t *testing.T) {
	if got := Title(`"quoted words here"`, lang.English); got != `"Quoted Words Here"` {
		t.Errorf("Title = %q", got)
	}
}

// Norwegian builds compounds constantly, so a known acronym has to be found
// inside one: "ai-drevet" is the exact case from the 2026 program.
func TestExpandsAcronymsInsideCompounds(t *testing.T) {
	for in, want := range map[string]string{
		"praktisk ai-drevet kubernetes-drift": "Praktisk AI-drevet kubernetes-drift",
		"en api-first tilnærming":             "En API-first tilnærming",
		"ingen forkortelser her":              "Ingen forkortelser her",
	} {
		if got := Title(in, lang.Norwegian); got != want {
			t.Errorf("Title(%q, no) = %q, want %q", in, got, want)
		}
	}
	// An already-correct compound is left exactly alone.
	for _, in := range []string{"Praktisk AI-drevet Kubernetes-drift", "En API-First Approach"} {
		if got := Title(in, lang.Norwegian); got != in {
			t.Errorf("Title(%q) = %q, want it untouched", in, got)
		}
	}
	// And in English titles too. Only the first element of a compound is
	// capitalised — "AI-native", "Hands-on" — which is how the program writes
	// its own titles.
	if got := Title("building ai-native platforms", lang.English); got != "Building AI-native Platforms" {
		t.Errorf("Title = %q", got)
	}
	if got := Title("a hands-on introduction", lang.English); got != "A Hands-on Introduction" {
		t.Errorf("Title = %q", got)
	}
}
