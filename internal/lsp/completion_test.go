package lsp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/filipejohansson/vane/internal/compiler"
)

// mustDoc compiles src and builds a *document the same way docStore.set does,
// so tests exercise the real SourceMap instead of a hand-built stand-in.
func mustDoc(t *testing.T, src string) *document {
	t.Helper()
	goSrc, sm, err := compiler.CompileWithMap(src, "Test.vane")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return &document{
		vaneLines: strings.Split(src, "\n"),
		goLines:   strings.Split(stripLineDirectives(goSrc), "\n"),
		sourceMap: sm,
	}
}

const completionFixtureSrc = `package main

import "github.com/filipejohansson/vane/core"

func App() core.Node {
	count := core.NewSignal(0)
	return core.Text("hi")
}
`

// goRangeForVaneIdent locates ident on vaneLine in doc, maps it through
// VaneToGo+mapColumn, and returns the resulting go (line, col) plus the vane
// (line, col) it started from, for building a synthetic gopls response.
func goRangeForVaneIdent(t *testing.T, doc *document, vaneLine int, ident string) (goLine, goCol, vaneLine2, vaneCol int) {
	t.Helper()
	vaneCol = strings.Index(doc.vaneLines[vaneLine], ident)
	if vaneCol < 0 {
		t.Fatalf("fixture missing %q on line %d: %q", ident, vaneLine, doc.vaneLines[vaneLine])
	}
	gl, gc, ok := doc.sourceMap.VaneToGo(vaneLine, vaneCol)
	if !ok {
		t.Fatalf("VaneToGo failed for line %d", vaneLine)
	}
	gl, gc = mapColumn(doc, vaneLine, vaneCol, gl, gc)
	return gl, gc, vaneLine, vaneCol
}

func newTestStore(vaneURI string, doc *document) *docStore {
	store := newDocStore()
	store.mu.Lock()
	store.docs[normalizeFileURI(vaneURI)] = doc
	store.mu.Unlock()
	return store
}

func TestTranslateCompletionResultJSON_CompletionList(t *testing.T) {
	doc := mustDoc(t, completionFixtureSrc)
	vaneURI := "file:///D:/proj/Test.vane"
	store := newTestStore(vaneURI, doc)

	goLine, goCol, vaneLine, vaneCol := goRangeForVaneIdent(t, doc, 5, "count")

	result, _ := json.Marshal(map[string]any{
		"isIncomplete": false,
		"items": []map[string]any{
			{
				"label": "count",
				"textEdit": map[string]any{
					"range": map[string]any{
						"start": map[string]any{"line": goLine, "character": goCol},
						"end":   map[string]any{"line": goLine, "character": goCol + len("count")},
					},
					"newText": "count",
				},
			},
		},
	})

	translated, changed := translateCompletionResultJSON(vaneURI, result, store)
	if !changed {
		t.Fatalf("expected translation to report a change")
	}

	var out struct {
		Items []struct {
			TextEdit struct {
				Range lspRange `json:"range"`
			} `json:"textEdit"`
		} `json:"items"`
	}
	if err := json.Unmarshal(translated, &out); err != nil {
		t.Fatalf("unmarshal translated result: %v", err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out.Items))
	}
	got := out.Items[0].TextEdit.Range.Start
	if got.Line != vaneLine || got.Character != vaneCol {
		t.Errorf("range start = %+v, want line %d col %d", got, vaneLine, vaneCol)
	}
}

