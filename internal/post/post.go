package post

import (
	"fmt"
	"strings"

	"github.com/vehagn/speaker-promos/internal/cnd"
)

// BlueskyLimit is the maximum length of a Bluesky post, in graphemes as Bluesky
// counts them; runes are a close enough proxy for Latin text with URLs.
const BlueskyLimit = 300

// Speaker is one presenter with everything the copy needs about them.
type Speaker struct {
	cnd.Speaker
	Role  Role
	Links cnd.Links
}

// Draft is generated copy for one platform.
type Draft struct {
	Platform string
	Text     string
	// Notes are things the user should check before posting: guessed employers,
	// speakers with no handle to mention, a post that had to be shortened.
	Notes []string
}

// Runes is the post's length as a platform would count it.
func (d Draft) Runes() int { return len([]rune(d.Text)) }

// Input is everything needed to draft copy for a session.
type Input struct {
	Conference cnd.Conference
	Session    cnd.Session
	Speakers   []Speaker
}

// LinkedIn drafts a LinkedIn post.
//
// LinkedIn has no practical length limit for these, so the copy can afford a
// teaser from the abstract and a full list of speakers. Handles are given as
// profile URLs rather than "@name": LinkedIn only turns a mention into a link
// when it is picked from its own autocomplete, so a typed @name would post as
// plain text. The URLs are listed separately for the user to convert.
func LinkedIn(in Input) Draft {
	var d Draft
	d.Platform = "linkedin"
	s := in.Session

	var b strings.Builder
	b.WriteString(hook(in))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "%s\n", quoteTitle(s.Talk.Title))

	if teaser := cnd.FirstSentences(s.Talk.Abstract, 320); teaser != "" {
		fmt.Fprintf(&b, "\n%s\n", teaser)
	}

	fmt.Fprintf(&b, "\n📅 %s · %s\n", s.TimeRange(), dayLabel(in.Conference, s))
	if track := shortTrack(s.Track); track != "" {
		fmt.Fprintf(&b, "📍 %s\n", track)
	}
	if url := speakerURL(in); url != "" {
		fmt.Fprintf(&b, "\n%s\n", url)
	}

	if tags := hashtags(in); tags != "" {
		fmt.Fprintf(&b, "\n%s\n", tags)
	}
	d.Text = strings.TrimRight(b.String(), "\n")
	d.Notes = notes(in, "linkedin")
	return d
}

// Bluesky drafts a Bluesky post.
//
// The 300-character limit is the binding constraint, so the copy is assembled
// shortest-first: the essential sentence, then the handles, then the link, and
// the teaser only if it still fits. Trimming afterwards would risk cutting the
// link, which is the one part that must survive.
func Bluesky(in Input) Draft {
	var d Draft
	d.Platform = "bluesky"
	s := in.Session

	url := speakerURL(in)
	mentions := blueskyMentions(in)

	// The post is built as head + optional middle + tail. The tail holds the
	// mentions and the link, which are the parts that must never be cut, so
	// they are reserved up front rather than trimmed off the end.
	head := hook(in) + "\n\n" + quoteTitle(s.Talk.Title)
	tail := ""
	if mentions != "" {
		tail += "\n\n" + mentions
	}
	if url != "" {
		tail += "\n" + url
	}

	slot := fmt.Sprintf("\n\n%s · %s", s.TimeRange(), dayLabel(in.Conference, s))
	fits := func(parts ...string) bool {
		return len([]rune(strings.Join(parts, ""))) <= BlueskyLimit
	}

	// If the hook and title alone cannot coexist with the tail, shorten the
	// HEAD rather than letting the whole post be trimmed from the end. trimTo
	// cuts at a word boundary from the right, and the title is the rightmost
	// part of the head, so this loses title words and keeps the link — the
	// opposite of what trimming the finished post would do.
	if !fits(head, tail) {
		if room := BlueskyLimit - len([]rune(tail)); room > 0 {
			head = trimTo(head, room)
		}
	}

	// Optional sections accumulate in display order. The teaser is sized
	// against the room left once the slot line is ALSO accounted for, so that
	// adding the teaser cannot squeeze the slot out — composing the teaser
	// variant from scratch previously dropped it.
	mid := ""
	if room := BlueskyLimit - len([]rune(head+slot+tail)) - 2; room > 60 {
		if teaser := cnd.FirstSentences(s.Talk.Abstract, room); teaser != "" {
			if fits(head, "\n\n", teaser, slot, tail) {
				mid = "\n\n" + teaser
			}
		}
	}
	if fits(head, mid, slot, tail) {
		mid += slot
	}
	text := head + mid + tail

	d.Notes = notes(in, "bluesky")
	if n := len([]rune(text)); n > BlueskyLimit {
		// Only reachable when the title alone is enormous; trim the title
		// rather than the link.
		text = trimTo(text, BlueskyLimit)
		d.Notes = append(d.Notes, fmt.Sprintf("post was %d chars and had to be shortened to %d", n, BlueskyLimit))
	}
	d.Text = text
	return d
}

