// Package manifest stores promo corrections as Kubernetes-style YAML documents.
//
// The data the program page gives us is not quite enough to post from: there is
// no structured employer field, so `promo post` guesses one, and a few talk
// titles are too long to read on a card. Those corrections have to live
// somewhere durable and reviewable, which means a file in git rather than
// re-typed flags.
//
// The format is a multi-document manifest — apiVersion, kind, metadata.name,
// spec — because there are two kinds of correction (speaker and talk) and that
// shape gives each its own object without inventing nesting. apiVersion and
// kind are validated rather than ignored, so a typo is an error instead of a
// silently dropped override.
package manifest

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/vehagn/speaker-promos/internal/cnd"
	"github.com/vehagn/speaker-promos/internal/post"
	"gopkg.in/yaml.v3"
)

// DefaultPath is the manifest a command reads when none is given.
const DefaultPath = "promos.yaml"

// APIVersion is the only accepted apiVersion.
const APIVersion = "promo.cloudnativedays.no/v1alpha1"

// The kinds of object a manifest may contain.
const (
	KindSpeakerOverride = "SpeakerOverride"
	KindTalkOverride    = "TalkOverride"
	// KindTalkInfo is informational: `promo export` writes it into a bundle's
	// promo.yaml to record everything known about the talk. It is accepted when
	// a manifest is loaded and then ignored, so an exported bundle can be
	// passed straight back with --manifest without having to be edited down
	// first.
	KindTalkInfo = "TalkInfo"
)

// Metadata names the object an override applies to.
type Metadata struct {
	// Name is a speaker slug or a talk id, exactly as `promo list` prints it.
	Name string `yaml:"name"`
}

// SpeakerSpec corrects what is known about one speaker.
//
// These are precisely the fields `promo post` flags for checking: the employer
// and job it guessed out of free text, and the handles it scraped from a
// speaker page.
type SpeakerSpec struct {
	Employer string `yaml:"employer,omitempty"`
	Job      string `yaml:"job,omitempty"`
	// Image replaces the speaker's photo. Five of the 2026 speakers have none
	// and get a monogram instead, and a CMS photo is sometimes just bad.
	//
	// Either an http(s) URL or a path on disk; a relative path resolves against
	// the manifest's own directory, so a bundle can carry its own photo next to
	// the promo.yaml that names it.
	Image string `yaml:"image,omitempty"`
	Links Links  `yaml:"links,omitempty"`
}

// Links are a speaker's profiles, overriding anything scraped.
type Links struct {
	LinkedIn string `yaml:"linkedin,omitempty"`
	Bluesky  string `yaml:"bluesky,omitempty"`
	X        string `yaml:"x,omitempty"`
	GitHub   string `yaml:"github,omitempty"`
}

func (l Links) empty() bool { return l == Links{} }

func (s SpeakerSpec) empty() bool {
	return s.Employer == "" && s.Job == "" && s.Image == "" && s.Links.empty()
}

// TalkSpec overrides how a talk is presented.
type TalkSpec struct {
	// DisplayTitle replaces the title on the card and in the copy, for titles
	// too long to read at card size.
	DisplayTitle string `yaml:"displayTitle,omitempty"`
	// Hidden excludes the talk from bulk operations: a cancelled session, or
	// one whose promo has already gone out.
	Hidden bool `yaml:"hidden,omitempty"`
}

func (t TalkSpec) empty() bool { return t.DisplayTitle == "" && !t.Hidden }

// Set is a loaded manifest.
//
// A Set owns both the overrides and the file they came from, and every mutating
// method persists before returning — there is no unsaved state in this design,
// so there is nowhere for the two to drift apart. That also makes a Set safe to
// share between concurrent HTTP handlers, which is what the preview server
// does.
type Set struct {
	mu       sync.Mutex
	path     string
	speakers map[string]SpeakerSpec
	talks    map[string]TalkSpec
}

// New returns an empty Set that will save to path.
func New(path string) *Set {
	if path == "" {
		path = DefaultPath
	}
	return &Set{
		path:     path,
		speakers: map[string]SpeakerSpec{},
		talks:    map[string]TalkSpec{},
	}
}

// Path is the file this Set saves to.
func (s *Set) Path() string { return s.path }

