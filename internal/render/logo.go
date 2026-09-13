package render

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	svgOpenRe    = regexp.MustCompile(`(?is)<svg\b[^>]*>`)
	viewBoxRe    = regexp.MustCompile(`(?is)\bviewBox\s*=\s*["']([^"']+)["']`)
	dimensionRe  = regexp.MustCompile(`(?is)\b(width|height)\s*=\s*["']([0-9.]+)(?:px)?["']`)
	xmlDeclRe    = regexp.MustCompile(`(?is)<\?xml.*?\?>`)
	docTypeRe    = regexp.MustCompile(`(?is)<!DOCTYPE[^>]*>`)
	commentRe    = regexp.MustCompile(`(?is)<!--.*?-->`)
	scriptTagRe  = regexp.MustCompile(`(?is)<script\b.*?</script\s*>`)
	foreignObjRe = regexp.MustCompile(`(?is)<foreignObject\b.*?</foreignObject\s*>`)
)

// svgBody extracts the contents and viewBox of an SVG document so it can be
// nested inside a card.
//
// A source without a viewBox is given one from its width/height attributes;
// without either it cannot be scaled predictably and is rejected rather than
// rendered at an arbitrary size.
//
// script and foreignObject elements are stripped. The logo markup comes from a
// CMS field, and while a conference logo is not a realistic attack vector, a
// generated SVG that may be opened in a browser should not carry executable
// content it does not need.
func svgBody(doc string) (inner, viewBox string, ok bool) {
	doc = strings.TrimSpace(doc)
	if doc == "" {
		return "", "", false
	}
	doc = xmlDeclRe.ReplaceAllString(doc, "")
	doc = docTypeRe.ReplaceAllString(doc, "")
	doc = commentRe.ReplaceAllString(doc, "")

	open := svgOpenRe.FindStringIndex(doc)
	if open == nil {
		return "", "", false
	}
	openTag := doc[open[0]:open[1]]

	close := strings.LastIndex(strings.ToLower(doc), "</svg")
	if close < open[1] {
		return "", "", false
	}
	inner = doc[open[1]:close]
	inner = scriptTagRe.ReplaceAllString(inner, "")
	inner = foreignObjRe.ReplaceAllString(inner, "")

	if m := viewBoxRe.FindStringSubmatch(openTag); m != nil {
		return inner, strings.TrimSpace(m[1]), true
	}

	dims := map[string]string{}
	for _, m := range dimensionRe.FindAllStringSubmatch(openTag, -1) {
		dims[strings.ToLower(m[1])] = m[2]
	}
	if w, okW := dims["width"]; okW {
		if h, okH := dims["height"]; okH {
			return inner, fmt.Sprintf("0 0 %s %s", w, h), true
		}
	}
	return "", "", false
}

// svgAspect returns the width/height ratio of an SVG document, and whether it
// could be determined. Cards size the logo by width and need the ratio to
// reserve the right amount of height for it.
func svgAspect(doc string) (float64, bool) {
	_, viewBox, ok := svgBody(doc)
	if !ok {
		return 0, false
	}
	var minX, minY, w, h float64
	if _, err := fmt.Sscanf(strings.NewReplacer(",", " ").Replace(viewBox), "%g %g %g %g", &minX, &minY, &w, &h); err != nil {
		return 0, false
	}
	if w <= 0 || h <= 0 {
		return 0, false
	}
	return w / h, true
}