// hook is the opening sentence, naming the speakers and their employers.
func hook(in Input) string {
	names := make([]string, 0, len(in.Speakers))
	for _, sp := range in.Speakers {
		name := sp.Name
		if sp.Role.Employer != "" {
			name += " (" + sp.Role.Employer + ")"
		}
		names = append(names, name)
	}
	who := joinAnd(names)
	if who == "" {
		who = "One of our speakers"
	}

	verb := "is speaking"
	if len(in.Speakers) > 1 {
		verb = "are speaking"
	}
	if strings.HasPrefix(in.Session.Talk.Format, "workshop") {
		verb = "is running a workshop"
		if len(in.Speakers) > 1 {
			verb = "are running a workshop"
		}
	}
	return fmt.Sprintf("%s %s at %s 🎤", who, verb, in.Conference.Title)
}

// quoteTitle wraps a talk title in quotation marks, unless it already carries
// its own.
func quoteTitle(title string) string {
	if title == "" {
		return ""
	}
	if strings.HasPrefix(title, "“") || strings.HasPrefix(title, "\"") {
		return title
	}
	return "“" + title + "”"
}

// dayLabel names the conference day, e.g. "Monday 26 October".
func dayLabel(conf cnd.Conference, s cnd.Session) string {
	if d := parseDate(s.Date); d != nil {
		return d.Format("Monday 2 January")
	}
	return conf.DateRange()
}

// speakerURL is the single most useful link for a promo: a talk has no page of
// its own, so the first speaker's profile is used, which lists their session.
func speakerURL(in Input) string {
	for _, sp := range in.Speakers {
		if u := in.Conference.SpeakerURL(sp.Speaker); u != "" {
			return u
		}
	}
	return in.Conference.URL() + "/program"
}

// blueskyMentions builds "@handle" mentions, which Bluesky resolves from plain
// text — unlike LinkedIn.
func blueskyMentions(in Input) string {
	var out []string
	for _, sp := range in.Speakers {
		if sp.Links.Bluesky != "" {
			out = append(out, "@"+sp.Links.Bluesky)
		}
	}
	return strings.Join(out, " ")
}

// hashtags are the conference tag plus the talk's topics, which are curated
// upstream and so make better tags than anything derived from the title.
func hashtags(in Input) string {
	tags := []string{"#CloudNativeDaysNorway", "#CloudNative"}
	seen := map[string]bool{}
	for _, t := range tags {
		seen[strings.ToLower(t)] = true
	}
	for _, topic := range in.Session.Talk.Topics {
		tag := "#" + hashtagify(topic)
		if len(tag) > 1 && !seen[strings.ToLower(tag)] {
			seen[strings.ToLower(tag)] = true
			tags = append(tags, tag)
		}
	}
	if len(tags) > 5 {
		tags = tags[:5]
	}
	return strings.Join(tags, " ")
}

// hashtagify reduces a topic name to a CamelCase tag.
func hashtagify(s string) string {
	var b strings.Builder
	for _, word := range strings.FieldsFunc(s, func(r rune) bool {
		return !isAlnum(r)
	}) {
		r := []rune(word)
		b.WriteString(strings.ToUpper(string(r[0])))
		if len(r) > 1 {
			b.WriteString(string(r[1:]))
		}
	}
	return b.String()
}

// notes collects things the user should verify before posting.
func notes(in Input, platform string) []string {
	var out []string
	for _, sp := range in.Speakers {
		if sp.Role.Guessed && sp.Role.Employer != "" {
			out = append(out, fmt.Sprintf("employer for %s guessed as %q from %q — check it",
				sp.Name, sp.Role.Employer, sp.Title))
		}
		if sp.Role.Employer == "" {
			out = append(out, fmt.Sprintf("no employer known for %s", sp.Name))
		}
		switch platform {
		case "bluesky":
			if sp.Links.Bluesky == "" {
				out = append(out, fmt.Sprintf("no Bluesky handle for %s", sp.Name))
			}
		case "linkedin":
			if sp.Links.LinkedIn == "" {
				out = append(out, fmt.Sprintf("no LinkedIn profile for %s", sp.Name))
			}
		}
	}
	return out
}

// Mentions lists the profile URLs for a draft, for the user to turn into real
// mentions by hand.
func Mentions(in Input) []string {
	var out []string
	for _, sp := range in.Speakers {
		var parts []string
		if sp.Links.LinkedIn != "" {
			parts = append(parts, sp.Links.LinkedIn)
		}
		if sp.Links.Bluesky != "" {
			parts = append(parts, "@"+sp.Links.Bluesky)
		}
		if sp.Links.X != "" {
			parts = append(parts, "x.com/"+sp.Links.X)
		}
		if len(parts) > 0 {
			out = append(out, fmt.Sprintf("%s: %s", sp.Name, strings.Join(parts, "  ")))
		}
	}
	return out
}