func TestTranslateCompletionResultJSON_BareArray(t *testing.T) {
	doc := mustDoc(t, completionFixtureSrc)
	vaneURI := "file:///D:/proj/Test.vane"
	store := newTestStore(vaneURI, doc)

	goLine, goCol, vaneLine, vaneCol := goRangeForVaneIdent(t, doc, 5, "count")

	result, _ := json.Marshal([]map[string]any{
		{
			"label": "count",
			"textEdit": map[string]any{
				"range": map[string]any{
					"start": map[string]any{"line": goLine, "character": goCol},
					"end":   map[string]any{"line": goLine, "character": goCol + len("count")},
				},
				"newText": "count",
			},
		},
	})

	translated, changed := translateCompletionResultJSON(vaneURI, result, store)
	if !changed {
		t.Fatalf("expected translation to report a change")
	}

	var items []struct {
		TextEdit struct {
			Range lspRange `json:"range"`
		} `json:"textEdit"`
	}
	if err := json.Unmarshal(translated, &items); err != nil {
		t.Fatalf("unmarshal translated result: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	got := items[0].TextEdit.Range.Start
	if got.Line != vaneLine || got.Character != vaneCol {
		t.Errorf("range start = %+v, want line %d col %d", got, vaneLine, vaneCol)
	}
}

func TestTranslateCompletionEdit_InsertReplace(t *testing.T) {
	doc := mustDoc(t, completionFixtureSrc)
	vaneURI := "file:///D:/proj/Test.vane"
	store := newTestStore(vaneURI, doc)

	goLine, goCol, vaneLine, vaneCol := goRangeForVaneIdent(t, doc, 5, "count")

	raw, _ := json.Marshal(map[string]any{
		"insert": map[string]any{
			"start": map[string]any{"line": goLine, "character": goCol},
			"end":   map[string]any{"line": goLine, "character": goCol + len("count")},
		},
		"replace": map[string]any{
			"start": map[string]any{"line": goLine, "character": goCol},
			"end":   map[string]any{"line": goLine, "character": goCol + len("count")},
		},
		"newText": "count",
	})

	translated, changed := translateCompletionEdit(vaneURI, raw, store)
	if !changed {
		t.Fatalf("expected translation to report a change")
	}

	var out struct {
		Insert  lspRange `json:"insert"`
		Replace lspRange `json:"replace"`
		NewText string   `json:"newText"`
	}
	if err := json.Unmarshal(translated, &out); err != nil {
		t.Fatalf("unmarshal translated edit: %v", err)
	}
	if out.Insert.Start.Line != vaneLine || out.Insert.Start.Character != vaneCol {
		t.Errorf("insert start = %+v, want line %d col %d", out.Insert.Start, vaneLine, vaneCol)
	}
	if out.Replace.Start.Line != vaneLine || out.Replace.Start.Character != vaneCol {
		t.Errorf("replace start = %+v, want line %d col %d", out.Replace.Start, vaneLine, vaneCol)
	}
	if out.NewText != "count" {
		t.Errorf("newText = %q, want %q", out.NewText, "count")
	}
}

func TestTranslateCompletionResultJSON_ItemDefaultsEditRange(t *testing.T) {
	doc := mustDoc(t, completionFixtureSrc)
	vaneURI := "file:///D:/proj/Test.vane"
	store := newTestStore(vaneURI, doc)

	goLine, goCol, vaneLine, vaneCol := goRangeForVaneIdent(t, doc, 5, "count")

	result, _ := json.Marshal(map[string]any{
		"isIncomplete": false,
		"itemDefaults": map[string]any{
			"editRange": map[string]any{
				"start": map[string]any{"line": goLine, "character": goCol},
				"end":   map[string]any{"line": goLine, "character": goCol + len("count")},
			},
		},
		"items": []map[string]any{
			{"label": "count", "insertText": "count"},
		},
	})

	translated, changed := translateCompletionResultJSON(vaneURI, result, store)
	if !changed {
		t.Fatalf("expected translation to report a change")
	}

	var out struct {
		ItemDefaults struct {
			EditRange lspRange `json:"editRange"`
		} `json:"itemDefaults"`
	}
	if err := json.Unmarshal(translated, &out); err != nil {
		t.Fatalf("unmarshal translated result: %v", err)
	}
	got := out.ItemDefaults.EditRange.Start
	if got.Line != vaneLine || got.Character != vaneCol {
		t.Errorf("editRange start = %+v, want line %d col %d", got, vaneLine, vaneCol)
	}
}

// TestMapColumn_ClampsOverlongColumn covers the "column is beyond end of line"
// gopls error seen live: right after typing "core." (cursor at end of the vane
// line, nothing typed yet), identAt returns "" so both tryLine paths are
// skipped and mapColumn falls through to the raw-column fallback. If the
// mapped go line is empty (n == 0) or simply shorter than the vane column,
// the fallback must clamp instead of forwarding an out-of-range column.
func TestMapColumn_ClampsOverlongColumn(t *testing.T) {
	cases := []struct {
		name      string
		goLine    string
		gc        int
		wantGoCol int
	}{
		{name: "empty go line", goLine: "", gc: 5, wantGoCol: 0},
		{name: "short go line", goLine: "x", gc: 5, wantGoCol: 1},
		{name: "column within bounds", goLine: "core.Get()", gc: 5, wantGoCol: 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := &document{
				vaneLines: []string{"core."},
				goLines:   []string{tc.goLine},
			}
			// vaneCol == len("core.") so identAt sees the cursor at end of
			// line (right after the dot) and returns "", matching the
			// "core.|" trigger-completion scenario.
			gl, gc := mapColumn(doc, 0, len("core."), 0, tc.gc)
			if gl != 0 || gc != tc.wantGoCol {
				t.Errorf("mapColumn() = (%d, %d), want (0, %d)", gl, gc, tc.wantGoCol)
			}
			if gc > len(tc.goLine) {
				t.Errorf("mapColumn() returned column %d beyond go line length %d", gc, len(tc.goLine))
			}
		})
	}
}

// TestMapColumn_ClampsOverlongGoLine covers the case seen live where "column
// is beyond end of line" persisted even after clamping gc: VaneToGo
// extrapolates goLine as a fixed offset from the nearest //line directive,
// which only holds in 1:1 passthrough regions. Deep inside a region that
// expanded or collapsed non-1:1 (e.g. typing "core.P" for "core.Portal"
// inside a JSX-adjacent return), that offset can point past the end of the
// real generated file. ident ("P") won't match any real identifier either,
// so both tryLine paths fail and mapColumn must still return in-bounds
// coordinates instead of the raw, unclamped (and here, out-of-range) goLine.
func TestMapColumn_ClampsOverlongGoLine(t *testing.T) {
	doc := &document{
		vaneLines: []string{"return core.P"},
		goLines:   []string{"a", "bb"}, // real generated file is only 2 lines
	}
	gl, gc := mapColumn(doc, 0, len("return core.P"), 5, 20)
	if gl < 0 || gl >= len(doc.goLines) {
		t.Fatalf("mapColumn returned out-of-range goLine %d (have %d lines)", gl, len(doc.goLines))
	}
	if gc < 0 || gc > len(doc.goLines[gl]) {
		t.Errorf("mapColumn returned out-of-range column %d for line %q", gc, doc.goLines[gl])
	}
}

func TestTranslateCompletionResultJSON_NoChangeWhenNull(t *testing.T) {
	result := json.RawMessage("null")
	translated, changed := translateCompletionResultJSON("file:///D:/proj/Test.vane", result, newDocStore())
	if changed {
		t.Errorf("expected no change for null result")
	}
	if string(translated) != "null" {
		t.Errorf("expected result to pass through unchanged, got %s", translated)
	}
}
