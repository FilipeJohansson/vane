package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/filipejohansson/vane/internal/compiler"
)

func TestParseHoverVarType(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		want   string
		wantOK bool
	}{
		{"same package type", "var t Todo", "Todo", true},
		{"cross-package qualified type", "var t otherpkg.Row", "otherpkg.Row", true},
		{"no var prefix", "func Foo()", "", false},
		{"empty", "", "", false},
		{"var with no type", "var t ", "", false},
		{"just var, no name or type", "var ", "", false},
		// gopls's hover contentFormat is negotiated session-wide by whatever
		// the real editor's initialize declared - VS Code's own client
		// always offers markdown, and gopls uses it when offered, so a
		// fenced response is the common real case, not an edge case.
		{"markdown-fenced, same package", "```go\nvar t Todo\n```", "Todo", true},
		{"markdown-fenced, cross package", "```go\nvar t otherpkg.Row\n```", "otherpkg.Row", true},
		{"markdown-fenced with trailing blank line", "```go\nvar t Todo\n```\n", "Todo", true},
		{"markdown fence, no var line", "```go\nfunc Foo()\n```", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseHoverVarType(tc.value)
			if ok != tc.wantOK || got != tc.want {
				t.Errorf("parseHoverVarType(%q) = (%q, %v), want (%q, %v)", tc.value, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

const keyedForFixtureSrc = "package main\n" +
	"import \"syscall/js\"\n" +
	"func F() js.Value {\n" +
	"\treturn (\n" +
	"\t\t<ul>{for _, t := range items {\n" +
	"\t\t\t<li key={t.ID}>{t.Text}</li>\n" +
	"\t\t}}</ul>\n" +
	"\t)\n" +
	"}\n"

// TestFindKeyedForCandidates_KeyedFor confirms a real keyed {for} block's
// range value variable is located correctly in both coordinate spaces: the
// stripped-go hover position (what gopls's document actually has open) and
// the .vane byte offset (compiler.ForTypeHint.Offset's own contract).
func TestFindKeyedForCandidates_KeyedFor(t *testing.T) {
	goSrc, sm, err := compiler.CompileWithMap(keyedForFixtureSrc, "Test.vane")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	strippedGo := stripLineDirectives(goSrc)

	candidates := findKeyedForCandidates(strippedGo, keyedForFixtureSrc, sm)
	if len(candidates) != 1 {
		t.Fatalf("want 1 candidate, got %d: %+v", len(candidates), candidates)
	}
	c := candidates[0]

	goLines := strings.Split(strippedGo, "\n")
	if c.hoverLine < 0 || c.hoverLine >= len(goLines) {
		t.Fatalf("hoverLine %d out of range (%d lines)", c.hoverLine, len(goLines))
	}
	line := goLines[c.hoverLine]
	if c.hoverCol < 0 || c.hoverCol >= len(line) || line[c.hoverCol] != 't' {
		t.Fatalf("hover position (%d,%d) doesn't land on the range value variable: line=%q", c.hoverLine, c.hoverCol, line)
	}

	if c.forOffset < 0 || c.forOffset+3 > len(keyedForFixtureSrc) || keyedForFixtureSrc[c.forOffset:c.forOffset+3] != "for" {
		t.Fatalf("forOffset %d doesn't point at \"for\" in the .vane source", c.forOffset)
	}
}

// TestFindKeyedForCandidates_NoRangeLoop confirms a file with no for-range
// loop at all yields no candidates - the common case for most .vane files,
// which should cost nothing.
func TestFindKeyedForCandidates_NoRangeLoop(t *testing.T) {
	src := "package main\nimport \"syscall/js\"\nfunc F() js.Value {\n\treturn <div>hi</div>\n}\n"
	goSrc, sm, err := compiler.CompileWithMap(src, "Test.vane")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	candidates := findKeyedForCandidates(stripLineDirectives(goSrc), src, sm)
	if len(candidates) != 0 {
		t.Fatalf("want 0 candidates, got %d: %+v", len(candidates), candidates)
	}
}

// hoverPipe wires a docStore's hover client to an in-process fake gopls: a
// background goroutine reads every message the store writes (hover
// requests, and the final resolved didChange) and answers each hover with
// respond. Returns a channel closed once the didChange that follows a
// successful resolve arrives, so a test can wait for the whole async pass
// to finish without sleeping/polling.
func hoverPipe(t *testing.T, store *docStore, respond func() (contents string, ok bool)) (didChange <-chan struct{}) {
	t.Helper()
	pr, pw := io.Pipe()
	store.hover = newHoverClient(pw)
	store.goplsIn = pw

	done := make(chan struct{})
	go func() {
		r := bufio.NewReader(pr)
		for {
			msg, err := ReadMessage(r)
			if err != nil {
				return
			}
			var env struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if json.Unmarshal(msg, &env) != nil {
				continue
			}
			switch env.Method {
			case "textDocument/hover":
				var idStr string
				_ = json.Unmarshal(env.ID, &idStr)
				var result json.RawMessage
				if value, ok := respond(); ok {
					b, _ := json.Marshal(map[string]any{
						"contents": map[string]any{"kind": "plaintext", "value": value},
					})
					result = b
				} else {
					result = json.RawMessage("null")
				}
				store.hover.take(idStr, result)
			case "textDocument/didChange":
				close(done)
				return
			}
		}
	}()
	return done
}

// TestScheduleKeyedForResolve_PromotesToKeyedShape is the end-to-end case:
// a fake gopls resolves the range value variable's type via hover, and the
// document store ends up holding the real, item-level-skip keyed codegen
// shape - not the compat shape handleDidOpen/handleDidChange's own
// synchronous compile always produces first.
func TestScheduleKeyedForResolve_PromotesToKeyedShape(t *testing.T) {
	const vaneURI = "file:///D:/proj/Test.vane"
	goSrc, sm, err := compiler.CompileWithMap(keyedForFixtureSrc, "Test.vane")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	strippedGo := stripLineDirectives(goSrc)

	store := newDocStore()
	store.set(vaneURI, keyedForFixtureSrc, goSrc, sm)
	done := hoverPipe(t, store, func() (string, bool) { return "var t Todo", true })

	store.scheduleKeyedForResolve(vaneURI, keyedForFixtureSrc, strippedGo, sm)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the resolved didChange")
	}

	doc, ok := store.get(vaneURI)
	if !ok {
		t.Fatal("document vanished from the store")
	}
	got := strings.Join(doc.goLines, "\n")
	if !strings.Contains(got, "func() []Todo { return items },") {
		t.Errorf("doc wasn't promoted to the keyed shape (missing itemsFn):\n%s", got)
	}
	if !strings.Contains(got, "func(t Todo) core.Node {") {
		t.Errorf("doc wasn't promoted to the keyed shape (missing renderFn):\n%s", got)
	}
}

// TestScheduleKeyedForResolve_HoverMissLeavesCompatShape confirms a hover
// that resolves to nothing (gopls has no info, or the response doesn't
// parse as "var <name> <Type>") leaves the document exactly as the
// synchronous compat-shape compile already published - no crash, no partial
// update, no wrong diagnostic.
func TestScheduleKeyedForResolve_HoverMissLeavesCompatShape(t *testing.T) {
	const vaneURI = "file:///D:/proj/Test.vane"
	goSrc, sm, err := compiler.CompileWithMap(keyedForFixtureSrc, "Test.vane")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	strippedGo := stripLineDirectives(goSrc)

	store := newDocStore()
	store.set(vaneURI, keyedForFixtureSrc, goSrc, sm)
	genBefore := store.generation(vaneURI)

	handled := make(chan struct{})
	pr, pw := io.Pipe()
	store.hover = newHoverClient(pw)
	store.goplsIn = pw
	go func() {
		r := bufio.NewReader(pr)
		msg, err := ReadMessage(r)
		if err != nil {
			return
		}
		var env struct {
			ID json.RawMessage `json:"id"`
		}
		_ = json.Unmarshal(msg, &env)
		var idStr string
		_ = json.Unmarshal(env.ID, &idStr)
		store.hover.take(idStr, json.RawMessage("null")) // simulates a hover miss
		close(handled)
	}()

	store.scheduleKeyedForResolve(vaneURI, keyedForFixtureSrc, strippedGo, sm)

	select {
	case <-handled:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the hover request")
	}
	// No didChange follows a hover miss - give the resolver goroutine's own
	// trailing return a brief, bounded moment to run (no further I/O left
	// on its path, so this is a settle window, not a poll/retry loop).
	time.Sleep(50 * time.Millisecond)

	if g := store.generation(vaneURI); g != genBefore {
		t.Errorf("generation changed (%d -> %d) after a hover miss - doc was updated when it shouldn't have been", genBefore, g)
	}
	doc, ok := store.get(vaneURI)
	if !ok {
		t.Fatal("document vanished from the store")
	}
	got := strings.Join(doc.goLines, "\n")
	if strings.Contains(got, "func() []Todo {") {
		t.Errorf("doc was promoted despite a hover miss:\n%s", got)
	}
}

// TestScheduleKeyedForResolve_NilHoverIsNoop confirms a docStore with no
// hover client wired up (server.go hasn't called Serve, or a test builds a
// store directly) never schedules anything - no goroutine, no panic on a
// nil s.hover.
func TestScheduleKeyedForResolve_NilHoverIsNoop(t *testing.T) {
	const vaneURI = "file:///D:/proj/Test.vane"
	goSrc, sm, err := compiler.CompileWithMap(keyedForFixtureSrc, "Test.vane")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	store := newDocStore()
	store.set(vaneURI, keyedForFixtureSrc, goSrc, sm)
	genBefore := store.generation(vaneURI)

	store.scheduleKeyedForResolve(vaneURI, keyedForFixtureSrc, stripLineDirectives(goSrc), sm)
	time.Sleep(20 * time.Millisecond)

	if g := store.generation(vaneURI); g != genBefore {
		t.Errorf("generation changed with no hover client wired up - should be a pure no-op")
	}
}

// TestScheduleKeyedForResolve_NoKeyInFileIsNoop confirms a file with no
// key={} attribute at all - the common case - is skipped without even
// parsing the compat-shape Go text, matching main.go's own file-level
// maybeKeyed heuristic.
func TestScheduleKeyedForResolve_NoKeyInFileIsNoop(t *testing.T) {
	const vaneURI = "file:///D:/proj/Test.vane"
	src := "package main\nimport \"syscall/js\"\nfunc F() js.Value {\n\treturn (\n\t\t<ul>{for _, t := range items { <li>{t.Text}</li> }}</ul>\n\t)\n}\n"
	goSrc, sm, err := compiler.CompileWithMap(src, "Test.vane")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	store := newDocStore()
	store.set(vaneURI, src, goSrc, sm)
	genBefore := store.generation(vaneURI)
	done := hoverPipe(t, store, func() (string, bool) { return "var t Todo", true })

	store.scheduleKeyedForResolve(vaneURI, src, stripLineDirectives(goSrc), sm)

	select {
	case <-done:
		t.Fatal("a hover round-trip happened for a file with no key={} at all")
	case <-time.After(100 * time.Millisecond):
	}
	if g := store.generation(vaneURI); g != genBefore {
		t.Errorf("generation changed for a file with no key={} at all")
	}
}

// TestHandleWatchedFilesChange_KeyedForResolves is a regression test for a
// real bug found running this live: workspace/didChangeWatchedFiles fires on
// every .vane save (the editor's own file watcher, separate from
// textDocument/didChange), and its handler (compileAndWrite, in
// handleWatchedFilesChange) used to always recompile the naive compat shape
// and write it straight to disk - never calling scheduleKeyedForResolve at
// all. A save landing after handleDidChange's own async promotion clobbered
// the promoted file back to the compat shape with nothing left to
// re-trigger resolution, leaving the document stuck there indefinitely.
// This confirms the save path now promotes too.
func TestHandleWatchedFilesChange_KeyedForResolves(t *testing.T) {
	dir := t.TempDir()
	vanePath := filepath.Join(dir, "Test.vane")
	if err := os.WriteFile(vanePath, []byte(keyedForFixtureSrc), 0600); err != nil {
		t.Fatalf("writing fixture .vane file: %v", err)
	}
	vaneURI := pathToFileURI(vanePath)

	store := newDocStore()
	// The synchronous compile a real didOpen/didChange would already have
	// published, exactly as compileAndWrite's own store.compile call
	// produces - not asserted on directly, just realistic starting state.
	goSrc, sm, err := compiler.CompileWithMap(keyedForFixtureSrc, "Test.vane")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	store.set(vaneURI, keyedForFixtureSrc, goSrc, sm)

	var goplsOut bytes.Buffer
	pr, pw := io.Pipe()
	store.hover = newHoverClient(pw)
	store.goplsIn = pw

	// store.goplsIn/store.hover (both wired to pw/pr below) only ever carry
	// the keyed-for resolution pass's own traffic - compileAndWrite's own
	// immediate compat-shape notify goes to goplsOut (a separate, unread
	// buffer) via the goplsIn parameter, so the only didChange this
	// responder ever sees is the promoted one.
	promoted := make(chan struct{})
	go func() {
		r := bufio.NewReader(pr)
		for {
			msg, err := ReadMessage(r)
			if err != nil {
				return
			}
			var env struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if json.Unmarshal(msg, &env) != nil {
				continue
			}
			switch env.Method {
			case "textDocument/hover":
				var idStr string
				_ = json.Unmarshal(env.ID, &idStr)
				b, _ := json.Marshal(map[string]any{
					"contents": map[string]any{"kind": "plaintext", "value": "var t Todo"},
				})
				store.hover.take(idStr, b)
			case "textDocument/didChange":
				close(promoted)
				return
			}
		}
	}()

	changedMsg, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "workspace/didChangeWatchedFiles",
		"params": map[string]any{
			"changes": []map[string]any{
				{"uri": vaneURI, "type": 2},
			},
		},
	})
	handleWatchedFilesChange(Message(changedMsg), &goplsOut, store)

	select {
	case <-promoted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the save path's own promotion")
	}

	written, err := os.ReadFile(filepath.Join(dir, "Test_vane.go"))
	if err != nil {
		t.Fatalf("reading written _vane.go: %v", err)
	}
	if !strings.Contains(string(written), "func() []Todo { return items },") {
		t.Errorf("watched-file recompile never promoted to the keyed shape:\n%s", written)
	}
}
