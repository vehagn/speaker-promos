package manifest

import (
	"fmt"

	"github.com/vehagn/speaker-promos/internal/cnd"
	"github.com/vehagn/speaker-promos/internal/post"
)

// bundleHeader introduces a per-talk manifest written into an export folder.
const bundleHeader = `# Everything that produced the cards in this folder.
#
# The TalkInfo object is a record: the whole talk as the tool saw it. It is
# ignored when this file is loaded, so the file can be passed straight back
# with --manifest.
#
# The SpeakerOverride and TalkOverride objects below it are the editable part.
# Their values are the ones actually used: a correction where you made one,
# otherwise the guess the tool made. Change one and pass this file with
# --manifest to apply it.
#
#   name:   the CMS is where people typed their own name, so accents go
#           missing. Correcting it here does not rename this folder.
#   title:  sets the card's role line verbatim, for roles that do not fit
#           "<job> at <employer>". Employer still drives what the post says.
#   image:  a speaker with no photo renders a monogram. Point this at a URL or
#           at a file next to this manifest to give them one.
`

// ConferenceInfo records the event a bundle belongs to.
type ConferenceInfo struct {
	Title      string `yaml:"title"`
	StartDate  string `yaml:"startDate"`
	EndDate    string `yaml:"endDate"`
	City       string `yaml:"city,omitempty"`
	Country    string `yaml:"country,omitempty"`
	Domain     string `yaml:"domain,omitempty"`
	ProgramURL string `yaml:"programUrl,omitempty"`
}

// TalkDetail records the talk itself.
type TalkDetail struct {
	ID string `yaml:"id"`
	// Title is what the cards say, after any displayTitle override.
	Title string `yaml:"title"`
	// SubmittedTitle is the title as submitted, present only when an override
	// changed it — so the record shows both.
	SubmittedTitle string   `yaml:"submittedTitle,omitempty"`
	Day            int      `yaml:"day"`
	Date           string   `yaml:"date"`
	StartTime      string   `yaml:"startTime,omitempty"`
	EndTime        string   `yaml:"endTime,omitempty"`
	Track          string   `yaml:"track,omitempty"`
	Format         string   `yaml:"format,omitempty"`
	FormatLabel    string   `yaml:"formatLabel,omitempty"`
	Level          string   `yaml:"level,omitempty"`
	Topics         []string `yaml:"topics,omitempty"`
	Abstract       string   `yaml:"abstract,omitempty"`
}

// SpeakerInfo records one speaker as the cards and copy saw them.
type SpeakerInfo struct {
	Name string `yaml:"name"`
	Slug string `yaml:"slug,omitempty"`
	// ProfileTitle is the upstream free-text field the employer was guessed
	// from, kept so a wrong guess can be judged without opening the website.
	ProfileTitle string `yaml:"profileTitle,omitempty"`
	Employer     string `yaml:"employer,omitempty"`
	Job          string `yaml:"job,omitempty"`
	// EmployerGuessed marks an employer derived from ProfileTitle rather than
	// taken from an override.
	EmployerGuessed bool   `yaml:"employerGuessed,omitempty"`
	Image           string `yaml:"image,omitempty"`
	// HasPhoto is false when the card fell back to a monogram, which is the
	// case an `image:` override exists to fix.
	HasPhoto   bool   `yaml:"hasPhoto"`
	ProfileURL string `yaml:"profileUrl,omitempty"`
	LinkedIn   string `yaml:"linkedin,omitempty"`
	Bluesky    string `yaml:"bluesky,omitempty"`
	X          string `yaml:"x,omitempty"`
	GitHub     string `yaml:"github,omitempty"`
}

