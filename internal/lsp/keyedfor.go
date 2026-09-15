package lsp

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/filipejohansson/vane/internal/compiler"
	"github.com/filipejohansson/vane/internal/typeresolve"
)

// This file resolves a keyed {for}'s range value variable's concrete type
// via a textDocument/hover request to the LSP's own already-running gopls,
// instead of always leaving the editor on the compatibility-shape codegen.
// docStore.compile (see documents.go) stays synchronous and naive - always
// compat shape, always fast, always valid - and scheduleKeyedForResolve
// runs afterward, asynchronously, to upgrade the document once (if)
// resolution succeeds.

// keyedForCandidate is one named-value for-range loop found in a file's
// already-compiled compat-shape Go text, with everything needed to resolve
// and then apply a type hint for it.
type keyedForCandidate struct {
	hoverLine, hoverCol int // 0-based, UTF-16 - stripped-go position to hover at
	forOffset           int // byte offset of the "for" keyword in the .vane source - compiler.ForTypeHint.Offset
}

// findKeyedForCandidates parses strippedGo - the exact text gopls has open
// for this file, no //line directives - and returns one candidate per
// named-value for-range loop. A plain unkeyed {for} is a harmless candidate
// too (resolving it costs one more hover call; CompileWithMapAndHints simply
// never looks up its hint, since only a keyed {for} calls hintForOffset) -
// not worth the risk of a second, looser parse of the .vane source itself
// just to filter it out first. strippedGo failing to parse (shouldn't
// happen - it's the same text gopls just built successfully) yields no
// candidates: the safe fallback is staying on the compat shape already
// published, never a crash.
func findKeyedForCandidates(strippedGo, vaneText string, sm *compiler.SourceMap) []keyedForCandidate {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", strippedGo, 0)
	if err != nil {
		return nil
	}
	goLines := strings.Split(strippedGo, "\n")

	var out []keyedForCandidate
	typeresolve.WalkNamedRangeValues(f, func(rs *ast.RangeStmt, id *ast.Ident) {
		// The stripped-go line differs from the .vane line (extra //line
		// comment lines above it were removed), but the column of id within
		// its own physical "for ... range ..." line is identical either
		// way - the line directive is a whole separate line, never spliced
		// into the for-header line itself. Only the line needs translating.
		forPos := fset.Position(rs.For)
		vaneLine, _, ok := sm.GoToVane(forPos.Line-1, 0)
		if !ok {
			return
		}
		offset, ok := compiler.OffsetOfForOnLine(vaneText, vaneLine+1)
		if !ok {
			return
		}

		idPos := fset.Position(id.Pos())
		hoverCol := idPos.Column - 1
		if idPos.Line-1 < len(goLines) {
			hoverCol = utf16Column(goLines[idPos.Line-1], hoverCol)
		}
		out = append(out, keyedForCandidate{
			hoverLine: idPos.Line - 1,
			hoverCol:  hoverCol,
			forOffset: offset,
		})
	})
	return out
}

// utf16Column converts byteCol, a 0-based byte offset into line (as
// go/token's Position.Column - 1 gives), to the equivalent 0-based UTF-16
// code unit offset the LSP protocol's hover position actually needs. Equal
// to byteCol only when everything before it is ASCII - a non-ASCII
// identifier or comment earlier on the same line (e.g. a range variable named
// with an accented letter, or a "//" comment containing one) shifts them
// apart, since UTF-8 multi-byte sequences and UTF-16 surrogate pairs don't
// count the same way.
func utf16Column(line string, byteCol int) int {
	if byteCol > len(line) {
		byteCol = len(line)
	}
	return len(utf16.Encode([]rune(line[:byteCol])))
}

