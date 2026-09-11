package lsp

import (
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
	mu   sync.RWMutex
	docs map[string]*document // keyed by vane URI
}

func newDocStore() *docStore {
	return &docStore{docs: make(map[string]*document)}
}

func (s *docStore) compile(vaneURI, text string) (string, *compiler.SourceMap, error) {
	filename := filepath.Base(uriToPath(vaneURI))
	return compiler.CompileWithMap(text, filename)
}

func (s *docStore) set(vaneURI, vaneContent, goContent string, sm *compiler.SourceMap) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// goLines must use the stripped content (no //line directives) because that is
	// what gopls receives. GoLine values in sm are also stripped-file indices.
	s.docs[normalizeFileURI(vaneURI)] = &document{
		vaneLines: strings.Split(vaneContent, "\n"),
		goLines:   strings.Split(stripLineDirectives(goContent), "\n"),
		sourceMap: sm,
	}
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
