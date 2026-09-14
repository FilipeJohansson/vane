package lsp

import (
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/filipejohansson/vane/internal/compiler"
)

type document struct {
	vaneLines []string // original vane source split by line
	goLines   []string // compiled go source split by line
	sourceMap *compiler.SourceMap
}

type docStore struct {
	mu         sync.RWMutex
	docs       map[string]*document // keyed by vane URI
	gens       map[string]int64     // vane URI -> generation, bumped on every set()
	goVersions map[string]int       // vane URI -> next version number for a synthetic message to the virtual _vane.go document

	// hover/goplsIn support the async keyed-{for} type resolution pass (see
	// keyedfor.go): hover resolves a range variable's type via gopls, and
	// goplsIn is where the resolved recompile's own synthetic didChange is
	// sent. Both nil until Serve wires them up; a nil hover just means that
	// pass never runs (documents stay on the compat shape), never a crash.
	hover   *hoverClient
	goplsIn io.Writer
}

func newDocStore() *docStore {
	return &docStore{
		docs:       make(map[string]*document),
		gens:       make(map[string]int64),
		goVersions: make(map[string]int),
	}
}

// nextGoVersion returns the next version number to use for a synthetic
// didOpen/didChange sent to gopls for vaneURI's virtual _vane.go document.
// One shared, monotonically increasing counter per document, used by every
// writer (handleDidOpen, handleDidChange, the keyed-for resolution pass,
// and the file-watcher-triggered recompile) - not the editor's own .vane
// document version number, which a synthetic follow-up message
// (scheduleKeyedForResolve's own promoted recompile, in particular) can't
// safely guess an unused value from: a guess like "version+1" can collide
// with the version the editor assigns its own next real edit, sent around
// the same time from a different goroutine. LSP only requires versions to
// strictly increase per document, not to match any particular scheme, so
// owning this counter entirely removes that collision risk.
func (s *docStore) nextGoVersion(vaneURI string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	norm := normalizeFileURI(vaneURI)
	s.goVersions[norm]++
	return s.goVersions[norm]
}

func (s *docStore) compile(vaneURI, text string) (string, *compiler.SourceMap, error) {
	filename := filepath.Base(uriToPath(vaneURI))
	return compiler.CompileWithMap(text, filename)
}

func (s *docStore) set(vaneURI, vaneContent, goContent string, sm *compiler.SourceMap) {
	s.mu.Lock()
	defer s.mu.Unlock()
	norm := normalizeFileURI(vaneURI)
	// goLines must use the stripped content (no //line directives) because that is
	// what gopls receives. GoLine values in sm are also stripped-file indices.
	s.docs[norm] = &document{
		vaneLines: strings.Split(vaneContent, "\n"),
		goLines:   strings.Split(stripLineDirectives(goContent), "\n"),
		sourceMap: sm,
	}
	s.gens[norm]++
}

// generation returns vaneURI's current generation counter, bumped by every
// set() call. Used by the async keyed-{for} resolution pass to detect that a
// newer edit landed while a hover round-trip was in flight, so it can drop
// its now-stale result instead of clobbering the newer document.
func (s *docStore) generation(vaneURI string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.gens[normalizeFileURI(vaneURI)]
}

func (s *docStore) get(vaneURI string) (*document, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.docs[normalizeFileURI(vaneURI)]
	return d, ok
}

func (s *docStore) delete(vaneURI string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, normalizeFileURI(vaneURI))
}

// getByNorm finds a document by its normalized vane URI (uppercase drive letter,
// decoded %3A). Used for response position translation where gopls returns
// normalized URIs (file:///D:/...) but the store may be keyed with raw VS Code
// URIs (file:///d%3A/...).
func (s *docStore) getByNorm(normalizedVaneURI string) (*document, bool) {
	return s.get(normalizedVaneURI) // store already normalizes keys
}

// virtualURI maps a .vane URI to a normalized virtual .go URI for gopls.
// file:///path/to/Home.vane → file:///path/to/Home_vane.go
// The result is normalized (uppercase drive letter, %3A decoded) to match
// the format gopls uses for workspace roots.
func virtualURI(vaneURI string) string {
	base := vaneURI
	if strings.HasSuffix(vaneURI, ".vane") {
		base = strings.TrimSuffix(vaneURI, ".vane") + "_vane.go"
	}
	return normalizeFileURI(base)
}

// normalizeFileURI decodes %3A/%3a and uppercases the drive letter so the
// URI matches the format gopls uses (e.g. file:///D:/ not file:///d%3A/).
func normalizeFileURI(uri string) string {
	uri = strings.ReplaceAll(uri, `%3A`, `:`)
	uri = strings.ReplaceAll(uri, `%3a`, `:`)
	const prefix = "file:///"
	if strings.HasPrefix(uri, prefix) {
		rest := uri[len(prefix):]
		if len(rest) >= 2 && rest[1] == ':' && rest[0] >= 'a' && rest[0] <= 'z' {
			uri = prefix + string(rest[0]-32) + rest[1:]
		}
	}
	return uri
}

func isVaneURI(uri string) bool {
	return strings.HasSuffix(uri, ".vane")
}

func pathToFileURI(path string) string {
	path = filepath.ToSlash(path)
	if len(path) >= 2 && path[1] == ':' {
		return "file:///" + path
	}
	return "file://" + path
}

func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	p := u.Path
	// Windows: file:///D:/path → /D:/path, strip leading slash before drive letter.
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}
