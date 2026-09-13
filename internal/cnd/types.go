// Package cnd models a Cloud Native Days conference program and loads it from
// the public website.
package cnd

import (
	"fmt"
	"strings"
	"time"
)

// Conference is the event a program belongs to.
type Conference struct {
	Title     string
	StartDate string // ISO date, e.g. "2026-10-26"
	EndDate   string
	City      string
	Country   string
	Domain    string // primary public domain
	// LogoBright is the conference wordmark as inline SVG, in the light-on-dark
	// variant. Promo cards paint on a saturated brand gradient, so this is the
	// variant that applies; LogoDark is kept for light themes.
	LogoBright string
	LogoDark   string
	// LogomarkBright is the square mark without wordmark, for tight layouts.
	LogomarkBright string
}

// Speaker is one presenter of a talk.
type Speaker struct {
	ID   string
	Name string
	Slug string
	// Title is free text from the speaker's profile and is wildly inconsistent
	// across speakers: "Staff Developer Advocate at Vestbit Labs", "Bysten Labs",
	// "Utvikler hos Bergsdal", or empty. There is no structured employer field, so
	// anything that needs an employer has to guess — see Employer in post copy.
	Title string
	// Image is a cdn.sanity.io URL. It accepts crop transforms; use ImageURL.
	Image string
}

// ImageURL returns the speaker photo cropped to a square of the given size.
// It returns "" when the speaker has no photo.
func (s Speaker) ImageURL(size int) string {
	if s.Image == "" {
		return ""
	}
	sep := "?"
	if strings.Contains(s.Image, "?") {
		sep = "&"
	}
	// `auto=format` is deliberately omitted: the fetcher sends `Accept: */*`,
	// so it would only widen the set of codecs handed back for no benefit.
	return fmt.Sprintf("%s%sw=%d&h=%d&fit=crop", s.Image, sep, size, size)
}

// Talk is an accepted session.
type Talk struct {
	ID string
	// Title is presenter-authored and may contain emoji, which no text font
	// covers. See internal/layout for how that is handled.
	Title string
	// Abstract is the talk description flattened to plain text.
	Abstract string
	Format   string // e.g. "workshop_120", "presentation_25"
	Level    string // e.g. "beginner", "intermediate", "advanced"
	Topics   []string
	Speakers []Speaker
}

// Session is a talk placed in the schedule.
type Session struct {
	Talk      Talk
	Date      string // ISO date of the day it runs
	Day       int    // 1-based day index within the conference
	Track     string
	StartTime string // "HH:MM"
	EndTime   string
}

// Program is a conference and its scheduled sessions.
type Program struct {
	Conference Conference
	Sessions   []Session
}

// Speakers returns every distinct speaker in the program, in first-appearance
// order.
func (p Program) Speakers() []Speaker {
	seen := make(map[string]bool)
	var out []Speaker
	for _, s := range p.Sessions {
		for _, sp := range s.Talk.Speakers {
			key := sp.Slug
			if key == "" {
				key = sp.ID
			}
			if !seen[key] {
				seen[key] = true
				out = append(out, sp)
			}
		}
	}
	return out
}

// SpeakerNames renders a session's speakers as prose: "A", "A and B", or
// "A, B and C".
func (s Session) SpeakerNames() string {
	names := make([]string, 0, len(s.Talk.Speakers))
	for _, sp := range s.Talk.Speakers {
		if sp.Name != "" {
			names = append(names, sp.Name)
		}
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

// DateRange renders the conference dates compactly: "26–27 October 2026", or
// "26 October – 2 November 2026" when the months differ.
func (c Conference) DateRange() string {
	start, err1 := time.Parse(time.DateOnly, c.StartDate)
	end, err2 := time.Parse(time.DateOnly, c.EndDate)
	switch {
	case err1 != nil && err2 != nil:
		return ""
	case err1 != nil:
		return end.Format("2 January 2006")
	case err2 != nil || start.Equal(end):
		return start.Format("2 January 2006")
	case start.Year() == end.Year() && start.Month() == end.Month():
		return fmt.Sprintf("%d–%s", start.Day(), end.Format("2 January 2006"))
	case start.Year() == end.Year():
		return fmt.Sprintf("%s – %s", start.Format("2 January"), end.Format("2 January 2006"))
	default:
		return fmt.Sprintf("%s – %s", start.Format("2 January 2006"), end.Format("2 January 2006"))
	}
}

// Location renders "City, Country", omitting either if absent.
func (c Conference) Location() string {
	parts := make([]string, 0, 2)
	if c.City != "" {
		parts = append(parts, c.City)
	}
	if c.Country != "" {
		parts = append(parts, c.Country)
	}
	return strings.Join(parts, ", ")
}

// URL is the conference site root.
func (c Conference) URL() string {
	if c.Domain == "" {
		return ""
	}
	return "https://" + c.Domain
}

// SpeakerURL is the public profile page for a speaker.
func (c Conference) SpeakerURL(s Speaker) string {
	if c.Domain == "" || s.Slug == "" {
		return ""
	}
	return fmt.Sprintf("https://%s/speaker/%s", c.Domain, s.Slug)
}
