package manifest

import (
	"fmt"
	"sort"
	"strings"
)

// Change is one field an import would alter.
type Change struct {
	Kind  string
	Name  string
	Field string
	// From is the project manifest's current value, empty when it had none.
	From string
	To   string
}

// String renders a change for a report line.
func (c Change) String() string {
	if c.From == "" {
		return fmt.Sprintf("%s/%s %s: %q", c.Kind, c.Name, c.Field, c.To)
	}
	return fmt.Sprintf("%s/%s %s: %q → %q", c.Kind, c.Name, c.Field, c.From, c.To)
}

// ImportOptions controls a merge.
type ImportOptions struct {
	// SpeakerBaseline reports what the tool would produce for a speaker with no
	// override at all. It is what makes an import mean "take the edits": an
	// exported manifest pre-fills the name, employer and job, so importing it
	// verbatim would turn every guess into a confirmed correction and silence
	// the warnings that exist to be read.
	SpeakerBaseline func(slug string) (SpeakerSpec, bool)
	// TalkBaseline reports a talk's submitted title, so a displayTitle that
	// merely repeats it is not mistaken for a shortening.
	TalkBaseline func(id string) (title string, ok bool)
	// ConfirmGuesses imports the pre-filled values too, which is how you say
	// "yes, that guess was right" and stop being asked.
	ConfirmGuesses bool
	// DryRun computes the changes without writing anything.
	DryRun bool
}

// ImportFrom merges another manifest's overrides into this one.
//
// The whole merge happens under one lock and is written once, so an import
// either lands completely or not at all — a half-merged manifest would be worse
// than a failed one, since the point of the file is to be trustworthy.
func (s *Set) ImportFrom(src *Set, opts ImportOptions) ([]Change, error) {
	if src == nil {
		return nil, nil
	}
	incoming := src.Speakers()
	incomingTalks := src.Talks()

	s.mu.Lock()
	defer s.mu.Unlock()

	var changes []Change
	for _, slug := range sortedKeys(incoming) {
		spec := trimSpeaker(incoming[slug])
		current := s.speakers[slug]

		var base SpeakerSpec
		if opts.SpeakerBaseline != nil && !opts.ConfirmGuesses {
			if b, ok := opts.SpeakerBaseline(slug); ok {
				base = trimSpeaker(b)
			}
		}

		merged := current
		for _, f := range speakerFields(&spec, &merged, &current, &base) {
			// Unset in the import: nothing said, so nothing changes. Clearing a
			// field is done by editing the project manifest, not by omitting it
			// from a bundle — half the bundles would otherwise clear whatever
			// they happened not to mention.
			if *f.incoming == "" {
				continue
			}
			// Equal to the baseline: the pre-filled guess came back unedited.
			if *f.incoming == *f.baseline {
				continue
			}
			if *f.incoming == *f.current {
				continue
			}
			changes = append(changes, Change{
				Kind: KindSpeakerOverride, Name: slug, Field: f.name,
				From: *f.current, To: *f.incoming,
			})
			*f.target = *f.incoming
		}
		if merged != current {
			if !opts.DryRun {
				s.speakers[slug] = trimSpeaker(merged)
				if s.speakers[slug].empty() {
					delete(s.speakers, slug)
				}
			}
		}
	}

	for _, id := range sortedKeys(incomingTalks) {
		spec := incomingTalks[id]
		current := s.talks[id]
		merged := current

		title := strings.TrimSpace(spec.DisplayTitle)
		if title != "" && title != current.DisplayTitle {
			submitted := ""
			if opts.TalkBaseline != nil {
				if t, ok := opts.TalkBaseline(id); ok {
					submitted = t
				}
			}
			if title != submitted {
				changes = append(changes, Change{
					Kind: KindTalkOverride, Name: id, Field: "displayTitle",
					From: current.DisplayTitle, To: title,
				})
				merged.DisplayTitle = title
			}
		}
		if lang := strings.TrimSpace(spec.Language); lang != "" && lang != current.Language {
			changes = append(changes, Change{
				Kind: KindTalkOverride, Name: id, Field: "language",
				From: current.Language, To: lang,
			})
			merged.Language = lang
		}
		// Hidden is a bool, so "unset" and "false" are indistinguishable in the
		// file. Only turning it ON is importable; unhiding is done in the
		// project manifest or the browser, where the intent is unambiguous.
		if spec.Hidden && !current.Hidden {
			changes = append(changes, Change{
				Kind: KindTalkOverride, Name: id, Field: "hidden",
				From: "false", To: "true",
			})
			merged.Hidden = true
		}

		if merged != current && !opts.DryRun {
			if merged.empty() {
				delete(s.talks, id)
			} else {
				s.talks[id] = merged
			}
		}
	}

	if len(changes) == 0 || opts.DryRun {
		return changes, nil
	}
	if err := s.save(); err != nil {
		return changes, err
	}
	return changes, nil
}

// speakerField binds one field across the four specs a merge compares.
type speakerField struct {
	name     string
	incoming *string
	target   *string
	current  *string
	baseline *string
}

func speakerFields(incoming, target, current, baseline *SpeakerSpec) []speakerField {
	return []speakerField{
		{"name", &incoming.Name, &target.Name, &current.Name, &baseline.Name},
		{"employer", &incoming.Employer, &target.Employer, &current.Employer, &baseline.Employer},
		{"job", &incoming.Job, &target.Job, &current.Job, &baseline.Job},
		{"title", &incoming.Title, &target.Title, &current.Title, &baseline.Title},
		{"image", &incoming.Image, &target.Image, &current.Image, &baseline.Image},
		{"linkedin", &incoming.Links.LinkedIn, &target.Links.LinkedIn, &current.Links.LinkedIn, &baseline.Links.LinkedIn},
		{"bluesky", &incoming.Links.Bluesky, &target.Links.Bluesky, &current.Links.Bluesky, &baseline.Links.Bluesky},
		{"x", &incoming.Links.X, &target.Links.X, &current.Links.X, &baseline.Links.X},
		{"github", &incoming.Links.GitHub, &target.Links.GitHub, &current.Links.GitHub, &baseline.Links.GitHub},
	}
}

// Speakers returns every speaker override, keyed by slug. The map is a copy.
func (s *Set) Speakers() map[string]SpeakerSpec {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]SpeakerSpec, len(s.speakers))
	for k, v := range s.speakers {
		out[k] = v
	}
	return out
}

// Talks returns every talk override, keyed by talk id. The map is a copy.
func (s *Set) Talks() map[string]TalkSpec {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]TalkSpec, len(s.talks))
	for k, v := range s.talks {
		out[k] = v
	}
	return out
}

// SortChanges orders changes for a stable report.
func SortChanges(changes []Change) {
	sort.Slice(changes, func(i, j int) bool {
		a, b := changes[i], changes[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Field < b.Field
	})
}
