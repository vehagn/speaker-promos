// Package post drafts social copy to accompany a promo card.
package post

import (
	"strings"

	"github.com/vehagn/speaker-promos/internal/cnd"
)

// Role is a speaker's job title and employer, as far as they can be determined.
type Role struct {
	// Job is the role, e.g. "Staff Developer Advocate". May be empty.
	Job string
	// Employer is the company, e.g. "Vestbit Labs". May be empty.
	Employer string
	// Guessed is true when Employer was inferred from free text rather than
	// taken from an override. Copy marks these so they get checked before
	// posting.
	Guessed bool
}

// separators split a profile title into role and employer.
//
// The upstream `title` field is free text with no structure, and the 2026
// program shows the full range: "Staff Developer Advocate at Vestbit Labs",
// "Senior Cloud Dev Advocate @Havbris", "Utvikler hos Bergsdal", "Bysten Labs", and
// empty. These are the separators that actually occur.
//
// " i " is deliberately absent. It is the Norwegian "in", and while
// "faggruppeleder i Tindra" does mean Tindra is the employer, " i " also appears
// mid-phrase often enough ("Drift i Praksis") that matching it produced worse
// guesses than leaving the whole string as the employer.
var separators = []string{" at ", " hos ", " @ ", " @", "@"}

// ParseRole splits a speaker's profile title into job and employer.
//
// The LEFTMOST separator wins rather than the first one in the list: the role
// comes before the employer, so in "Platform lead hos Tindra at Oslo" the split
// belongs at " hos ", and preferring " at " because it happens to be listed
// first would take "Oslo" as the employer.
//
// A title with no separator is treated as a bare employer rather than a bare
// job: the cases that occur in practice are company names ("Bysten Labs"), and
// naming the wrong thing in a post is worse than naming nothing.
func ParseRole(title string) Role {
	title = strings.TrimSpace(title)
	if title == "" {
		return Role{}
	}

	best := -1
	var bestSep string
	for _, sep := range separators {
		i := strings.Index(title, sep)
		if i < 0 {
			continue
		}
		// On a tie the longer separator wins, so " @ " beats "@" at the same
		// position and the surrounding spaces are not left in the output.
		if best < 0 || i < best || (i == best && len(sep) > len(bestSep)) {
			best, bestSep = i, sep
		}
	}
	if best >= 0 {
		job := strings.TrimSpace(title[:best])
		employer := strings.TrimSpace(title[best+len(bestSep):])
		if job != "" && employer != "" {
			return Role{Job: job, Employer: employer, Guessed: true}
		}
	}
	// No usable separator: assume the whole string names the employer.
	return Role{Employer: title, Guessed: true}
}

// Overrides maps a speaker slug to corrections for the guessed data.
//
// The heuristic above is wrong often enough that a correction file is part of
// the design rather than an afterthought; see `promo post --speakers`.
type Overrides map[string]Override

// Override corrects or supplements what is known about one speaker.
type Override struct {
	Employer string `yaml:"employer"`
	Job      string `yaml:"job"`
	LinkedIn string `yaml:"linkedin"`
	Bluesky  string `yaml:"bluesky"`
	X        string `yaml:"x"`
	GitHub   string `yaml:"github"`
}

// RoleFor resolves a speaker's role, preferring an override over the guess.
func (o Overrides) RoleFor(s cnd.Speaker) Role {
	r := ParseRole(s.Title)
	ov, ok := o[s.Slug]
	if !ok {
		return r
	}
	if ov.Employer != "" {
		r.Employer = ov.Employer
		r.Guessed = false
	}
	if ov.Job != "" {
		r.Job = ov.Job
	}
	return r
}

// LinksFor merges scraped links with any overrides, which win.
func (o Overrides) LinksFor(s cnd.Speaker, scraped cnd.Links) cnd.Links {
	ov, ok := o[s.Slug]
	if !ok {
		return scraped
	}
	if ov.LinkedIn != "" {
		scraped.LinkedIn = ov.LinkedIn
	}
	if ov.Bluesky != "" {
		scraped.Bluesky = strings.TrimPrefix(ov.Bluesky, "@")
	}
	if ov.X != "" {
		scraped.X = strings.TrimPrefix(ov.X, "@")
	}
	if ov.GitHub != "" {
		scraped.GitHub = ov.GitHub
	}
	return scraped
}
