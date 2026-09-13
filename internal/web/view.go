package web

import (
	"fmt"
	"strings"

	"github.com/vehagn/speaker-promos/internal/cnd"
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
	// Hidden is kept separate from Talk so the template does not have to know
	// that a missing override means "not hidden".
	Hidden bool
	// Warnings are the render-side notes for this card: truncated text, or
	// emoji that some renderers will drop.
	Warnings []string
}

// speakerView is one speaker's resolved facts plus the form's current values.
type speakerView struct {
	Speaker cnd.Speaker
	Role    post.Role
	Links   cnd.Links
	// Override holds what the manifest currently says, which is what the form
	// inputs show. It is empty for a speaker with no corrections yet, so the
	// placeholders fall back to the guessed values.
	Override manifest.SpeakerSpec
	// Guessed marks an employer that came from the heuristic rather than the
	// manifest, so the form can flag it for checking.
	Guessed bool
	// HasPhoto is false when the card fell back to a monogram — either because
	// the speaker has no photo upstream or because the one they have could not
	// be fetched. It is what the image field on the form is for.
	HasPhoto bool
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
