package post

import (
	"strings"
	"time"
)

// joinAnd joins names as prose, using the given conjunction.
func joinAnd(items []string, and string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " " + and + " " + items[len(items)-1]
	}
}

func shortTrack(t string) string {
	if _, rest, ok := strings.Cut(t, ": "); ok {
		return rest
	}
	return t
}

func parseDate(s string) *time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return nil
	}
	return &d
}

func isAlnum(r rune) bool {
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

// trimTo shortens text to at most n runes, cutting at a word boundary.
func trimTo(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		// Nothing useful fits; an ellipsis alone is one rune.
		if n == 1 {
			return "…"
		}
		return ""
	}
	cut := string(r[:n-1])
	if i := strings.LastIndexAny(cut, " \n"); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " \n") + "…"
}
