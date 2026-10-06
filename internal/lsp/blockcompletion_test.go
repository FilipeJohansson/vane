package lsp

import (
	"encoding/json"
	"strings"
	"testing"
)

const blockCompletionSrc = "package pages\n\nimport (\n\t\"github.com/filipejohansson/vane/core\"\n\t\"github.com/filipejohansson/vane/core/router\"\n)\n\nfunc P() core.Node {\n\treturn (\n\t\t<div>\n\t\t\t{router.Link(router.LinkProps{  }, \"x\")}\n\t\t</div>\n\t)\n}\n"

// Inside a `{...}` expression block the generated Go wraps the expression in
// code with no .vane counterpart, so a plain column offset lands in that
// wrapper. A cursor on blank space (no identifier to look up) must still map
// to the same spot in the copied expression text.
func TestTranslateRequestPos_CompletionInBlankSpaceInsideExpressionBlock(t *testing.T) {
	doc := mustDoc(t, blockCompletionSrc)
	const vaneURI = "file:///D:/proj/Test.vane"
	store := newTestStore(vaneURI, doc)

	const vaneLine = 10
	vaneCol := strings.Index(doc.vaneLines[vaneLine], "{  }") + 2 // between the two spaces

	req, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/completion",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": vaneURI},
			"position":     map[string]any{"line": vaneLine, "character": vaneCol},
		},
	})
	out := translateRequestPos(Message(req), store)

	var got struct {
		Params struct {
			Position lspPos `json:"position"`
		} `json:"params"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	line := doc.goLines[got.Params.Position.Line]
	before := line[:utf16ToByte(line, got.Params.Position.Character)]
	if !strings.HasSuffix(before, "router.LinkProps{ ") {
		t.Errorf("completion position maps to the wrong place in the generated line:\n  go line: %q\n  text before mapped cursor: %q", line, before)
	}
}
