package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vehagn/speaker-promos/internal/cnd"
)

func tempPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "promos.yaml")
}

// The manifest in the README, verbatim, so the documented format is the one
// that actually parses.
const sample = `apiVersion: promo.cloudnativedays.no/v1alpha1
kind: SpeakerOverride
metadata:
  name: dario-haaland
spec:
  employer: Bysten Labs
  job: Infrastructure Engineer
  links:
    linkedin: https://www.linkedin.com/in/dario
    bluesky: dario.bsky.social
---
apiVersion: promo.cloudnativedays.no/v1alpha1
kind: TalkOverride
metadata:
  name: 584db4de-d0ac-4d3a-9fc7-33d541b6c862
spec:
  displayTitle: Kort tittel
  hidden: false
`

func TestLoadSample(t *testing.T) {
	path := tempPath(t)
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	set, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	sp, ok := set.Speaker("dario-haaland")
	if !ok {
		t.Fatal("speaker override not loaded")
	}
	if sp.Employer != "Bysten Labs" || sp.Job != "Infrastructure Engineer" {
		t.Errorf("speaker spec = %+v", sp)
	}
	if sp.Links.Bluesky != "dario.bsky.social" {
		t.Errorf("bluesky = %q", sp.Links.Bluesky)
	}

	tk, ok := set.Talk("584db4de-d0ac-4d3a-9fc7-33d541b6c862")
	if !ok {
		t.Fatal("talk override not loaded")
	}
	if tk.DisplayTitle != "Kort tittel" || tk.Hidden {
		t.Errorf("talk spec = %+v", tk)
	}

	// The post package is what resolves overrides, so the bridge to it matters
	// as much as the parse.
	role := set.Overrides().RoleFor(cnd.Speaker{Slug: "dario-haaland", Title: "Bysten Labs"})
	if role.Employer != "Bysten Labs" || role.Job != "Infrastructure Engineer" {
		t.Errorf("RoleFor = %+v", role)
	}
	if role.Guessed {
		t.Error("an overridden employer must not be reported as guessed")
	}
}

func TestMissingFileIsEmpty(t *testing.T) {
	set, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("Load of a missing file: %v", err)
	}
	if s, tk := set.Len(); s != 0 || tk != 0 {
		t.Errorf("Len = %d, %d; want 0, 0", s, tk)
	}
}

// A save must be re-loadable and byte-stable, otherwise the file churns in git
// every time the server touches it.
func TestRoundTripIsStableAndSorted(t *testing.T) {
	path := tempPath(t)
	set := New(path)
	for _, slug := range []string{"zoe", "adam", "mia"} {
		if err := set.SetSpeaker(slug, SpeakerSpec{Employer: "Corp " + slug}); err != nil {
			t.Fatal(err)
		}
	}
	if err := set.SetTalk("t-2", TalkSpec{Hidden: true}); err != nil {
		t.Fatal(err)
	}
	if err := set.SetTalk("t-1", TalkSpec{DisplayTitle: "Short"}); err != nil {
		t.Fatal(err)
	}

	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if s, tk := reloaded.Len(); s != 3 || tk != 2 {
		t.Fatalf("reloaded Len = %d, %d; want 3, 2", s, tk)
	}
	if err := reloaded.Save(); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("load → save is not byte-stable:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}

	// Sorted by kind, then name.
	var order []string
	for _, line := range strings.Split(string(first), "\n") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "name: "); ok {
			order = append(order, name)
		}
	}
	want := []string{"adam", "mia", "zoe", "t-1", "t-2"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("object order = %v; want %v", order, want)
	}
	if !strings.HasPrefix(string(first), "#") {
		t.Error("saved manifest should start with its explanatory header")
	}
}

// Clearing every field should remove the object rather than leave an empty one
// behind, since the preview server writes on every keystroke-ish edit.
func TestEmptySpecIsDeleted(t *testing.T) {
	path := tempPath(t)
	set := New(path)
	if err := set.SetSpeaker("adam", SpeakerSpec{Employer: "Corp"}); err != nil {
		t.Fatal(err)
	}
	if err := set.SetSpeaker("adam", SpeakerSpec{Employer: "   "}); err != nil {
		t.Fatal(err)
	}
	if _, ok := set.Speaker("adam"); ok {
		t.Error("override with only blank fields should have been removed")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "adam") {
		t.Errorf("manifest still mentions the cleared speaker:\n%s", b)
	}
}

