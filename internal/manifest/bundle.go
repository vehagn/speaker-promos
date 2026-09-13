package manifest

import (
	"fmt"
	"strings"

	"github.com/vehagn/speaker-promos/internal/cnd"
	"github.com/vehagn/speaker-promos/internal/post"
	"gopkg.in/yaml.v3"
)

// bundleHeader introduces a per-talk manifest written into an export folder.
const bundleHeader = `# Promo overrides for one talk, as used for the cards in this folder.
#
# Values are the ones that actually produced them: a correction from the project
# manifest where one exists, otherwise the guess the tool made. Edit this file
# and pass it with --manifest to feed the corrections back.
`

// ForSession renders the manifest documents for one talk as YAML.
//
// Unlike Set.Save, every field is written even when it holds a guess rather
// than a correction. An export folder is a record of what produced its cards,
// and a manifest that omitted the guessed employer would not say what the card
// actually claimed — nor give you a line to edit.
//
// The talk's own document is emitted only when it carries something: a display
// title or a hidden flag. There is nothing to pre-fill it with otherwise, and an
// empty object would just be noise.
func (s *Set) ForSession(sess cnd.Session, overrides post.Overrides) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var docs []document
	for _, sp := range sess.Talk.Speakers {
		if sp.Slug == "" {
			// Overrides are keyed by slug; a speaker without one cannot be
			// addressed, so there is no document to write.
			continue
		}
		spec := s.speakers[sp.Slug]
		if spec.Employer == "" && spec.Job == "" {
			// Pre-fill from what the card used, so the file is editable rather
			// than blank. RoleFor resolves the override first and falls back to
			// the heuristic, which is exactly what the card rendered.
			role := overrides.RoleFor(sp)
			spec.Employer, spec.Job = role.Employer, role.Job
		}
		docs = append(docs, document{APIVersion, KindSpeakerOverride, Metadata{sp.Slug}, spec})
	}

	if spec, ok := s.talks[sess.Talk.ID]; ok && !spec.empty() {
		docs = append(docs, document{APIVersion, KindTalkOverride, Metadata{sess.Talk.ID}, spec})
	}

	var b strings.Builder
	b.WriteString(bundleHeader)
	if len(docs) > 0 {
		enc := yaml.NewEncoder(&b)
		enc.SetIndent(2)
		for _, doc := range docs {
			if err := enc.Encode(doc); err != nil {
				return nil, fmt.Errorf("encoding manifest for %q: %w", sess.Talk.Title, err)
			}
		}
		if err := enc.Close(); err != nil {
			return nil, fmt.Errorf("encoding manifest for %q: %w", sess.Talk.Title, err)
		}
	}
	return []byte(b.String()), nil
}
