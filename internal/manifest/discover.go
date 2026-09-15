package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/vehagn/speaker-promos/internal/cnd"
	"github.com/vehagn/speaker-promos/internal/post"
)

// FindFiles expands paths into the manifest files they contain.
//
// A directory is searched rather than rejected, because the thing you have
// after an export is `out/` — one folder per talk — and naming every promo.yaml
// inside it would be absurd. Shared by the CLI and the preview server so the
// two cannot disagree about what an import covers.
func FindFiles(args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, errors.New("give a promo.yaml, a bundle folder, or an export directory")
	}

	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		if !seen[abs] {
			seen[abs] = true
			out = append(out, p)
		}
	}

	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", arg, err)
		}
		if !info.IsDir() {
			add(arg)
			continue
		}
		var found int
		err = filepath.WalkDir(arg, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && d.Name() == "promo.yaml" {
				add(path)
				found++
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("searching %s: %w", arg, err)
		}
		if found == 0 {
			return nil, fmt.Errorf("no promo.yaml found under %s", arg)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Baseline is what the tool produces for a speaker with no override at all:
// their upstream name and photo, the employer and job guessed out of the
// free-text profile title, and the handles scraped from their profile page.
//
// It is what a submitted correction is compared against, so that touching one
// field cannot quietly record the rest of the found values as corrections too.
func Baseline(sp cnd.Speaker, links cnd.Links) SpeakerSpec {
	role := post.ParseRole(sp.Title)
	return SpeakerSpec{
		Name:     sp.Name,
		Employer: role.Employer,
		Job:      role.Job,
		Title:    sp.Title,
		Image:    sp.Image,
		Links:    LinksOf(links),
	}
}

// speakerBaseline reports the fields an exported bundle pre-fills, which are
// the only ones an import may dismiss as "the guess came back unedited".
//
// It is deliberately narrower than Baseline: a bundle pre-fills the name and
// the guessed employer and job, but never the role line, the photo or the
// handles, so a value in one of those is always something someone typed and is
// taken as an edit.
func speakerBaseline(program *cnd.Program) func(string) (SpeakerSpec, bool) {
	base := map[string]SpeakerSpec{}
	for _, sp := range program.Speakers() {
		if sp.Slug == "" {
			continue
		}
		spec := Baseline(sp, cnd.Links{})
		spec.Title, spec.Image = "", ""
		base[sp.Slug] = spec
	}
	return func(slug string) (SpeakerSpec, bool) {
		spec, ok := base[slug]
		return spec, ok
	}
}

// talkBaseline reports a talk's submitted title.
func talkBaseline(program *cnd.Program) func(string) (string, bool) {
	base := map[string]string{}
	for _, s := range program.Sessions {
		base[s.Talk.ID] = s.Talk.Title
	}
	return func(id string) (string, bool) {
		title, ok := base[id]
		return title, ok
	}
}

// BaselineFor builds the import options that make a merge mean "take the
// edits": the program supplies what the tool would say with no overrides at
// all, so a pre-filled guess coming back unchanged is not mistaken for a
// correction.
func BaselineFor(program *cnd.Program) ImportOptions {
	return ImportOptions{
		SpeakerBaseline: speakerBaseline(program),
		TalkBaseline:    talkBaseline(program),
	}
}
