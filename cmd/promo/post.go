package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/vehagn/speaker-promos/internal/cnd"
	"github.com/vehagn/speaker-promos/internal/post"
	"gopkg.in/yaml.v3"
)

func cmdPost(args []string) error {
	fs := newFlagSet("post")
	var common commonFlags
	common.register(fs)
	all := fs.Bool("all", false, "draft copy for every talk")
	platform := fs.String("platform", "both", "linkedin, bluesky, or both")
	speakersFile := fs.String("speakers", "speakers.yaml", "overrides for guessed employers and handles")
	noLinks := fs.Bool("no-links", false, "skip fetching speaker pages for social handles")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	program, err := common.load()
	if err != nil {
		return err
	}
	sessions, err := selectSessions(program, *all, fs.Args())
	if err != nil {
		return err
	}
	overrides, err := loadOverrides(*speakersFile)
	if err != nil {
		return err
	}

	loader := cnd.NewLoader(common.domain, common.ttl)
	loader.Cache.Disabled = common.noCache

	for i, s := range sessions {
		if i > 0 {
			fmt.Println(strings.Repeat("─", 72))
		}
		in := post.Input{Conference: program.Conference, Session: s}
		for _, sp := range s.Talk.Speakers {
			var links cnd.Links
			if !*noLinks {
				// A speaker page is ~3 MB and this is only for optional
				// @-mention suggestions, so a failure is reported and skipped
				// rather than aborting the draft.
				got, err := loader.SpeakerLinks(sp)
				if err != nil {
					fmt.Fprintf(os.Stderr, "note: could not read %s's profile page: %v\n", sp.Name, err)
				}
				links = got
			}
			in.Speakers = append(in.Speakers, post.Speaker{
				Speaker: sp,
				Role:    overrides.RoleFor(sp),
				Links:   overrides.LinksFor(sp, links),
			})
		}

		var drafts []post.Draft
		switch *platform {
		case "linkedin":
			drafts = []post.Draft{post.LinkedIn(in)}
		case "bluesky":
			drafts = []post.Draft{post.Bluesky(in)}
		case "both", "all":
			drafts = []post.Draft{post.LinkedIn(in), post.Bluesky(in)}
		default:
			return fmt.Errorf("unknown platform %q (want linkedin, bluesky or both)", *platform)
		}

		for _, d := range drafts {
			printDraft(d)
		}
		if mentions := post.Mentions(in); len(mentions) > 0 {
			fmt.Println("\nprofiles to mention:")
			for _, m := range mentions {
				fmt.Println("  " + m)
			}
		}
	}
	return nil
}

func printDraft(d post.Draft) {
	limit := ""
	if d.Platform == "bluesky" {
		limit = fmt.Sprintf("/%d", post.BlueskyLimit)
		if d.Runes() > post.BlueskyLimit {
			limit += " OVER LIMIT"
		}
	}
	fmt.Printf("\n── %s (%d%s chars) ──\n\n%s\n", d.Platform, d.Runes(), limit, d.Text)
	if len(d.Notes) > 0 {
		fmt.Println("\ncheck before posting:")
		for _, n := range d.Notes {
			fmt.Println("  - " + n)
		}
	}
}

// loadOverrides reads the speaker correction file. A missing file is not an
// error: the flag has a default path and most runs will not have one.
func loadOverrides(path string) (post.Overrides, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return post.Overrides{}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var o post.Overrides
	if err := yaml.Unmarshal(b, &o); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return o, nil
}
