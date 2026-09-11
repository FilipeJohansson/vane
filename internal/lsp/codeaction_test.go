package lsp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// codeActionFixtureSrc mirrors the shape that actually corrupted a file live:
// a small import block followed by a function, with a call to an
// as-yet-unimported package (router.Navigate) that gopls's "organize
// imports" quick fix would want to add an import for.
const codeActionFixtureSrc = `package pages

import (
	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/examples/fullstack-app/src/store"
)

func Home() core.Node {
	return core.Text("hi")
}
`

// TestTranslateWorkspaceEditJSON_DropsImportBlockEdit is the regression test
// for the actual file-corruption bug: translateGoRangeToVane returns
// (r, ok=false, drop=true) for a range landing in the package/import
// preamble, but the DocumentChanges loop only acted "if ok", an edit that
// couldn't be translated was left in the array with its original
// go-coordinate range untouched, still targeting the .vane URI. The editor
// applied that raw go-coordinate edit directly to the .vane buffer, splicing
// garbage into the middle of an existing import string literal (confirmed
// live). The edit must be removed entirely, not left untranslated.
func TestTranslateWorkspaceEditJSON_DropsImportBlockEdit(t *testing.T) {
	doc := mustDoc(t, codeActionFixtureSrc)
	vaneURI := "file:///D:/proj/Home.vane"

	// Locate the go line for the "core" import so the edit's range lands
	// solidly inside the import block, the same region the live bug hit.
	goLine, _, ok := doc.sourceMap.VaneToGo(3, 0)
	if !ok {
		t.Fatalf("VaneToGo failed for import line")
	}

	editJSON, _ := json.Marshal(map[string]any{
		"documentChanges": []map[string]any{
			{
				"textDocument": map[string]any{"uri": vaneURI, "version": 1},
				"edits": []map[string]any{
					{
						"range": map[string]any{
							"start": map[string]any{"line": goLine, "character": 0},
							"end":   map[string]any{"line": goLine, "character": 0},
						},
						"newText": "\t\"github.com/filipejohansson/vane/core/router\"\n",
					},
				},
			},
		},
	})

	translated, changed := translateWorkspaceEditJSON(editJSON, newDocStore())
	_ = changed // dropping the only edit still counts as "changed" (see below)

	var out struct {
		DocumentChanges []struct {
			Edits []json.RawMessage `json:"edits"`
		} `json:"documentChanges"`
	}
	if err := json.Unmarshal(translated, &out); err != nil {
		t.Fatalf("unmarshal translated edit: %v", err)
	}

	// The docStore passed above is empty (no doc registered for vaneURI), so
	// translateGoRangeToVane's !ok path (store.getByNorm miss) exercises the
	// same "couldn't translate, must not leave raw go-coordinates" contract
	// as the drop path. Either way, nothing should survive with an
	// untranslated go-coordinate range against a .vane file.
	for _, dc := range out.DocumentChanges {
		if len(dc.Edits) != 0 {
			t.Errorf("expected no surviving edits for an untranslatable range, got %d: %s", len(dc.Edits), translated)
		}
	}
	if len(out.DocumentChanges) > 1 {
		t.Errorf("expected the empty documentChanges entry to be dropped too, got %d entries", len(out.DocumentChanges))
	}
}