// The whole point of validating apiVersion and kind is that a typo is loud.
func TestRejectsBadDocuments(t *testing.T) {
	cases := []struct {
		name, yaml, want string
	}{
		{
			"wrong apiVersion",
			"apiVersion: promo.cloudnativedays.no/v1\nkind: SpeakerOverride\nmetadata:\n  name: a\nspec:\n  employer: C\n",
			"unsupported apiVersion",
		},
		{
			"unknown kind",
			"apiVersion: " + APIVersion + "\nkind: SpeakerOverrides\nmetadata:\n  name: a\nspec:\n  employer: C\n",
			"unknown kind",
		},
		{
			"missing name",
			"apiVersion: " + APIVersion + "\nkind: SpeakerOverride\nmetadata: {}\nspec:\n  employer: C\n",
			"metadata.name",
		},
		{
			"typo in a spec field",
			"apiVersion: " + APIVersion + "\nkind: SpeakerOverride\nmetadata:\n  name: a\nspec:\n  employeer: C\n",
			`unknown field "employeer"`,
		},
		{
			"typo in a link field",
			"apiVersion: " + APIVersion + "\nkind: SpeakerOverride\nmetadata:\n  name: a\nspec:\n  links:\n    linkedn: x\n",
			`unknown field "linkedn"`,
		},
		{
			"typo at the top level",
			"apiVersion: " + APIVersion + "\nkind: TalkOverride\nmetadata:\n  name: a\nspecs:\n  hidden: true\n",
			`unknown field "specs"`,
		},
		{
			"talk field on a speaker",
			"apiVersion: " + APIVersion + "\nkind: SpeakerOverride\nmetadata:\n  name: a\nspec:\n  hidden: true\n",
			`unknown field "hidden"`,
		},
		{
			"duplicate object",
			"apiVersion: " + APIVersion + "\nkind: TalkOverride\nmetadata:\n  name: a\nspec:\n  hidden: true\n---\n" +
				"apiVersion: " + APIVersion + "\nkind: TalkOverride\nmetadata:\n  name: a\nspec:\n  hidden: false\n",
			"duplicate",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := tempPath(t)
			if err := os.WriteFile(path, []byte(c.yaml), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil {
				t.Fatalf("Load accepted %s", c.name)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %q; want it to mention %q", err, c.want)
			}
			if !strings.Contains(err.Error(), "line ") {
				t.Errorf("error = %q; want it to name a line", err)
			}
		})
	}
}

// Blank documents occur naturally when a file is edited by hand.
func TestSkipsEmptyDocuments(t *testing.T) {
	path := tempPath(t)
	body := "---\n" + sample + "---\n# just a comment\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	set, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s, tk := set.Len(); s != 1 || tk != 1 {
		t.Errorf("Len = %d, %d; want 1, 1", s, tk)
	}
}

func TestApplyRewritesAndFilters(t *testing.T) {
	set := New(tempPath(t))
	if err := set.SetTalk("keep", TalkSpec{DisplayTitle: "Short Title"}); err != nil {
		t.Fatal(err)
	}
	if err := set.SetTalk("gone", TalkSpec{Hidden: true}); err != nil {
		t.Fatal(err)
	}

	in := []cnd.Session{
		{Talk: cnd.Talk{ID: "keep", Title: "A Very Long Original Title"}},
		{Talk: cnd.Talk{ID: "gone", Title: "Cancelled"}},
		{Talk: cnd.Talk{ID: "other", Title: "Untouched"}},
	}
	got := set.Apply(in)
	if len(got) != 2 {
		t.Fatalf("Apply returned %d sessions; want 2", len(got))
	}
	if got[0].Talk.Title != "Short Title" {
		t.Errorf("title = %q; want the display title", got[0].Talk.Title)
	}
	if got[1].Talk.Title != "Untouched" {
		t.Errorf("title = %q; want it unchanged", got[1].Talk.Title)
	}
	// Apply must not mutate the caller's slice.
	if in[0].Talk.Title != "A Very Long Original Title" {
		t.Errorf("Apply mutated its input: %q", in[0].Talk.Title)
	}
	// An explicit selection is rewritten but not filtered.
	if s := set.Rewrite(in[1]); s.Talk.Title != "Cancelled" {
		t.Errorf("Rewrite of a hidden talk = %q", s.Talk.Title)
	}
	if !set.Hidden("gone") || set.Hidden("keep") {
		t.Error("Hidden disagrees with the specs")
	}
}

