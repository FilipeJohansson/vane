package lsp

import (
	"encoding/json"
	"strings"
	"testing"
)

const attrNameClashSrc = "package ui\n\nimport \"github.com/filipejohansson/vane/core\"\n\nfunc Field(value *core.Signal[string]) core.Node {\n\treturn (\n\t\t<input\n\t\t\tvalue={value.Get()}\n\t\t/>\n\t)\n}\n"

// A bound attribute whose name is also the variable used in its value
// (`value={value.Get()}`) leaves two whole-word `value`s in the generated line:
// the attribute name as a string literal, and the variable. Navigation must
// land on the variable.
func TestTranslateRequestPos_IdentifierSharingNameWithAttribute(t *testing.T) {
	doc := mustDoc(t, attrNameClashSrc)
	const vaneURI = "file:///D:/proj/Test.vane"
	store := newTestStore(vaneURI, doc)

	const vaneLine = 7
	vaneCol := strings.Index(doc.vaneLines[vaneLine], "value.Get") + 2 // inside the variable

	req, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "textDocument/definition",
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
	after := line[utf16ToByte(line, got.Params.Position.Character):]
	if !strings.HasPrefix(after, "value.Get") && !strings.HasPrefix(after, "lue.Get") && !strings.HasPrefix(after, "ue.Get") {
		t.Errorf("definition position maps to the wrong `value`:\n  go line: %q\n  text from mapped cursor: %q", line, after)
	}
}