// TestTranslateWorkspaceEditJSON_TranslatesImportBlockEdit covers the "add
// missing import" quick fix specifically: unlike navigation results, an edit
// landing in the import block is meaningful (the block is pure passthrough
// between .vane and .go) and must be translated, not dropped. Dropping it
// unconditionally (the original, overly-conservative behavior) silently
// broke "add missing import", gopls no longer offered or applied it,
// instead of just failing loudly.
func TestTranslateWorkspaceEditJSON_TranslatesImportBlockEdit(t *testing.T) {
	doc := mustDoc(t, codeActionFixtureSrc)
	vaneURI := "file:///D:/proj/Home.vane"
	store := newTestStore(vaneURI, doc)

	const vaneLine, vaneCol = 3, 0 // start of the "core" import line
	goLine, goCol, ok := doc.sourceMap.VaneToGo(vaneLine, vaneCol)
	if !ok {
		t.Fatalf("VaneToGo failed for import line")
	}

	editJSON, _ := json.Marshal(map[string]any{
		"documentChanges": []map[string]any{
			{
				"textDocument": map[string]any{"uri": vaneURI, "version": 1},
				"edits": []map[string]any{
					{
						"range": map[string]any{
							"start": map[string]any{"line": goLine, "character": goCol},
							"end":   map[string]any{"line": goLine, "character": goCol},
						},
						"newText": "\t\"github.com/filipejohansson/vane/core/router\"\n",
					},
				},
			},
		},
	})

	translated, changed := translateWorkspaceEditJSON(editJSON, store)
	if !changed {
		t.Fatalf("expected changed=true")
	}

	var out struct {
		DocumentChanges []struct {
			Edits []struct {
				Range   lspRange `json:"range"`
				NewText string   `json:"newText"`
			} `json:"edits"`
		} `json:"documentChanges"`
	}
	if err := json.Unmarshal(translated, &out); err != nil {
		t.Fatalf("unmarshal translated edit: %v", err)
	}
	if len(out.DocumentChanges) != 1 || len(out.DocumentChanges[0].Edits) != 1 {
		t.Fatalf("expected the import-block edit to survive translation, got %+v", out.DocumentChanges)
	}
	got := out.DocumentChanges[0].Edits[0]
	if got.Range.Start.Line != vaneLine || got.Range.Start.Character != vaneCol {
		t.Errorf("range start = %+v, want line %d col %d", got.Range.Start, vaneLine, vaneCol)
	}
	if !strings.Contains(got.NewText, "core/router") {
		t.Errorf("newText = %q, expected the new import to be preserved", got.NewText)
	}
}

// TestTranslateWorkspaceEditJSON_TranslatesFuncBodyEdit is the positive-path
// sibling: an edit landing inside the function body (not the preamble)
// should survive and have its range translated to vane coordinates, proving
// the fix doesn't just drop everything indiscriminately.
func TestTranslateWorkspaceEditJSON_TranslatesFuncBodyEdit(t *testing.T) {
	doc := mustDoc(t, codeActionFixtureSrc)
	vaneURI := "file:///D:/proj/Home.vane"
	store := newTestStore(vaneURI, doc)

	vaneLine := 7 // `func Home() core.Node {`
	vaneCol := strings.Index(doc.vaneLines[vaneLine], "Home")
	if vaneCol < 0 {
		t.Fatalf("fixture missing 'Home' on line %d: %q", vaneLine, doc.vaneLines[vaneLine])
	}
	goLine, goCol, ok := doc.sourceMap.VaneToGo(vaneLine, vaneCol)
	if !ok {
		t.Fatalf("VaneToGo failed for line %d", vaneLine)
	}
	goLine, goCol = mapColumn(doc, vaneLine, vaneCol, goLine, goCol)

	editJSON, _ := json.Marshal(map[string]any{
		"documentChanges": []map[string]any{
			{
				"textDocument": map[string]any{"uri": vaneURI, "version": 1},
				"edits": []map[string]any{
					{
						"range": map[string]any{
							"start": map[string]any{"line": goLine, "character": goCol},
							"end":   map[string]any{"line": goLine, "character": goCol + len("Home")},
						},
						"newText": "HomePage",
					},
				},
			},
		},
	})

	translated, changed := translateWorkspaceEditJSON(editJSON, store)
	if !changed {
		t.Fatalf("expected changed=true")
	}

	var out struct {
		DocumentChanges []struct {
			Edits []struct {
				Range   lspRange `json:"range"`
				NewText string   `json:"newText"`
			} `json:"edits"`
		} `json:"documentChanges"`
	}
	if err := json.Unmarshal(translated, &out); err != nil {
		t.Fatalf("unmarshal translated edit: %v", err)
	}
	if len(out.DocumentChanges) != 1 || len(out.DocumentChanges[0].Edits) != 1 {
		t.Fatalf("expected exactly 1 surviving edit, got %+v", out.DocumentChanges)
	}
	got := out.DocumentChanges[0].Edits[0].Range.Start
	if got.Line != vaneLine || got.Character != vaneCol {
		t.Errorf("range start = %+v, want line %d col %d", got, vaneLine, vaneCol)
	}
}