func TestRoleTitle(t *testing.T) {
	for _, tc := range []struct {
		spec SpeakerSpec
		want string
	}{
		// The upstream convention, so a corrected role reads like an
		// uncorrected one.
		{SpeakerSpec{Job: "Senior Platform Engineer", Employer: "Vestbit"}, "Senior Platform Engineer at Vestbit"},
		{SpeakerSpec{Employer: "Bysten Labs"}, "Bysten Labs"},
		{SpeakerSpec{Job: "Utvikler"}, "Utvikler"},
		// Nothing said about the role: the caller keeps the upstream value.
		{SpeakerSpec{}, ""},
		{SpeakerSpec{Links: Links{Bluesky: "a.example"}}, ""},
	} {
		if got := tc.spec.RoleTitle(); got != tc.want {
			t.Errorf("RoleTitle(%+v) = %q, want %q", tc.spec, got, tc.want)
		}
	}
}

// A speaker override has to reach the CARD, not just the copy: the reason to
// correct an employer is that the graphic says the wrong thing.
func TestRewriteAppliesSpeakerOverrideToCardTitle(t *testing.T) {
	set := New(tempPath(t))
	if err := set.SetSpeaker("dario-haaland", SpeakerSpec{
		Employer: "Bysten Labs AS",
		Job:      "Infrastructure Engineer",
	}); err != nil {
		t.Fatal(err)
	}

	sess := cnd.Session{Talk: cnd.Talk{
		ID: "talk-1",
		Speakers: []cnd.Speaker{
			{Slug: "dario-haaland", Name: "Dario Haaland", Title: "Bysten Labs"},
			{Slug: "untouched", Name: "Someone Else", Title: "Dev at Acme"},
		},
	}}

	got := set.Rewrite(sess)
	if want := "Infrastructure Engineer at Bysten Labs AS"; got.Talk.Speakers[0].Title != want {
		t.Errorf("speaker 0 title = %q, want %q", got.Talk.Speakers[0].Title, want)
	}
	// A speaker with no override keeps the upstream value.
	if got.Talk.Speakers[1].Title != "Dev at Acme" {
		t.Errorf("speaker 1 title = %q, want it untouched", got.Talk.Speakers[1].Title)
	}
}

// Regression: cnd.Session is a value but its Speakers slice shares a backing
// array with the Program, so a speaker on two talks has one array. Writing a
// rewritten title in place leaked one session's override into every other
// session that speaker appeared in.
func TestRewriteDoesNotMutateTheSharedSpeakerSlice(t *testing.T) {
	set := New(tempPath(t))
	if err := set.SetSpeaker("shared", SpeakerSpec{Employer: "Corrected"}); err != nil {
		t.Fatal(err)
	}

	// Both sessions share one backing array, as they do when they come from a
	// single Program.
	speakers := []cnd.Speaker{{Slug: "shared", Name: "Shared Speaker", Title: "Original"}}
	first := cnd.Session{Talk: cnd.Talk{ID: "talk-1", Speakers: speakers}}
	second := cnd.Session{Talk: cnd.Talk{ID: "talk-2", Speakers: speakers}}

	rewritten := set.Rewrite(first)
	if rewritten.Talk.Speakers[0].Title != "Corrected" {
		t.Fatalf("rewrite did not apply: %q", rewritten.Talk.Speakers[0].Title)
	}
	if speakers[0].Title != "Original" {
		t.Errorf("the shared backing array was mutated: %q", speakers[0].Title)
	}
	if second.Talk.Speakers[0].Title != "Original" {
		t.Errorf("the override leaked into another session: %q", second.Talk.Speakers[0].Title)
	}
}

