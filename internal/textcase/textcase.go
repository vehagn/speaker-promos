// Package textcase tidies capitalisation in text that came from a form.
//
// Both transforms are deliberately crude. They are wired to a button next to
// the field they change, so a wrong result is visible immediately and corrected
// by typing — which is a better trade than a clever rule nobody can predict.
package textcase

import (
	"strings"
	"unicode"

	"github.com/vehagn/speaker-promos/internal/lang"
)

// minorWords stay lowercase in an English title unless they lead it, end it, or
// follow a colon.
//
// The list follows Chicago: articles, coordinating conjunctions, and
// prepositions regardless of length, so "with" and "from" are lowercase. AP
// would capitalise those. Either is defensible and the point of the button is
// to pick ONE, so a title already set in the other style will be changed — that
// is normalisation, not damage.
var minorWords = map[string]bool{
	"a": true, "an": true, "and": true, "as": true, "at": true, "but": true,
	"by": true, "for": true, "from": true, "in": true, "into": true, "nor": true,
	"of": true, "off": true, "on": true, "onto": true, "or": true, "over": true,
	"per": true, "the": true, "to": true, "up": true, "via": true, "with": true,
	"vs": true, "yet": true, "so": true,
}

// acronyms are the ones this audience writes in lower case and means in
// capitals. A short, explicit list beats a clever rule: without it the button
// turns "ai" into "Ai", which is a new error rather than a correction.
var acronyms = map[string]string{
	"ai": "AI", "ml": "ML", "llm": "LLM", "api": "API", "apis": "APIs",
	"cli": "CLI", "ci": "CI", "cd": "CD", "sre": "SRE", "cncf": "CNCF",
	"yaml": "YAML", "json": "JSON", "http": "HTTP", "https": "HTTPS",
	"sql": "SQL", "aws": "AWS", "gcp": "GCP", "ui": "UI", "ux": "UX",
	"os": "OS", "vm": "VM", "vms": "VMs", "dns": "DNS", "tls": "TLS",
	"gpu": "GPU", "gpus": "GPUs", "cpu": "CPU", "crd": "CRD", "crds": "CRDs",
	"rbac": "RBAC", "iam": "IAM", "sbom": "SBOM", "otel": "OTel",
	"ebpf": "eBPF", "devops": "DevOps", "gitops": "GitOps", "mlops": "MLOps",
}

// Title fixes a talk title's capitalisation for the language it is written in.
//
// English gets title case. Norwegian gets SENTENCE case, because that is the
// Norwegian convention — title-casing "Akkurat nok nett" would be an error, not
// a correction, and a third of the 2026 program is Norwegian.
func Title(title string, l lang.Language) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	if l.Resolve(title, "") == lang.Norwegian {
		return sentenceCase(title)
	}
	return titleCase(title)
}

// titleCase capitalises each word, leaving minor words lowercase in the middle.
func titleCase(title string) string {
	words := strings.Fields(title)
	out := make([]string, 0, len(words))
	for i, word := range words {
		// A word after a colon starts a subtitle, so it is capitalised even if
		// it is a minor word: "Git Outta Here: The Gitless GitOps Story".
		leads := i == 0 || (i > 0 && endsClause(words[i-1]))
		last := i == len(words)-1

		switch {
		case keepAsWritten(word):
			// A deliberately-cased name — OpenTelemetry, eBPF, k6, YAML.
			// Recasing these is the one thing a title fixer must not do.
			out = append(out, word)
		case expand(word) != "":
			out = append(out, expand(word))
		case !leads && !last && minorWords[strings.ToLower(trimPunct(word))]:
			out = append(out, strings.ToLower(word))
		default:
			out = append(out, upperFirst(word))
		}
	}
	return strings.Join(out, " ")
}

// expand returns the conventional capitalisation of a known acronym, keeping
// any punctuation around it, or "" when the word contains none.
//
// Hyphenated compounds are expanded segment by segment, because Norwegian
// builds them constantly: "ai-drevet" should read "AI-drevet", and only the
// first half is an acronym.
func expand(word string) string {
	bare := trimPunct(word)
	if bare == "" {
		return ""
	}
	if full, ok := acronyms[strings.ToLower(bare)]; ok {
		return strings.Replace(word, bare, full, 1)
	}
	if !strings.Contains(bare, "-") {
		return ""
	}

	parts := strings.Split(bare, "-")
	changed := false
	for i, part := range parts {
		if full, ok := acronyms[strings.ToLower(part)]; ok && part != full {
			parts[i] = full
			changed = true
		}
	}
	if !changed {
		return ""
	}
	return strings.Replace(word, bare, strings.Join(parts, "-"), 1)
}