// TestTranslateCodeActionResultJSON_TranslatesImportFixEdit exercises the full
// textDocument/codeAction response path end-to-end: a CodeAction (as gopls's
// "organize imports" quick fix would return) whose embedded edit targets the
// virtual _vane.go URI at a position inside the import block. The edit must
// survive with its range correctly translated to vane coordinates, this is
// the actual "add missing import" quick fix the user relies on.
func TestTranslateCodeActionResultJSON_TranslatesImportFixEdit(t *testing.T) {
	doc := mustDoc(t, codeActionFixtureSrc)
	vaneURI := "file:///D:/proj/Home.vane"
	store := newTestStore(vaneURI, doc)

	const vaneLine, vaneCol = 3, 0
	goLine, goCol, ok := doc.sourceMap.VaneToGo(vaneLine, vaneCol)
	if !ok {
		t.Fatalf("VaneToGo failed for import line")
	}

	result, _ := json.Marshal([]map[string]any{
		{
			"title": "Organize Imports",
			"kind":  "source.organizeImports",
			"edit": map[string]any{
				"documentChanges": []map[string]any{
					{
						"textDocument": map[string]any{"uri": "file:///D:/proj/Home_vane.go", "version": 1},
						"edits": []map[string]any{
							{
								"range": map[string]any{
									"start": map[string]any{"line": goLine, "character": goCol},
									"end":   map[string]any{"line": goLine, "character": goCol},
								},
								"newText": "\t\"github.com/filipejohansson/vane/core/router\"\n",
							},
						},
					},
				},
			},
		},
	})

	translated, changed := translateCodeActionResultJSON(vaneURI, result, store)
	if !changed {
		t.Fatalf("expected changed=true")
	}

	var items []struct {
		Edit struct {
			DocumentChanges []struct {
				Edits []struct {
					Range lspRange `json:"range"`
				} `json:"edits"`
			} `json:"documentChanges"`
		} `json:"edit"`
	}
	if err := json.Unmarshal(translated, &items); err != nil {
		t.Fatalf("unmarshal translated codeAction result: %v", err)
	}
	if len(items) != 1 || len(items[0].Edit.DocumentChanges) != 1 || len(items[0].Edit.DocumentChanges[0].Edits) != 1 {
		t.Fatalf("expected the import-fix edit to survive translation, got %+v", items)
	}
	got := items[0].Edit.DocumentChanges[0].Edits[0].Range.Start
	if got.Line != vaneLine || got.Character != vaneCol {
		t.Errorf("range start = %+v, want line %d col %d", got, vaneLine, vaneCol)
	}
}

// TestTranslateCompletionResultJSON_TranslatesImportFixAdditionalEdit is the
// regression test for the second corruption path found live: completing an
// unimported symbol (Ctrl+Space on "router") pairs the completion item with
// an additionalTextEdits entry that inserts the missing import — a
// different code path from the codeAction "organize imports" quick fix, and
// one translateCompletionItemsJSON's additionalTextEdits handling was still
// routing through the navigation-style translateGoRangeToVane (drops
// import-block ranges) instead of translateGoRangeToVaneForEdit.
func TestTranslateCompletionResultJSON_TranslatesImportFixAdditionalEdit(t *testing.T) {
	doc := mustDoc(t, codeActionFixtureSrc)
	vaneURI := "file:///D:/proj/Home.vane"
	store := newTestStore(vaneURI, doc)

	const vaneLine, vaneCol = 3, 0
	goLine, goCol, ok := doc.sourceMap.VaneToGo(vaneLine, vaneCol)
	if !ok {
		t.Fatalf("VaneToGo failed for import line")
	}

	result, _ := json.Marshal(map[string]any{
		"isIncomplete": false,
		"items": []map[string]any{
			{
				"label":      "router",
				"insertText": "router",
				"additionalTextEdits": []map[string]any{
					{
						"range": map[string]any{
							"start": map[string]any{"line": goLine, "character": goCol},
							"end":   map[string]any{"line": goLine, "character": goCol},
						},
						"newText": "\t\"github.com/filipejohansson/vane/core/router\"\n",
					},
				},
			},
		},
	})

	translated, changed := translateCompletionResultJSON(vaneURI, -1, -1, -1, -1, result, store)
	if !changed {
		t.Fatalf("expected changed=true")
	}

	var out struct {
		Items []struct {
			AdditionalTextEdits []struct {
				Range lspRange `json:"range"`
			} `json:"additionalTextEdits"`
		} `json:"items"`
	}
	if err := json.Unmarshal(translated, &out); err != nil {
		t.Fatalf("unmarshal translated result: %v", err)
	}
	if len(out.Items) != 1 || len(out.Items[0].AdditionalTextEdits) != 1 {
		t.Fatalf("expected the import-fix additionalTextEdit to survive translation, got %+v", out.Items)
	}
	got := out.Items[0].AdditionalTextEdits[0].Range.Start
	if got.Line != vaneLine || got.Character != vaneCol {
		t.Errorf("range start = %+v, want line %d col %d", got, vaneLine, vaneCol)
	}
}