func TestRewriteWithoutOverridesReturnsInputUnchanged(t *testing.T) {
	set := New(tempPath(t))
	speakers := []cnd.Speaker{{Slug: "nobody", Title: "Original"}}
	sess := cnd.Session{Talk: cnd.Talk{ID: "talk-1", Title: "Kept", Speakers: speakers}}

	got := set.Rewrite(sess)
	if got.Talk.Title != "Kept" || got.Talk.Speakers[0].Title != "Original" {
		t.Errorf("unchanged session was altered: %+v", got.Talk)
	}
}

// TalkInfo is informational: it must load without error and must not become an
// override, so an exported bundle can be fed straight back.
func TestTalkInfoIsAcceptedAndIgnored(t *testing.T) {
	path := tempPath(t)
	body := "apiVersion: " + APIVersion + `
kind: TalkInfo
metadata:
  name: talk-1
spec:
  conference:
    title: Cloud Native Days Norway 2026
  talk:
    id: talk-1
    title: Kan skyen kjøre?
  speakers:
    - name: Dario Haaland
      hasPhoto: false
---
apiVersion: ` + APIVersion + `
kind: SpeakerOverride
metadata:
  name: dario-haaland
spec:
  employer: Bysten Labs
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	set, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := set.Talk("talk-1"); ok {
		t.Error("TalkInfo was mistaken for a TalkOverride")
	}
	if s, tk := set.Len(); s != 1 || tk != 0 {
		t.Errorf("Len = %d, %d; want 1, 0", s, tk)
	}

	// Its spec is deliberately not field-checked, so a bundle written by a
	// newer version stays loadable.
	unknown := "apiVersion: " + APIVersion + "\nkind: TalkInfo\nmetadata:\n  name: t\nspec:\n  somethingNew: 1\n"
	if err := os.WriteFile(path, []byte(unknown), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Errorf("an unknown TalkInfo field should be tolerated: %v", err)
	}
}

// The image override has to reach the rendered speaker, and a relative path has
// to resolve against the manifest rather than the working directory.
func TestImageOverrideAppliesAndResolves(t *testing.T) {
	path := tempPath(t)
	set := New(path)
	if err := set.SetSpeaker("dario-haaland", SpeakerSpec{Image: "photos/dario.jpg"}); err != nil {
		t.Fatal(err)
	}
	sess := cnd.Session{Talk: cnd.Talk{ID: "t", Speakers: []cnd.Speaker{
		{Slug: "dario-haaland", Name: "Dario Haaland"},
		{Slug: "other", Name: "Someone Else", Image: "https://example.com/keep.jpg"},
	}}}

	got := set.Rewrite(sess)
	want := filepath.Join(filepath.Dir(path), "photos/dario.jpg")
	if got.Talk.Speakers[0].Image != want {
		t.Errorf("image = %q, want %q resolved against the manifest", got.Talk.Speakers[0].Image, want)
	}
	if got.Talk.Speakers[1].Image != "https://example.com/keep.jpg" {
		t.Errorf("an unrelated speaker's image changed: %q", got.Talk.Speakers[1].Image)
	}

	// An URL and an absolute path are passed through untouched.
	for _, image := range []string{"https://example.com/a.jpg", "/tmp/a.jpg"} {
		if err := set.SetSpeaker("dario-haaland", SpeakerSpec{Image: image}); err != nil {
			t.Fatal(err)
		}
		if got := set.Rewrite(sess).Talk.Speakers[0].Image; got != image {
			t.Errorf("image %q became %q", image, got)
		}
	}
}

// An image-only override is a real override, so it must survive a save/load
// cycle rather than being pruned as empty.
func TestImageOnlyOverrideRoundTrips(t *testing.T) {
	path := tempPath(t)
	set := New(path)
	if err := set.SetSpeaker("dario-haaland", SpeakerSpec{Image: "photos/dario.jpg"}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	spec, ok := reloaded.Speaker("dario-haaland")
	if !ok || spec.Image != "photos/dario.jpg" {
		t.Errorf("reloaded = %+v, ok=%v", spec, ok)
	}
}

// A name is typed by the speaker into a CMS, so accents go missing. Correcting
// it must reach the card and the copy but must NOT change the slug, which is
// the speaker's identity and drives folder names and selectors.
func TestNameOverrideDoesNotChangeIdentity(t *testing.T) {
	path := tempPath(t)
	set := New(path)
	if err := set.SetSpeaker("aurelie-vache", SpeakerSpec{Name: "Aurélie Vache"}); err != nil {
		t.Fatal(err)
	}
	sess := cnd.Session{Day: 2, StartTime: "15:30", Talk: cnd.Talk{
		ID: "t", Title: "Understanding Kubernetes",
		Speakers: []cnd.Speaker{{Slug: "aurelie-vache", Name: "Aurelie Vache"}},
	}}
	before := sess.FileStem()

	got := set.Rewrite(sess)
	if got.Talk.Speakers[0].Name != "Aurélie Vache" {
		t.Errorf("name = %q", got.Talk.Speakers[0].Name)
	}
	if got.Talk.Speakers[0].Slug != "aurelie-vache" {
		t.Errorf("slug changed to %q", got.Talk.Speakers[0].Slug)
	}
	if after := got.FileStem(); after != before {
		t.Errorf("file stem changed from %q to %q", before, after)
	}
}

// Plenty of real titles do not fit "<job> at <employer>", so an explicit title
// sets the role line verbatim and wins over both.
func TestTitleOverrideWinsOverJobAndEmployer(t *testing.T) {
	verbatim := "Maintainer, Principal Open source Architect, Co-Chair CNCF TAG Infrastructure"
	for _, tc := range []struct {
		spec SpeakerSpec
		want string
	}{
		{SpeakerSpec{Title: verbatim}, verbatim},
		{SpeakerSpec{Title: verbatim, Job: "Engineer", Employer: "Vestbit"}, verbatim},
		{SpeakerSpec{Job: "Engineer", Employer: "Vestbit"}, "Engineer at Vestbit"},
		{SpeakerSpec{Name: "Only a name"}, ""},
	} {
		if got := tc.spec.RoleTitle(); got != tc.want {
			t.Errorf("RoleTitle(%+v) = %q, want %q", tc.spec, got, tc.want)
		}
	}

	// Employer still drives what the POST names, so a title alone changes the
	// card without silencing the employer guess.
	path := tempPath(t)
	set := New(path)
	if err := set.SetSpeaker("a", SpeakerSpec{Title: verbatim}); err != nil {
		t.Fatal(err)
	}
	role := set.Overrides().RoleFor(cnd.Speaker{Slug: "a", Title: "Dev at Acme"})
	if role.Employer != "Acme" {
		t.Errorf("employer = %q, want it still guessed from upstream", role.Employer)
	}
	if !role.Guessed {
		t.Error("a title-only override should leave the employer marked as guessed")
	}
}

// A name-only or title-only override is a real override and must survive a
// save/load cycle rather than being pruned as empty.
func TestNameAndTitleOnlyOverridesRoundTrip(t *testing.T) {
	for _, spec := range []SpeakerSpec{
		{Name: "Aurélie Vache"},
		{Title: "Tech Lead, Platform"},
	} {
		path := tempPath(t)
		set := New(path)
		if err := set.SetSpeaker("a", spec); err != nil {
			t.Fatal(err)
		}
		reloaded, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := reloaded.Speaker("a")
		if !ok || got != spec {
			t.Errorf("reloaded %+v as %+v (ok=%v)", spec, got, ok)
		}
	}
}

func TestSpeakerSpecRejectsUnknownFields(t *testing.T) {
	path := tempPath(t)
	body := "apiVersion: " + APIVersion + "\nkind: SpeakerOverride\nmetadata:\n  name: a\nspec:\n  nmae: x\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("want an error for a misspelled field")
	}
	// The error must list the fields that now exist, name and title included.
	for _, want := range []string{"name", "title", "employer", "job", "image", "links"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should list %q", err, want)
		}
	}
}
