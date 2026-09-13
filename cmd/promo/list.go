package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/vehagn/speaker-promos/internal/cnd"
)

func cmdList(args []string) error {
	fs := newFlagSet("list")
	var common commonFlags
	common.register(fs)
	day := fs.Int("day", 0, "show only this conference day (1-based)")
	speaker := fs.String("speaker", "", "show only talks by this speaker (slug or name substring)")
	asJSON := fs.Bool("json", false, "emit JSON instead of a table")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	program, err := common.load()
	if err != nil {
		return err
	}

	sessions := program.Sessions
	if *day > 0 {
		sessions = filter(sessions, func(s cnd.Session) bool { return s.Day == *day })
	}
	if *speaker != "" {
		q := strings.ToLower(*speaker)
		sessions = filter(sessions, func(s cnd.Session) bool {
			for _, sp := range s.Talk.Speakers {
				if strings.EqualFold(sp.Slug, q) || strings.Contains(strings.ToLower(sp.Name), q) {
					return true
				}
			}
			return false
		})
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(sessions)
	}

	c := program.Conference
	fmt.Printf("%s — %s, %s\n", c.Title, c.DateRange(), c.Location())
	fmt.Printf("%d talks, %d speakers\n\n", len(sessions), len(cnd.Program{Sessions: sessions}.Speakers()))

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "DAY\tTIME\tTRACK\tSPEAKERS\tTALK\tSELECTOR")
	for _, s := range sessions {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n",
			s.Day,
			s.TimeRange(),
			truncate(shortTrack(s.Track), 18),
			truncate(s.SpeakerNames("and"), 28),
			truncate(s.Talk.Title, 44),
			primarySelector(s),
		)
	}
	return w.Flush()
}

// shortTrack drops the "Track N: " prefix the website uses, which is noise once
// the column is labelled.
func shortTrack(t string) string {
	if _, rest, ok := strings.Cut(t, ": "); ok {
		return rest
	}
	return t
}

// primarySelector is the shortest thing the user can copy to select a talk
// again: its first speaker's slug, falling back to a talk id prefix.
func primarySelector(s cnd.Session) string {
	for _, sp := range s.Talk.Speakers {
		if sp.Slug != "" {
			return sp.Slug
		}
	}
	if len(s.Talk.ID) >= 8 {
		return s.Talk.ID[:8]
	}
	return s.Talk.ID
}

func filter(in []cnd.Session, keep func(cnd.Session) bool) []cnd.Session {
	var out []cnd.Session
	for _, s := range in {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out
}