// Load reads a manifest. A missing file yields an empty Set and no error: the
// path has a default and most runs start without one.
func Load(path string) (*Set, error) {
	set := New(path)
	f, err := os.Open(set.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return set, nil
		}
		return nil, fmt.Errorf("reading %s: %w", set.path, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	for i := 0; ; i++ {
		// Each document is decoded into a Node first so that every error can
		// name the line it is on. Hand-edited config is miserable precisely
		// when it fails without saying where.
		var node yaml.Node
		if err := dec.Decode(&node); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("parsing %s: %w", set.path, err)
		}
		if err := set.addDocument(&node); err != nil {
			return nil, fmt.Errorf("%s: %w", set.path, err)
		}
	}
	return set, nil
}

// addDocument validates one manifest document and records its override.
func (s *Set) addDocument(node *yaml.Node) error {
	// A document node wraps its single content node; an empty document (a
	// stray "---" or a trailing separator) has none.
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil
		}
		node = node.Content[0]
	}
	if node.Kind == 0 || node.Tag == "!!null" {
		return nil
	}

	var doc struct {
		APIVersion string    `yaml:"apiVersion"`
		Kind       string    `yaml:"kind"`
		Metadata   Metadata  `yaml:"metadata"`
		Spec       yaml.Node `yaml:"spec"`
	}
	if err := node.Decode(&doc); err != nil {
		return fmt.Errorf("line %d: %w", node.Line, err)
	}
	if err := checkFields(node, "apiVersion", "kind", "metadata", "spec"); err != nil {
		return err
	}

	if doc.APIVersion != APIVersion {
		return fmt.Errorf("line %d: unsupported apiVersion %q (want %q)", node.Line, doc.APIVersion, APIVersion)
	}
	if doc.Metadata.Name == "" {
		return fmt.Errorf("line %d: %s needs a metadata.name (a speaker slug or talk id)", node.Line, doc.Kind)
	}

	switch doc.Kind {
	case KindSpeakerOverride:
		if err := checkFields(&doc.Spec, "employer", "job", "image", "links"); err != nil {
			return err
		}
		var spec SpeakerSpec
		if err := doc.Spec.Decode(&spec); err != nil {
			return fmt.Errorf("line %d: %s/%s: %w", doc.Spec.Line, doc.Kind, doc.Metadata.Name, err)
		}
		if links := field(&doc.Spec, "links"); links != nil {
			if err := checkFields(links, "linkedin", "bluesky", "x", "github"); err != nil {
				return err
			}
		}
		if _, dup := s.speakers[doc.Metadata.Name]; dup {
			return fmt.Errorf("line %d: duplicate %s for %q", node.Line, doc.Kind, doc.Metadata.Name)
		}
		s.speakers[doc.Metadata.Name] = spec
	case KindTalkOverride:
		if err := checkFields(&doc.Spec, "displayTitle", "hidden"); err != nil {
			return err
		}
		var spec TalkSpec
		if err := doc.Spec.Decode(&spec); err != nil {
			return fmt.Errorf("line %d: %s/%s: %w", doc.Spec.Line, doc.Kind, doc.Metadata.Name, err)
		}
		if _, dup := s.talks[doc.Metadata.Name]; dup {
			return fmt.Errorf("line %d: duplicate %s for %q", node.Line, doc.Kind, doc.Metadata.Name)
		}
		s.talks[doc.Metadata.Name] = spec
	case KindTalkInfo:
		// Informational, and deliberately not field-checked: it is a record of
		// what an export contained, so it may grow fields that an older binary
		// has never heard of. Rejecting those would make bundles from a newer
		// version unloadable for no benefit.
	default:
		return fmt.Errorf("line %d: unknown kind %q (want %s, %s or %s)",
			node.Line, doc.Kind, KindSpeakerOverride, KindTalkOverride, KindTalkInfo)
	}
	return nil
}

