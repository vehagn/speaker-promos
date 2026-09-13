package cnd

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Find returns the sessions matching a selector.
//
// Selectors are resolved most-specific first so that a precise identifier is
// never ambiguous: talk id prefix, then speaker slug, then a case-insensitive
// substring of the talk title, then of a speaker name. The first tier that
// matches anything wins, which is why `promo svg sindre-vik` and
// `promo svg "pods on mars"` both do the obvious thing.
func (p Program) Find(selector string) []Session {
	q := strings.ToLower(strings.TrimSpace(selector))
	if q == "" {
		return nil
	}

	tiers := []func(Session) bool{
		func(s Session) bool { return strings.HasPrefix(strings.ToLower(s.Talk.ID), q) },
		func(s Session) bool {
			for _, sp := range s.Talk.Speakers {
				if strings.EqualFold(sp.Slug, q) {
					return true
				}
			}
			return false
		},
		func(s Session) bool { return strings.Contains(strings.ToLower(s.Talk.Title), q) },
		func(s Session) bool { return strings.Contains(strings.ToLower(slugify(s.Talk.Title)), slugify(q)) },
		func(s Session) bool {
			for _, sp := range s.Talk.Speakers {
				if strings.Contains(strings.ToLower(sp.Name), q) {
					return true
				}
			}
			return false
		},
	}

	for _, match := range tiers {
		var hits []Session
		for _, s := range p.Sessions {
			if match(s) {
				hits = append(hits, s)
			}
		}
		if len(hits) > 0 {
			return hits
		}
	}
	return nil
}

// FindOne resolves a selector that must identify exactly one session.
func (p Program) FindOne(selector string) (Session, error) {
	hits := p.Find(selector)
	switch len(hits) {
	case 0:
		return Session{}, fmt.Errorf("no talk matches %q", selector)
	case 1:
		return hits[0], nil
	default:
		var names []string
		for _, h := range hits {
			names = append(names, fmt.Sprintf("%q (%s)", h.Talk.Title, h.SpeakerNames()))
		}
		return Session{}, fmt.Errorf("%q matches %d talks: %s", selector, len(hits), strings.Join(names, ", "))
	}
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slugify reduces text to a lowercase ASCII slug, transliterating the Norwegian
// letters so that "Håvard" and "Havard" both match.
func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case 'æ':
			b.WriteString("ae")
		case 'ø':
			b.WriteString("o")
		case 'å':
			b.WriteString("a")
		default:
			if r > unicode.MaxASCII {
				// Strip combining marks so "é" becomes "e" rather than vanishing.
				if d := unicode.ToLower(r); d < unicode.MaxASCII {
					b.WriteRune(d)
					continue
				}
				b.WriteByte('-')
				continue
			}
			b.WriteRune(r)
		}
	}
	return strings.Trim(nonSlug.ReplaceAllString(b.String(), "-"), "-")
}

// FileStem is the output filename base for a session's promo, e.g.
// "d1-0900-sindre-vik-pods-on-mars". The day and start time lead so that a
// directory listing falls into schedule order.
func (s Session) FileStem() string {
	parts := []string{fmt.Sprintf("d%d", s.Day)}
	if t := nonSlug.ReplaceAllString(strings.ToLower(s.StartTime), ""); t != "" {
		parts = append(parts, t)
	}
	var who []string
	for _, sp := range s.Talk.Speakers {
		if sl := sp.Slug; sl != "" {
			who = append(who, slugify(sl))
		} else if sp.Name != "" {
			who = append(who, slugify(sp.Name))
		}
	}
	if len(who) > 2 {
		who = append(who[:2], "et-al")
	}
	if len(who) > 0 {
		parts = append(parts, strings.Join(who, "_"))
	}
	if title := slugify(s.Talk.Title); title != "" {
		parts = append(parts, truncateSlug(title, 48))
	}
	return strings.Join(parts, "-")
}

func truncateSlug(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if i := strings.LastIndexByte(cut, '-'); i > max/2 {
		cut = cut[:i]
	}
	return strings.Trim(cut, "-")
}

// formatLabels maps the website's format identifiers to display text. Unknown
// identifiers fall back to a generic prettifier rather than being dropped, so a
// newly added format still renders something sensible.
var formatLabels = map[string]string{
	"presentation_20": "20 min talk",
	"presentation_25": "25 min talk",
	"presentation_40": "40 min talk",
	"presentation_45": "45 min talk",
	"lightning_10":    "Lightning talk",
	"workshop_120":    "2 h workshop",
	"workshop_240":    "4 h workshop",
}

// FormatLabel renders a talk's format for display.
func (t Talk) FormatLabel() string {
	if l, ok := formatLabels[t.Format]; ok {
		return l
	}
	return prettify(t.Format)
}

// LevelLabel renders a talk's audience level for display.
func (t Talk) LevelLabel() string { return prettify(t.Level) }

// prettify turns an identifier like "workshop_120" or "intermediate" into
// "Workshop 120" / "Intermediate".
func prettify(s string) string {
	s = strings.NewReplacer("_", " ", "-", " ").Replace(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// TimeRange renders a session's slot as "09:00–11:00".
func (s Session) TimeRange() string {
	switch {
	case s.StartTime == "":
		return ""
	case s.EndTime == "":
		return s.StartTime
	default:
		return s.StartTime + "–" + s.EndTime
	}
}