// parseHoverVarType parses a gopls hover value for a "var <name> <Type>"
// declaration and returns the resolved type string. gopls's hover
// contentFormat is negotiated once, session-wide, by whatever capabilities
// the real editor declared at initialize - a server-initiated request here
// gets formatted the same way as an ordinary editor-triggered hover, so this
// must handle both: bare "var t Todo" (plaintext), and markdown's fenced
// "```go\nvar t Todo\n```" (VS Code's own client always advertises markdown
// support, so this is the common real case, not the exception). Scans line
// by line, skipping blank lines and ``` fence markers, and requires the
// first real content line to start with "var " - anything else (a hover
// miss, or a differently-shaped response from an untested gopls version)
// returns false, so the caller falls back to the compat shape rather than
// feed a bogus type into codegen.
func parseHoverVarType(value string) (string, bool) {
	for _, raw := range strings.Split(value, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "```") {
			continue
		}
		const prefix = "var "
		if !strings.HasPrefix(line, prefix) {
			return "", false
		}
		rest := line[len(prefix):]
		sp := strings.IndexByte(rest, ' ')
		if sp < 0 {
			return "", false
		}
		typ := strings.TrimSpace(rest[sp+1:])
		if typ == "" {
			return "", false
		}
		return typ, true
	}
	return "", false
}

// syncWriter serializes writes to w. gopls's stdin is one stream; once the
// keyed-for resolution goroutine can write to it (a hover request, then
// later a synthetic didChange) alongside the main editor-proxy loop, an
// unsynchronized interleaving of two messages' bytes would corrupt LSP
// framing. WriteMessage writes a whole message in one Write call
// specifically so locking around each Write here gives real per-message
// atomicity.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// hoverClient issues server-initiated textDocument/hover requests to gopls
// and matches responses back to their caller - distinct from the
// editor-initiated request/response tracking in server.go's own
// pendingMethods, which only ever expects a response for a request the
// editor itself sent. An hoverClient request's id is prefixed so Serve's
// gopls->editor pipe can recognize and consume the response itself,
// dropping it rather than forwarding a reply the editor never asked for.
type hoverClient struct {
	out io.Writer

	mu      sync.Mutex
	nextID  int64
	pending map[string]chan hoverReply
}

type hoverReply struct {
	value string
	ok    bool
}

func newHoverClient(out io.Writer) *hoverClient {
	return &hoverClient{out: out, pending: make(map[string]chan hoverReply)}
}

const hoverIDPrefix = "vane-hover-"

// take delivers result to id's waiting caller, if id is one of this
// client's own outstanding requests. Returns false for any other id (an
// ordinary editor-originated response), which the caller must then forward
// normally.
func (c *hoverClient) take(id string, result json.RawMessage) bool {
	c.mu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if !ok {
		return false
	}

	var reply hoverReply
	var hover struct {
		Contents struct {
			Value string `json:"value"`
		} `json:"contents"`
	}
	if len(result) > 0 && string(result) != "null" && json.Unmarshal(result, &hover) == nil {
		reply.value = hover.Contents.Value
		reply.ok = reply.value != ""
	}
	ch <- reply
	return true
}

// hover requests gopls's hover at (uri, line, col) - 0-based line, UTF-16
// col - and blocks for a parseable "var <name> <Type>" result or a 2s
// timeout, whichever comes first. uri must already be known to gopls (this
// always runs after the synthetic didOpen/didChange that compiled the
// document it's resolving).
func (c *hoverClient) hover(uri string, line, col int) (string, bool) {
	c.mu.Lock()
	c.nextID++
	id := fmt.Sprintf("%s%d", hoverIDPrefix, c.nextID)
	ch := make(chan hoverReply, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	req, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "textDocument/hover",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"position":     map[string]any{"line": line, "character": col},
		},
	})
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return "", false
	}
	if err := WriteMessage(c.out, Message(req)); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return "", false
	}

	select {
	case reply := <-ch:
		if !reply.ok {
			fmt.Fprintf(os.Stderr, "[vane lsp] keyed-for hover: empty/null hover result at %s %d:%d\n", uri, line, col)
			return "", false
		}
		typ, ok := parseHoverVarType(reply.value)
		if !ok {
			// Real diagnostic, not noise: this only fires when a hover
			// response didn't match any expected shape, which either means
			// a gopls response format this hasn't been tested against, or a
			// genuine bug in parseHoverVarType - worth seeing the raw value
			// rather than silently falling back with no trace.
			fmt.Fprintf(os.Stderr, "[vane lsp] keyed-for hover: unparseable response: %q\n", reply.value)
		}
		return typ, ok
	case <-time.After(2 * time.Second):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		fmt.Fprintf(os.Stderr, "[vane lsp] keyed-for hover: timed out waiting for gopls at %s %d:%d\n", uri, line, col)
		return "", false
	}
}

