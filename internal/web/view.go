package web

import (
	"fmt"
	"strings"

	"github.com/vehagn/speaker-promos/internal/cnd"
	"github.com/vehagn/speaker-promos/internal/lang"
	"github.com/vehagn/speaker-promos/internal/manifest"
	"github.com/vehagn/speaker-promos/internal/post"
)

// talkView is everything the page shows about one talk.
//
// The templates render only from this, never from the domain types directly, so
// that "what the card will say" and "what the form is pre-filled with" are
// computed in one place and cannot drift apart.
type talkView struct {
	Session cnd.Session
	// CardURL carries the manifest revision so an edit busts the browser's
	// cache for that one card. Without it an HTMX swap re-inserts the same
	// <img src>, the browser serves the stale bytes, and the edit looks like it
	// did nothing.
	CardURL  string
	Size     string
	Speakers []speakerView
	Drafts   []draftView
	Talk     manifest.TalkSpec
	// SubmittedTitle is the talk's title before any displayTitle override. The
	// form is pre-filled with the effective title, so this is what a submission
	// is diffed against.
	SubmittedTitle string
	// Hidden is kept separate from Talk so the template does not have to know
	// that a missing override means "not hidden".
	Hidden bool
	// Warnings are the render-side notes for this card: truncated text, or
	// emoji that some renderers will drop.
	Warnings []string
	// Language is the language the drafts below were actually written in, after
	// any override and detection, so a wrong guess is visible.
	Language lang.Language
	// Detected is what detection picks on its own, so the "auto" option can say
	// what it means for this talk.
	Detected lang.Language
}

// speakerView is one speaker's resolved facts plus the form's current values.
type speakerView struct {
	Speaker cnd.Speaker
	Role    post.Role
	Links   cnd.Links
	// Override holds what the manifest currently says. It is empty for a
	// speaker with no corrections yet.
	Override manifest.SpeakerSpec
	// Effective is what the card and copy actually use: the correction where
	// there is one, otherwise the value the tool found. It is what the form
	// inputs are pre-filled with, so a found value can be edited in place
	// rather than retyped from a grey placeholder.
	//
	// Because every field therefore arrives populated, the handler stores only
	// what differs from Baseline. Writing the lot back would turn every guess
	// into a confirmed correction the first time any field was touched.
	Effective manifest.SpeakerSpec
	// Baseline is the same speaker with no override at all — what the program
	// and the scrape give. The handler diffs against it; the view uses it to
	// tell a found value from a corrected one.
	Baseline manifest.SpeakerSpec
	// Guessed marks an employer that came from the heuristic rather than the
	// manifest, so the form can flag it for checking.
	Guessed bool
	// HasPhoto is false when the card fell back to a monogram — either because
	// the speaker has no photo upstream or because the one they have could not
	// be fetched. It is what the image field on the form is for.
	HasPhoto bool
}

// DisplayName is the name the card shows: the override when there is one, so
// the form's heading matches the artwork rather than the CMS.
func (v speakerView) DisplayName() string {
	if v.Override.Name != "" {
		return v.Override.Name
	}
	return v.Speaker.Name
}

// draftView is one platform's copy, with its length budget resolved.
type draftView struct {
	Draft post.Draft
	Limit int
	Over  bool
}

// Percent is how much of the platform's limit the draft uses, for a meter.
func (d draftView) Percent() int {
	if d.Limit <= 0 {
		return 0
	}
	p := d.Draft.Runes() * 100 / d.Limit
	if p > 100 {
		p = 100
	}
	return p
}

// Count renders the length as "260/300" where a limit applies, else "582".
func (d draftView) Count() string {
	if d.Limit <= 0 {
		return fmt.Sprintf("%d", d.Draft.Runes())
	}
	return fmt.Sprintf("%d/%d", d.Draft.Runes(), d.Limit)
}

// Anchor is a stable DOM id for a talk's row, used as the HTMX swap target.
func (v talkView) Anchor() string { return "talk-" + domID(v.Session.Talk.ID) }

// domID reduces an identifier to something safe in an HTML id attribute.
func domID(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// Corrected reports whether a field currently differs from what was found,
// which is what the form marks as yours rather than the tool's.
func (v speakerView) Corrected(field string) bool {
	switch field {
	case "name":
		return v.Override.Name != "" && v.Override.Name != v.Baseline.Name
	case "employer":
		return v.Override.Employer != "" && v.Override.Employer != v.Baseline.Employer
	case "job":
		return v.Override.Job != "" && v.Override.Job != v.Baseline.Job
	case "title":
		return v.Override.Title != "" && v.Override.Title != v.Baseline.Title
	case "image":
		return v.Override.Image != "" && v.Override.Image != v.Baseline.Image
	default:
		return false
	}
}