// TalkInfoSpec is the whole record for one bundle.
//
// Deliberately no generation timestamp: an export should be reproducible, so
// that re-running it produces an identical folder and a bundle committed to git
// does not churn. The file times carry that information anyway.
type TalkInfoSpec struct {
	Conference ConferenceInfo `yaml:"conference"`
	Talk       TalkDetail     `yaml:"talk"`
	Speakers   []SpeakerInfo  `yaml:"speakers"`
	// Cards lists the image files written beside this manifest.
	Cards []string `yaml:"cards,omitempty"`
	// Warnings is what the render reported: truncated text, or emoji that some
	// renderers drop.
	Warnings []string `yaml:"warnings,omitempty"`
}

// SessionInfo is everything the caller knows that the manifest does not.
type SessionInfo struct {
	Conference cnd.Conference
	// Session after overrides, i.e. what the cards actually rendered.
	Session cnd.Session
	// Submitted is the session before overrides, for recording the original
	// title alongside the displayed one.
	Submitted cnd.Session
	Speakers  []SpeakerInfo
	Cards     []string
	Warnings  []string
}

// ForSession renders the manifest documents for one talk as YAML.
//
// Unlike Set.Save, every field is written even when it holds a guess rather
// than a correction. An export folder is a record of what produced its cards,
// and a manifest that omitted the guessed employer would not say what the card
// actually claimed — nor give you a line to edit.
//
// The talk's own document is emitted only when it carries something: a display
// title or a hidden flag. There is nothing to pre-fill it with otherwise, and an
// empty object would just be noise.
func (s *Set) ForSession(info SessionInfo, overrides post.Overrides) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess := info.Session
	talk := TalkDetail{
		ID: sess.Talk.ID, Title: sess.Talk.Title, Day: sess.Day, Date: sess.Date,
		StartTime: sess.StartTime, EndTime: sess.EndTime, Track: sess.Track,
		Format: sess.Talk.Format, FormatLabel: sess.Talk.FormatLabel(),
		Level: sess.Talk.Level, Topics: sess.Talk.Topics, Abstract: sess.Talk.Abstract,
	}
	if original := info.Submitted.Talk.Title; original != "" && original != sess.Talk.Title {
		talk.SubmittedTitle = original
	}
	conf := info.Conference

	docs := []document{{
		APIVersion, KindTalkInfo, Metadata{sess.Talk.ID}, TalkInfoSpec{
			Conference: ConferenceInfo{
				Title: conf.Title, StartDate: conf.StartDate, EndDate: conf.EndDate,
				City: conf.City, Country: conf.Country, Domain: conf.Domain,
				ProgramURL: conf.ProgramURL(),
			},
			Talk:     talk,
			Speakers: info.Speakers,
			Cards:    info.Cards,
			Warnings: info.Warnings,
		},
	}}
	for _, sp := range sess.Talk.Speakers {
		if sp.Slug == "" {
			// Overrides are keyed by slug; a speaker without one cannot be
			// addressed, so there is no document to write.
			continue
		}
		spec := s.speakers[sp.Slug]
		// Pre-filled so the name is a line you can correct rather than a field
		// you have to know exists. Title is deliberately left out: it is an
		// escape hatch, and pre-filling it would freeze the composed
		// job-and-employer line and stop those two from doing anything.
		if spec.Name == "" {
			spec.Name = sp.Name
		}
		if spec.Employer == "" && spec.Job == "" {
			// Pre-fill from what the card used, so the file is editable rather
			// than blank. RoleFor resolves the override first and falls back to
			// the heuristic, which is exactly what the card rendered.
			role := overrides.RoleFor(sp)
			spec.Employer, spec.Job = role.Employer, role.Job
		}
		docs = append(docs, document{APIVersion, KindSpeakerOverride, Metadata{sp.Slug}, spec})
	}

	if spec, ok := s.talks[sess.Talk.ID]; ok && !spec.empty() {
		docs = append(docs, document{APIVersion, KindTalkOverride, Metadata{sess.Talk.ID}, spec})
	}

	data, err := encode(bundleHeader, docs)
	if err != nil {
		return nil, fmt.Errorf("encoding manifest for %q: %w", sess.Talk.Title, err)
	}
	return data, nil
}