// scheduleKeyedForResolve runs the hover-based resolution pass for uri's
// just-published compat-shape compile, asynchronously - see this file's own
// top comment. text/strippedGo/sm are exactly what produced the compile
// already published for uri (the caller's own synchronous compile - see
// handleDidOpen, handleDidChange, and handleWatchedFilesChange's own
// compileAndWrite, which all call this the same way), so hint offsets and
// the hover positions line up with what gopls has open right now.
//
// A no-op (returns immediately, nothing scheduled) when s.hover is nil
// (Serve hasn't wired resolution up - e.g. in tests that construct a
// docStore directly), when text has no "key=" at all (main.go's own
// maybeKeyed heuristic, applied here per-file same as there - cheap, and a
// file with no keyed {for} at all has nothing this pass could ever change),
// or when the compat-shape Go has no range loop with a named value variable.
func (s *docStore) scheduleKeyedForResolve(uri, text, strippedGo string, sm *compiler.SourceMap) {
	if s.hover == nil || !strings.Contains(text, "key=") {
		return
	}
	candidates := findKeyedForCandidates(strippedGo, text, sm)
	if len(candidates) == 0 {
		return
	}
	genBefore := s.generation(uri)
	fmt.Fprintf(os.Stderr, "[vane lsp] keyed-for: resolving %d candidate(s) for %s\n", len(candidates), uri)

	go func() {
		vURI := virtualURI(uri)
		var hints []compiler.ForTypeHint
		for _, c := range candidates {
			typ, ok := s.hover.hover(vURI, c.hoverLine, c.hoverCol)
			if !ok {
				continue
			}
			hints = append(hints, compiler.ForTypeHint{Offset: c.forOffset, Type: typ})
		}
		if len(hints) == 0 {
			fmt.Fprintf(os.Stderr, "[vane lsp] keyed-for: 0/%d hints resolved for %s, staying on compat shape\n", len(candidates), uri)
			return
		}

		filename := filepath.Base(uriToPath(uri))
		goContent, sm2, err := compiler.CompileWithMapAndHints(text, filename, hints)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[vane lsp] keyed-for: recompile with %d resolved hint(s) failed for %s: %v\n", len(hints), uri, err)
			return
		}
		fmt.Fprintf(os.Stderr, "[vane lsp] keyed-for: %d/%d hints resolved for %s, promoting\n", len(hints), len(candidates), uri)

		// A newer edit already landed while hovering was in flight - that
		// edit's own synchronous compile is what's published now (and it
		// will trigger its own resolve pass against current content).
		// Publishing this stale result would clobber it with an outdated
		// document, checked and applied under one lock so no later set()
		// can land between the check and the write.
		s.mu.Lock()
		norm := normalizeFileURI(uri)
		if s.gens[norm] != genBefore {
			s.mu.Unlock()
			fmt.Fprintf(os.Stderr, "[vane lsp] keyed-for: dropping stale resolve for %s (gen %d -> %d)\n", uri, genBefore, s.gens[norm])
			return
		}
		s.docs[norm] = &document{
			vaneLines: strings.Split(text, "\n"),
			goLines:   strings.Split(stripLineDirectives(goContent), "\n"),
			sourceMap: sm2,
		}
		s.gens[norm]++
		s.mu.Unlock()

		writeToDisk(uri, goContent)
		synthetic := buildDidChange(vURI, stripLineDirectives(goContent), s.nextGoVersion(uri))
		if s.goplsIn != nil {
			if err := WriteMessage(s.goplsIn, synthetic); err != nil {
				fmt.Fprintf(os.Stderr, "[vane lsp] send resolved keyed-for didChange error: %v\n", err)
			}
		}
	}()
}
