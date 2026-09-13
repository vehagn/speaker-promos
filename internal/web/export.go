package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// handleExport writes every visible card to the output directory.
//
// This is the same work `promo svg --all` does, exposed so that a review
// session can end without switching back to the terminal. Hidden talks are
// skipped, matching the CLI.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	size := s.sizeParam(r)
	dir := s.opts.OutDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.fail(w, fmt.Errorf("creating %s: %w", dir, err))
		return
	}

	s.mu.Lock()
	sessions := s.opts.Set.Apply(s.opts.Program.Sessions)
	skipped := len(s.opts.Program.Sessions) - len(sessions)
	s.mu.Unlock()

	written := 0
	for _, sess := range sessions {
		res, err := s.renderer.Card(s.opts.Program.Conference, sess, size)
		if err != nil {
			s.fail(w, fmt.Errorf("rendering %q: %w", sess.Talk.Title, err))
			return
		}
		path := filepath.Join(dir, sess.FileStem()+".svg")
		if err := os.WriteFile(path, []byte(res.SVG), 0o644); err != nil {
			s.fail(w, fmt.Errorf("writing %s: %w", path, err))
			return
		}
		written++
	}

	msg := fmt.Sprintf("wrote %d cards", written)
	if skipped > 0 {
		msg += fmt.Sprintf(" (%d hidden)", skipped)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, htmlEscape(msg))
}

// htmlEscape is used for the few plain-text responses that land in the DOM.
func htmlEscape(s string) string {
	var out []byte
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			out = append(out, "&lt;"...)
		case '>':
			out = append(out, "&gt;"...)
		case '&':
			out = append(out, "&amp;"...)
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}