// checkFields rejects keys a mapping does not recognise.
//
// yaml.v3 offers Decoder.KnownFields for this, but it cannot be used here: the
// spec is decoded from an already-parsed Node, and a strict stream decoder
// would reject the spec of the *other* kind. Checking the keys directly also
// means the error can point at the offending line rather than the document.
func checkFields(n *yaml.Node, allowed ...string) error {
	if n == nil || n.Kind == 0 || n.Tag == "!!null" {
		return nil
	}
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: expected a mapping with keys %s", n.Line, strings.Join(allowed, ", "))
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := n.Content[i]
		if !contains(allowed, key.Value) {
			return fmt.Errorf("line %d: unknown field %q (known fields: %s)",
				key.Line, key.Value, strings.Join(allowed, ", "))
		}
	}
	return nil
}

// field returns the value node for a key in a mapping, or nil.
func field(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// Speaker returns the override for a speaker slug.
func (s *Set) Speaker(slug string) (SpeakerSpec, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	spec, ok := s.speakers[slug]
	return spec, ok
}

// Talk returns the override for a talk id.
func (s *Set) Talk(id string) (TalkSpec, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	spec, ok := s.talks[id]
	return spec, ok
}

// Len reports how many overrides of each kind are loaded.
func (s *Set) Len() (speakers, talks int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.speakers), len(s.talks)
}

// SetSpeaker records a speaker override and saves the manifest.
//
// An override that sets nothing is deleted rather than written as an empty
// object, so clearing a field in the preview server leaves the manifest as
// clean as it was before the edit.
func (s *Set) SetSpeaker(slug string, spec SpeakerSpec) error {
	spec = trimSpeaker(spec)
	s.mu.Lock()
	defer s.mu.Unlock()
	if spec.empty() {
		delete(s.speakers, slug)
	} else {
		s.speakers[slug] = spec
	}
	return s.save()
}

// SetTalk records a talk override and saves the manifest.
func (s *Set) SetTalk(id string, spec TalkSpec) error {
	spec.DisplayTitle = strings.TrimSpace(spec.DisplayTitle)
	s.mu.Lock()
	defer s.mu.Unlock()
	if spec.empty() {
		delete(s.talks, id)
	} else {
		s.talks[id] = spec
	}
	return s.save()
}

func trimSpeaker(spec SpeakerSpec) SpeakerSpec {
	spec.Employer = strings.TrimSpace(spec.Employer)
	spec.Job = strings.TrimSpace(spec.Job)
	spec.Links.LinkedIn = strings.TrimSpace(spec.Links.LinkedIn)
	spec.Links.Bluesky = strings.TrimSpace(spec.Links.Bluesky)
	spec.Links.X = strings.TrimSpace(spec.Links.X)
	spec.Links.GitHub = strings.TrimSpace(spec.Links.GitHub)
	return spec
}

// Save writes the manifest to its path.
func (s *Set) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save()
}

const fileHeader = `# Promo overrides for Cloud Native Days.
#
# Written by "promo serve" and read by "promo export" and "promo post". Safe to
# edit by hand and meant to be committed: this is the record of every
# correction made to the guessed employers and the titles that were too long.
`

// save writes the manifest. The caller must hold s.mu.
func (s *Set) save() error {
	if s.path == "" {
		return errors.New("manifest has no path to save to")
	}

	var b strings.Builder
	b.WriteString(fileHeader)
	// A yaml.Encoder that is closed without having written a document errors,
	// and an empty manifest is a normal state: every override can be cleared.
	if docs := s.documents(); len(docs) > 0 {
		enc := yaml.NewEncoder(&b)
		enc.SetIndent(2)
		for _, doc := range docs {
			if err := enc.Encode(doc); err != nil {
				return fmt.Errorf("encoding %s: %w", s.path, err)
			}
		}
		if err := enc.Close(); err != nil {
			return fmt.Errorf("encoding %s: %w", s.path, err)
		}
	}

	// Written via a temporary file in the same directory so an interrupted
	// save cannot truncate a manifest that holds an afternoon of corrections.
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	return nil
}

// document is one object as it appears on disk.
type document struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       any      `yaml:"spec"`
}

