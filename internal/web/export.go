package web

import (
	"fmt"
	"net/http"
	"strings"
)

// handleExport writes a bundle per visible talk into the output directory.
//
// It goes through the same internal/export code as `promo export`, so a bundle
// produced from the browser and one produced from the terminal are identical.
// Hidden talks are skipped, matching the CLI.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	size := s.sizeParam(r)

	s.mu.Lock()
	sessions := s.opts.Set.Apply(s.opts.Program.Sessions)
	skipped := len(s.opts.Program.Sessions) - len(sessions)
	exporter := s.exporter([]string{size})
	s.mu.Unlock()

	talks, files := 0, 0
	var warnings []string
	for _, sess := range sessions {
		res, err := exporter.Write(s.opts.OutDir, sess)
		if err != nil {
			s.fail(w, err)
			return
		}
		talks++
		files += len(res.Files)
		warnings = append(warnings, res.Warnings...)
	}

	msg := fmt.Sprintf("wrote %d talks, %d files", talks, files)
	if skipped > 0 {
		msg += fmt.Sprintf(" (%d hidden)", skipped)
	}
	if !s.hasConverter {
		msg += " — SVG only, no rasteriser on PATH"
	}
	if n := len(warnings); n > 0 {
		msg += fmt.Sprintf(", %d warning(s)", n)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, htmlEscape(msg))
}

// htmlEscape is used for the few plain-text responses that land in the DOM.
func htmlEscape(s string) string {
	return strings.NewReplacer("<", "&lt;", ">", "&gt;", "&", "&amp;").Replace(s)
}