// TestTranslateGoRangeToVaneForEdit_DistinctZeroWidthInsertsStayDistinct is
// the regression test for "Overlapping ranges are not allowed!": gopls's
// goimports rewrites an import line via a byte-level diff, producing several
// zero-width insertions at distinct, adjacent columns on the same go line
// (observed live: "re/", "out", "r" inserted at columns 36, 37, 38 to splice
// "core/router" out of an existing "core" import). translateGoRangeToVane's
// identifier-snapping (built for hover/rename, where you want the whole
// identifier under the cursor) resolved all three to the same containing
// identifier and therefore the same range, three edits colliding into one,
// which the editor rejected. translateGoRangeToVaneForEdit must preserve each
// insertion's own exact column instead.
func TestTranslateGoRangeToVaneForEdit_DistinctZeroWidthInsertsStayDistinct(t *testing.T) {
	doc := mustDoc(t, codeActionFixtureSrc)
	vaneURI := "file:///D:/proj/Home.vane"
	store := newTestStore(vaneURI, doc)

	vaneLine := 3 // `	"github.com/filipejohansson/vane/core"`
	base := strings.Index(doc.vaneLines[vaneLine], "core")
	if base < 0 {
		t.Fatalf("fixture missing 'core' on line %d: %q", vaneLine, doc.vaneLines[vaneLine])
	}

	// Three adjacent zero-width columns landing inside/around the same
	// identifier ("core"), the same shape as the live "re/"/"out"/"r" splice.
	cols := []int{base, base + 1, base + 2}
	seen := map[string]bool{}
	for _, vaneCol := range cols {
		goLine, goCol, ok := doc.sourceMap.VaneToGo(vaneLine, vaneCol)
		if !ok {
			t.Fatalf("VaneToGo failed for col %d", vaneCol)
		}
		r := lspRange{lspPos{goLine, goCol}, lspPos{goLine, goCol}}
		got, ok, drop := translateGoRangeToVaneForEdit(vaneURI, r, store)
		if !ok || drop {
			t.Fatalf("translateGoRangeToVaneForEdit failed for col %d: ok=%v drop=%v", vaneCol, ok, drop)
		}
		if got.Start != got.End {
			t.Errorf("col %d: expected a zero-width range, got %+v", vaneCol, got)
		}
		if got.Start.Line != vaneLine || got.Start.Character != vaneCol {
			t.Errorf("col %d: translated to %+v, want line %d col %d", vaneCol, got.Start, vaneLine, vaneCol)
		}
		key := fmt.Sprintf("%d:%d", got.Start.Line, got.Start.Character)
		if seen[key] {
			t.Errorf("col %d: collided with a previous edit's translated range %s — this is exactly the overlap bug", vaneCol, key)
		}
		seen[key] = true
	}
}

// TestClampRangeOrder covers the "start (offset N) > end (offset M)" error
// gopls returned live for a codeAction request: translateRequestPos's Range
// branch refines each endpoint via mapColumn independently, and nothing
// guarantees the refined end still follows the refined start.
func TestClampRangeOrder(t *testing.T) {
	cases := []struct {
		name           string
		sl, sc, el, ec int
		wantEl, wantEc int
	}{
		{name: "already ordered", sl: 1, sc: 5, el: 1, ec: 10, wantEl: 1, wantEc: 10},
		{name: "different lines, ordered", sl: 1, sc: 5, el: 2, ec: 0, wantEl: 2, wantEc: 0},
		{name: "same line, inverted columns", sl: 1, sc: 10, el: 1, ec: 5, wantEl: 1, wantEc: 10},
		{name: "inverted lines", sl: 2, sc: 0, el: 1, ec: 5, wantEl: 2, wantEc: 0},
		{name: "equal", sl: 1, sc: 5, el: 1, ec: 5, wantEl: 1, wantEc: 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotEl, gotEc := clampRangeOrder(tc.sl, tc.sc, tc.el, tc.ec)
			if gotEl != tc.wantEl || gotEc != tc.wantEc {
				t.Errorf("clampRangeOrder(%d,%d,%d,%d) = (%d,%d), want (%d,%d)",
					tc.sl, tc.sc, tc.el, tc.ec, gotEl, gotEc, tc.wantEl, tc.wantEc)
			}
		})
	}
}
