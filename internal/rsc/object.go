package rsc

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FindObject locates a JSON object embedded in a flight stream.
//
// Flight rows are not pure JSON — a props row looks like
// `50:["$","$L54",null,{"schedules":[…]}]` — and the object of interest is
// nested at an unknown depth inside prose and element markers. Rather than
// modelling the whole element tree, this searches for a distinctive substring
// (`needle`) and then walks *outward* to the enclosing object.
//
// Because the nearest enclosing `{` is not always the one wanted (the
// conference object, for instance, sits several levels above the first date
// string found inside it), candidate opening braces are tried from the innermost
// outward and the first object that both parses and carries every key in
// `require` wins. Passing `require` is therefore what disambiguates; with no
// requirements the innermost enclosing object is returned.
func FindObject(flight, needle string, require ...string) (json.RawMessage, error) {
	at := strings.Index(flight, needle)
	if at < 0 {
		return nil, fmt.Errorf("marker %q not found in flight payload", needle)
	}

	for search := at; search >= 0; {
		open := strings.LastIndexByte(flight[:search], '{')
		if open < 0 {
			break
		}
		search = open
		raw, ok := balanced(flight, open)
		if !ok || open+len(raw) <= at {
			// Unbalanced, or closes before the needle — so it does not contain it.
			continue
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &obj) != nil {
			continue
		}
		if hasAll(obj, require) {
			return json.RawMessage(raw), nil
		}
	}
	return nil, fmt.Errorf("no JSON object around %q with keys %v", needle, require)
}

func hasAll(obj map[string]json.RawMessage, keys []string) bool {
	for _, k := range keys {
		if _, ok := obj[k]; !ok {
			return false
		}
	}
	return true
}

// balanced returns the substring starting at the brace at `open` and ending at
// its matching close brace. Braces inside JSON string literals are skipped, and
// backslash escapes are honoured so that a `\"` does not end a string early.
func balanced(s string, open int) (string, bool) {
	depth := 0
	inStr := false
	esc := false
	for i := open; i < len(s); i++ {
		c := s[i]
		switch {
		case esc:
			esc = false
		case inStr && c == '\\':
			esc = true
		case inStr && c == '"':
			inStr = false
		case inStr:
			// Ordinary character inside a string literal.
		case c == '"':
			inStr = true
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return s[open : i+1], true
			}
		}
	}
	return "", false
}
