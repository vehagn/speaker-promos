package layout

import "strings"

// Run is a stretch of text to be rendered with one font family.
type Run struct {
	Text string
	// Fallback is true when the primary font has no glyphs for this run and a
	// system family must render it.
	Fallback bool
}

// SplitRuns divides a line into runs by whether the font can render them.
//
// Talk titles contain emoji — the 2026 program has "Kan 🇳🇴 skyen kjøre på en
// brødrister?" — and no text font covers them. Emitting such a title as one <text>
// element leaves a renderer to substitute per-glyph, which browsers do but
// resvg does not, producing tofu. Splitting the line lets the emoji run carry an
// explicit colour-emoji family while the text keeps the brand face.
//
// Whitespace is attached to the surrounding primary run rather than being
// classified on its own, so a space between two words never becomes a separate
// element.
//
// Flag emoji are a pair of regional indicators and skin-tone and ZWJ sequences
// are several code points; because each of those code points is individually
// absent from a text font, they all land in the same fallback run and stay
// together, which is what lets the renderer compose them.
func SplitRuns(f *Font, line string) []Run {
	if line == "" {
		return nil
	}

	var runs []Run
	var current strings.Builder
	currentFallback := false
	started := false

	flush := func() {
		if current.Len() > 0 {
			runs = append(runs, Run{Text: current.String(), Fallback: currentFallback})
			current.Reset()
		}
	}

	for _, r := range line {
		// Spaces and joiners take the mode of whatever they follow, keeping
		// grapheme clusters and inter-word spaces intact.
		if isNeutral(r) && started {
			current.WriteRune(r)
			continue
		}
		fallback := !f.Has(r)
		if started && fallback != currentFallback {
			flush()
		}
		currentFallback = fallback
		started = true
		current.WriteRune(r)
	}
	flush()
	return runs
}

// isNeutral reports runes that should not themselves decide a run's font:
// whitespace, variation selectors, and the zero-width joiner used to build
// composite emoji.
func isNeutral(r rune) bool {
	switch {
	case r == ' ', r == '\t':
		return true
	case r == 0x200D: // zero-width joiner
		return true
	case r >= 0xFE00 && r <= 0xFE0F: // variation selectors
		return true
	default:
		return false
	}
}

// HasFallback reports whether any run needs the fallback family.
func HasFallback(runs []Run) bool {
	for _, r := range runs {
		if r.Fallback {
			return true
		}
	}
	return false
}