// documents renders the whole Set, sorted by kind then name so that the file
// stays diff-stable: an edit to one speaker should show up as a change to one
// object, not a reshuffle of the file. The caller must hold s.mu.
func (s *Set) documents() []document {
	out := make([]document, 0, len(s.speakers)+len(s.talks))
	for _, name := range sortedKeys(s.speakers) {
		out = append(out, document{APIVersion, KindSpeakerOverride, Metadata{name}, s.speakers[name]})
	}
	for _, name := range sortedKeys(s.talks) {
		out = append(out, document{APIVersion, KindTalkOverride, Metadata{name}, s.talks[name]})
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Overrides exposes the speaker corrections in the form the post package
// already resolves against, rather than reimplementing that resolution here.
func (s *Set) Overrides() post.Overrides {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(post.Overrides, len(s.speakers))
	for slug, spec := range s.speakers {
		out[slug] = post.Override{
			Employer: spec.Employer,
			Job:      spec.Job,
			LinkedIn: spec.Links.LinkedIn,
			Bluesky:  spec.Links.Bluesky,
			X:        spec.Links.X,
			GitHub:   spec.Links.GitHub,
		}
	}
	return out
}

// Hidden reports whether a talk is excluded from bulk operations.
func (s *Set) Hidden(talkID string) bool {
	spec, ok := s.Talk(talkID)
	return ok && spec.Hidden
}

// Rewrite applies a session's overrides — both the talk's and its speakers'.
//
// Overrides are applied by rewriting the session before it reaches the renderer
// or the copy, which leaves both of those packages untouched and unaware that
// overrides exist.
//
// Speaker overrides are folded into cnd.Speaker.Title, which is what a card's
// role line renders. Without this, correcting an employer would change the
// social copy but not the graphic sitting next to it — and the whole reason to
// correct it is that the card says the wrong thing.
func (s *Set) Rewrite(sess cnd.Session) cnd.Session {
	if spec, ok := s.Talk(sess.Talk.ID); ok && spec.DisplayTitle != "" {
		sess.Talk.Title = spec.DisplayTitle
	}

	// The speaker slice is shared with the Program — a speaker appearing on two
	// talks has one backing array — so it must be cloned before any element is
	// touched. Writing in place would leak this session's overrides into every
	// other session that speaker appears in.
	var speakers []cnd.Speaker
	for i, sp := range sess.Talk.Speakers {
		spec, ok := s.Speaker(sp.Slug)
		if !ok {
			continue
		}
		title := spec.RoleTitle()
		image := s.resolveImage(spec.Image)
		changesTitle := title != "" && title != sp.Title
		changesImage := image != "" && image != sp.Image
		if !changesTitle && !changesImage {
			continue
		}
		if speakers == nil {
			speakers = slices.Clone(sess.Talk.Speakers)
		}
		if changesTitle {
			speakers[i].Title = title
		}
		if changesImage {
			speakers[i].Image = image
		}
	}
	if speakers != nil {
		sess.Talk.Speakers = speakers
	}
	return sess
}

// RoleTitle renders a speaker override as the free-text title a card shows,
// following the upstream convention ("Senior Platform Engineer at
// Vestbit"). It returns "" when the override says nothing about the role, so the
// upstream value is kept.
func (spec SpeakerSpec) RoleTitle() string {
	switch {
	case spec.Job != "" && spec.Employer != "":
		return spec.Job + " at " + spec.Employer
	case spec.Job != "":
		return spec.Job
	default:
		return spec.Employer
	}
}

// Apply rewrites every session and drops the hidden ones. It is what bulk
// selections (--all) use; an explicitly selected talk is rewritten but not
// filtered, since asking for a talk by name and being told it does not exist
// would be worse than rendering it.
func (s *Set) Apply(in []cnd.Session) []cnd.Session {
	out := make([]cnd.Session, 0, len(in))
	for _, sess := range in {
		if s.Hidden(sess.Talk.ID) {
			continue
		}
		out = append(out, s.Rewrite(sess))
	}
	return out
}

// resolveImage turns an override's image value into something a renderer can
// fetch.
//
// A relative path is resolved against the manifest's own directory rather than
// the process working directory, so a bundle that carries a photo next to its
// promo.yaml keeps working whatever directory the tool is run from.
func (s *Set) resolveImage(image string) string {
	image = strings.TrimSpace(image)
	if image == "" || cnd.IsRemoteImage(image) || filepath.IsAbs(image) {
		return image
	}
	if dir := filepath.Dir(s.path); dir != "" && dir != "." {
		return filepath.Join(dir, image)
	}
	return image
}
