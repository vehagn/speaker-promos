package cnd

import (
	"encoding/json"
	"strings"
)

// portableBlock is the subset of Sanity's Portable Text format that talk
// abstracts use: blocks of spans, optionally as list items.
type portableBlock struct {
	Type     string `json:"_type"`
	Style    string `json:"style"`
	ListItem string `json:"listItem"`
	Children []struct {
		Type string `json:"_type"`
		Text string `json:"text"`
	} `json:"children"`
}

// flattenPortableText renders a Portable Text array as plain text.
//
// Abstracts are used for social copy and for optional teaser lines on a card,
// neither of which can carry marks, so emphasis and links are dropped and only
// the text survives. Blocks become paragraphs; list items keep a bullet so an
// abstract written as a list still reads as one.
//
// Input is `any` because it arrives already decoded from the RSC payload, where
// a `$ref` may have replaced the whole array with a plain string.
func flattenPortableText(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		// A resolved reference, or a plain-string description.
		return strings.TrimSpace(t)
	}

	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	var blocks []portableBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}

	var paras []string
	for _, b := range blocks {
		if b.Type != "" && b.Type != "block" {
			// Images, code blocks and other embeds have no text form here.
			continue
		}
		var sb strings.Builder
		for _, c := range b.Children {
			if c.Type == "" || c.Type == "span" {
				sb.WriteString(c.Text)
			}
		}
		text := strings.TrimSpace(sb.String())
		if text == "" {
			continue
		}
		if b.ListItem != "" {
			text = "• " + text
		}
		paras = append(paras, text)
	}
	return strings.Join(paras, "\n\n")
}

// FirstSentences returns the leading sentences of text, up to maxRunes,
// preferring to cut on a sentence boundary and falling back to a word boundary.
// It is used for teaser copy where a mid-word cut would look broken.
func FirstSentences(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(strings.ReplaceAll(text, "\n", " ")), " ")
	if len([]rune(text)) <= maxRunes {
		return text
	}
	runes := []rune(text)[:maxRunes]

	// Prefer the last sentence end, but only if it keeps a useful amount.
	if i := lastIndexAny(runes, ".!?"); i > maxRunes/3 {
		return strings.TrimSpace(string(runes[:i+1]))
	}
	if i := lastIndexAny(runes, " "); i > 0 {
		return strings.TrimSpace(string(runes[:i])) + "…"
	}
	return strings.TrimSpace(string(runes)) + "…"
}

func lastIndexAny(runes []rune, chars string) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if strings.ContainsRune(chars, runes[i]) {
			return i
		}
	}
	return -1
}
