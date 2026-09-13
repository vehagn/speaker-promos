// Package rsc parses the React Server Components "flight" payload that Next.js
// embeds in server-rendered HTML.
//
// The Cloud Native Days website exposes no public API for its program (every
// tRPC schedule/speaker procedure is admin-only, and the Sanity dataset refuses
// anonymous reads), but the public program page ships the whole thing in its RSC
// payload. Recovering it takes three steps, one per exported function here:
//
//  1. Chunks. The payload arrives as a series of
//     `self.__next_f.push([1,"<json string>"])` calls whose decoded contents
//     concatenate into one stream.
//  2. Rows. The stream is newline-separated `<id>:<payload>` rows. A payload
//     beginning with `T<hexlen>,` is raw text of that many *bytes*; anything
//     else is JSON.
//  3. References. To avoid repeating itself, the payload replaces strings with
//     `"$<id>"` pointers into the row table. Leaving them unresolved would put a
//     literal "$55" in a rendered promo, so Resolve walks decoded JSON and
//     substitutes them.
package rsc

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// pushRe matches the `self.__next_f.push([1,"…"])` calls carrying the payload.
// The capture is a JSON string literal, quotes included, so it can be handed
// straight to json.Unmarshal to undo the escaping.
var pushRe = regexp.MustCompile(`(?s)self\.__next_f\.push\(\[1,("(?:[^"\\]|\\.)*")\]\)`)

// Chunks extracts and concatenates the flight stream from a page's HTML.
func Chunks(html string) (string, error) {
	matches := pushRe.FindAllStringSubmatch(html, -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("no self.__next_f.push chunks found: not a Next.js RSC page?")
	}
	var b strings.Builder
	for i, m := range matches {
		var s string
		if err := json.Unmarshal([]byte(m[1]), &s); err != nil {
			return "", fmt.Errorf("decoding chunk %d of %d: %w", i+1, len(matches), err)
		}
		b.WriteString(s)
	}
	return b.String(), nil
}

// rowRe matches a row header at the start of a line: a hex id and a colon.
//
// The type tag that may follow (T, I, H, E, …) is deliberately NOT captured
// here. Only `T` changes how the row is delimited, so only `T` is consumed —
// swallowing any letter would silently drop the first character of, say, an
// `I[…]` module row.
var rowRe = regexp.MustCompile(`^([0-9a-f]+):`)

// textRe matches the `T<hexlen>,` prefix of a length-delimited text row.
var textRe = regexp.MustCompile(`^T([0-9a-f]+),`)

// Rows splits a flight stream into its id-keyed rows.
//
// Text rows (`<id>:T<hexlen>,<text>`) are length-prefixed and may themselves
// contain newlines, so the stream cannot simply be split on "\n" — the declared
// byte length has to be honoured to find where each one ends. Lengths are in
// bytes while Go indexes strings by byte, so the two agree; slicing by rune
// count here would corrupt every row after the payload's first non-ASCII text
// (Norwegian prose, in practice).
func Rows(flight string) map[string]string {
	rows := make(map[string]string)
	for pos := 0; pos < len(flight); {
		m := rowRe.FindStringSubmatchIndex(flight[pos:])
		if m == nil {
			// Not a row header — skip to the next line and resynchronise.
			nl := strings.IndexByte(flight[pos:], '\n')
			if nl < 0 {
				break
			}
			pos += nl + 1
			continue
		}
		id := flight[pos+m[2] : pos+m[3]]
		body := pos + m[1]

		if t := textRe.FindStringSubmatch(flight[body:]); t != nil {
			n, err := strconv.ParseInt(t[1], 16, 64)
			if err == nil && n >= 0 {
				start := body + len(t[0])
				end := min(start+int(n), len(flight))
				rows[id] = flight[start:end]
				pos = end
				for pos < len(flight) && flight[pos] == '\n' {
					pos++
				}
				continue
			}
			// Length prefix present but unusable; fall through to line mode so
			// the row is still captured rather than dropped.
		}

		nl := strings.IndexByte(flight[body:], '\n')
		if nl < 0 {
			rows[id] = flight[body:]
			break
		}
		rows[id] = flight[body : body+nl]
		pos = body + nl + 1
	}
	return rows
}

// Resolve replaces `$<id>` reference strings in a decoded JSON value with the
// rows they point at, recursing through objects and arrays.
//
// Only references naming a known row are substituted. The payload uses the same
// `$`-prefix for things that are not row pointers — `$undefined`, and the
// `["$","div",…]` element markers — so an unknown reference is deliberately left
// verbatim rather than blanked out.
func Resolve(v any, rows map[string]string) any {
	switch t := v.(type) {
	case string:
		if id, ok := strings.CutPrefix(t, "$"); ok {
			if row, found := rows[id]; found {
				return row
			}
		}
		return t
	case []any:
		for i, e := range t {
			t[i] = Resolve(e, rows)
		}
		return t
	case map[string]any:
		for k, e := range t {
			t[k] = Resolve(e, rows)
		}
		return t
	default:
		return v
	}
}
