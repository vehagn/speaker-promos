// Command promo generates speaker promo graphics and social copy for a Cloud
// Native Days conference, from the live program on the conference website.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/vehagn/speaker-promos/internal/cnd"
)

const usage = `promo — speaker promo graphics for Cloud Native Days

Usage:
  promo list [flags]                 index the program
  promo svg  [flags] <selector>...   render promo SVGs
  promo post [flags] <selector>      draft LinkedIn / Bluesky copy
  promo fonts install                install the brand fonts locally
  promo theme dump                   print the built-in theme as YAML

A <selector> picks talks by id prefix, speaker slug, or a substring of the
talk title, e.g. "gunvor-rønning" or "Nok nok nett". Use --all to
select every talk.

Run "promo <command> -h" for the flags of a command.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "promo:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "list":
		return cmdList(rest)
	case "svg":
		return cmdSVG(rest)
	case "post":
		return cmdPost(rest)
	case "fonts":
		return cmdFonts(rest)
	case "theme":
		return cmdTheme(rest)
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
	}
}

// commonFlags are the data-source flags every program-reading command shares.
type commonFlags struct {
	domain  string
	ttl     time.Duration
	noCache bool
}

func (c *commonFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&c.domain, "domain", cnd.DefaultDomain, "conference site to read the program from")
	fs.DurationVar(&c.ttl, "cache-ttl", 6*time.Hour, "how long a cached page stays fresh")
	fs.BoolVar(&c.noCache, "no-cache", false, "always re-fetch, ignoring the cache")
}

func (c *commonFlags) load() (*cnd.Program, error) {
	loader := cnd.NewLoader(c.domain, c.ttl)
	loader.Cache.Disabled = c.noCache
	return loader.Load()
}

// selectSessions resolves positional selectors, or every session with --all.
func selectSessions(p *cnd.Program, all bool, selectors []string) ([]cnd.Session, error) {
	if all {
		if len(selectors) > 0 {
			return nil, errors.New("--all takes no selectors")
		}
		return p.Sessions, nil
	}
	if len(selectors) == 0 {
		return nil, errors.New("give at least one selector, or --all")
	}

	seen := make(map[string]bool)
	var out []cnd.Session
	for _, sel := range selectors {
		hits := p.Find(sel)
		if len(hits) == 0 {
			return nil, fmt.Errorf("no talk matches %q", sel)
		}
		for _, h := range hits {
			if !seen[h.Talk.ID] {
				seen[h.Talk.ID] = true
				out = append(out, h)
			}
		}
	}
	return out, nil
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet("promo "+name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: %s\n\nFlags:\n",
			strings.TrimSpace("promo "+name+" [flags] "+positionalHint(name)))
		fs.PrintDefaults()
	}
	return fs
}

func positionalHint(cmd string) string {
	switch cmd {
	case "svg":
		return "<selector>..."
	case "post":
		return "<selector>"
	default:
		return ""
	}
}

// truncate shortens s to at most n runes for tabular output.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return strings.TrimRight(string(r[:n-1]), " ") + "…"
}

// parseFlags parses args allowing flags to appear after positional arguments.
//
// Go's flag package stops parsing at the first non-flag argument, so
// `promo svg gunvor-rønning --out promos/` would silently treat "--out" and
// "promos/" as selectors. That word order is the natural one and every other
// modern CLI accepts it, so the arguments are permuted first: flags (with their
// values) are hoisted ahead of the positionals.
func parseFlags(fs *flag.FlagSet, args []string) error {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]

		// A bare "--" ends flag parsing; everything after it is positional.
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' {
			positional = append(positional, a)
			continue
		}

		flags = append(flags, a)
		name, inlineValue := cutFlagValue(strings.TrimLeft(a, "-"))
		if inlineValue {
			continue
		}
		// A non-boolean flag takes the following argument as its value, so that
		// argument must travel with it rather than becoming a positional.
		if f := fs.Lookup(name); f != nil && !isBoolFlag(f) && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return fs.Parse(append(flags, positional...))
}

// cutFlagValue splits "name=value" and reports whether a value was inline.
func cutFlagValue(s string) (string, bool) {
	if name, _, ok := strings.Cut(s, "="); ok {
		return name, true
	}
	return s, false
}

func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}
