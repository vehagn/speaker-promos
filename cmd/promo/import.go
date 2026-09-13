package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vehagn/speaker-promos/internal/cnd"
	"github.com/vehagn/speaker-promos/internal/manifest"
	"github.com/vehagn/speaker-promos/internal/post"
)

func cmdImport(args []string) error {
	fs := newFlagSet("import")
	var common commonFlags
	common.register(fs)
	var manifestPath manifestFlag
	manifestPath.register(fs)
	dryRun := fs.Bool("dry-run", false, "report what would change without writing")
	confirm := fs.Bool("confirm-guesses", false,
		"also import the pre-filled guesses, accepting them as correct")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	paths, err := findManifests(fs.Args())
	if err != nil {
		return err
	}
	fmt.Printf("importing %d file(s)\n", len(paths))

	target, err := manifestPath.load()
	if err != nil {
		return err
	}
	// The program supplies the baseline: what the tool would say with no
	// overrides at all. Without it an import cannot tell an edit from a
	// pre-filled guess coming back unchanged.
	program, err := common.load()
	if err != nil {
		return err
	}
	opts := manifest.ImportOptions{
		SpeakerBaseline: speakerBaseline(program),
		TalkBaseline:    talkBaseline(program),
		ConfirmGuesses:  *confirm,
		DryRun:          *dryRun,
	}

	var all []manifest.Change
	for _, path := range paths {
		src, err := manifest.Load(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		changes, err := target.ImportFrom(src, opts)
		if err != nil {
			return err
		}
		if len(changes) > 0 {
			rel := relativeTo(path)
			fmt.Printf("\n%s\n", rel)
			manifest.SortChanges(changes)
			for _, c := range changes {
				fmt.Println("  " + c.String())
			}
		}
		all = append(all, changes...)
	}

	fmt.Println()
	switch {
	case len(all) == 0 && *confirm:
		fmt.Println("nothing to import: every value already matches the manifest")
	case len(all) == 0:
		fmt.Printf("nothing to import: every value matches either the manifest or the\n" +
			"tool's own guess. Use --confirm-guesses to accept the guesses as correct.\n")
	case *dryRun:
		fmt.Printf("%d change(s) — nothing written (--dry-run)\n", len(all))
	default:
		fmt.Printf("%d change(s) written to %s\n", len(all), target.Path())
	}
	return nil
}

// findManifests expands the positional arguments into manifest files.
//
// A directory is searched rather than rejected, because the thing you have
// after an export is `out/` — one folder per talk — and asking for every
// promo.yaml inside it by hand would be absurd.
func findManifests(args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, errors.New("give a promo.yaml, a bundle folder, or an export directory")
	}

	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		if !seen[abs] {
			seen[abs] = true
			out = append(out, p)
		}
	}

	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", arg, err)
		}
		if !info.IsDir() {
			add(arg)
			continue
		}
		var found int
		err = filepath.WalkDir(arg, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && d.Name() == "promo.yaml" {
				add(path)
				found++
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("searching %s: %w", arg, err)
		}
		if found == 0 {
			return nil, fmt.Errorf("no promo.yaml found under %s", arg)
		}
	}
	sort.Strings(out)
	return out, nil
}

// speakerBaseline reports what the tool would produce for a speaker with no
// override: their upstream name, and the employer and job guessed out of the
// free-text profile title.
func speakerBaseline(program *cnd.Program) func(string) (manifest.SpeakerSpec, bool) {
	base := map[string]manifest.SpeakerSpec{}
	for _, sp := range program.Speakers() {
		if sp.Slug == "" {
			continue
		}
		role := post.ParseRole(sp.Title)
		base[sp.Slug] = manifest.SpeakerSpec{
			Name:     sp.Name,
			Employer: role.Employer,
			Job:      role.Job,
		}
	}
	return func(slug string) (manifest.SpeakerSpec, bool) {
		spec, ok := base[slug]
		return spec, ok
	}
}

// talkBaseline reports a talk's submitted title.
func talkBaseline(program *cnd.Program) func(string) (string, bool) {
	base := map[string]string{}
	for _, s := range program.Sessions {
		base[s.Talk.ID] = s.Talk.Title
	}
	return func(id string) (string, bool) {
		title, ok := base[id]
		return title, ok
	}
}

// relativeTo shortens a path against the working directory for reporting.
func relativeTo(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(wd, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}