// endsClause reports a word that closes a clause, after which the next word is
// capitalised regardless of what it is.
func endsClause(word string) bool {
	return strings.HasSuffix(word, ":") || strings.HasSuffix(word, ".") ||
		strings.HasSuffix(word, "?") || strings.HasSuffix(word, "!") ||
		strings.HasSuffix(word, "—") || strings.HasSuffix(word, "–")
}

// sentenceCase capitalises the first letter and the first letter after a
// sentence-ending colon, and touches nothing else.
//
// It never lowercases: doing so would turn "Kubernetes" into "kubernetes", and
// there is no way to tell a proper noun from an over-capitalised word without
// a dictionary.
func sentenceCase(title string) string {
	words := strings.Fields(title)
	capitaliseNext := true
	for i, word := range words {
		switch {
		case keepAsWritten(word):
		case expand(word) != "":
			words[i] = expand(word)
		case capitaliseNext:
			words[i] = upperFirst(word)
		}
		capitaliseNext = endsClause(word)
	}
	return strings.Join(words, " ")
}

// particles stay lowercase in a name: "Jeroen van Erp", "Andrés de la Cruz".
// Capitalising those is a change for the worse, and they are a short list.
var particles = map[string]bool{
	"van": true, "von": true, "de": true, "del": true, "della": true, "di": true,
	"da": true, "dos": true, "das": true, "la": true, "le": true, "du": true,
	"den": true, "der": true, "ter": true, "bin": true, "bint": true, "al": true,
	"af": true, "av": true, "ibn": true, "mac": true, "vander": true,
}

// Name capitalises the first letter of each part of a person's name.
//
// This is for the speakers who typed their own name in lower case — "leffen",
// "lars". Words already carrying a capital past their first letter are left
// alone, so "McDonald" and "O'Brien" survive, and the lowercase particles above
// stay lowercase unless they lead the name.
func Name(name string) string {
	words := strings.Fields(name)
	for i, word := range words {
		switch {
		case i != 0 && particles[strings.ToLower(word)]:
			// Checked before the shout rule so that "VAN ERP" still yields
			// "van Erp" rather than "Van Erp".
			words[i] = strings.ToLower(word)
		case shouted(word):
			// An all-capitals word in a NAME is someone shouting their
			// surname — "Abdel SGHIOUAR". In a title the same word would more
			// likely be an acronym, which is why this rule lives here and not
			// in keepAsWritten.
			words[i] = titleWord(word)
		case keepAsWritten(word):
			// Deliberate casing: McDonald, O'Brien, DuPont.
		default:
			words[i] = upperFirst(word)
		}
	}
	return strings.Join(words, " ")
}

// shouted reports a word written entirely in capitals, with at least two
// letters so that an initial ("J.") is left alone.
func shouted(word string) bool {
	letters := 0
	for _, r := range word {
		if !unicode.IsLetter(r) {
			continue
		}
		if !unicode.IsUpper(r) {
			return false
		}
		letters++
	}
	return letters >= 2
}

// titleWord lowercases a word and capitalises its first letter and the letter
// after each hyphen or apostrophe, so "ANNE-MARIE" gives "Anne-Marie" and
// "O'BRIEN" gives "O'Brien".
func titleWord(word string) string {
	runes := []rune(strings.ToLower(word))
	upperNext := true
	for i, r := range runes {
		switch {
		case upperNext && unicode.IsLetter(r):
			runes[i] = unicode.ToUpper(r)
			upperNext = false
		case r == '-' || r == '\'' || r == '’':
			upperNext = true
		}
	}
	return string(runes)
}

// keepAsWritten reports a word whose spelling someone clearly chose.
//
// That is any word carrying a capital past its first letter (OpenTelemetry,
// eBPF, iOS) and any word containing a digit (k6, k8s, s3, IPv6) — capitalising
// "k6" to "K6" is a new error, and a digit is a reliable sign of a product name
// rather than a word.
func keepAsWritten(word string) bool {
	for i, r := range word {
		if i > 0 && unicode.IsUpper(r) {
			return true
		}
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// upperFirst capitalises the first letter, skipping any leading punctuation so
// that a quoted or parenthesised word is still fixed.
func upperFirst(word string) string {
	runes := []rune(word)
	for i, r := range runes {
		if unicode.IsLetter(r) {
			runes[i] = unicode.ToUpper(r)
			return string(runes)
		}
	}
	return word
}

// trimPunct strips surrounding punctuation for a minor-word lookup, so that
// "of:" and "(of)" are recognised.
func trimPunct(word string) string {
	return strings.Trim(word, `.,:;!?"'“”«»()[]`)
}
