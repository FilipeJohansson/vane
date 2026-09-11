package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestHandleGoToVanePosition covers the custom vane/goToVanePosition request:
// the extension's navigation-redirect feature asks the running vane lsp
// server to translate a position in a generated _vane.go file (as VS Code's
// own Go tooling reports it, never routed through this proxy) back to the
// real .vane file and position, using the doc's real SourceMap instead of
// re-parsing stripped-of-directives file text client-side.
func TestHandleGoToVanePosition(t *testing.T) {
	// Plain Go code with no vane syntax around it, matching the real-world
	// case (a plain call like TopicBySlug(slug), not a JSX identifier). This
	// region is pure 1:1 passthrough, so line correctness is exact; column
	// isn't asserted here since anchor-based column mapping (see
	// sourcemap_test.go) isn't guaranteed exact away from a directive's own
	// anchor point.
	const ident = "doSomething"
	vaneSrc := "package main\nimport \"syscall/js\"\nfunc helper() {\n\tdoSomething()\n}\nfunc F() js.Value {\n\treturn (\n\t\t<div>hi</div>\n\t)\n}\n"
	doc := mustDoc(t, vaneSrc)

	goLine, goByte := findLine(t, doc.goLines, ident)
	goCol := byteToUTF16(doc.goLines[goLine], goByte)

	const vaneURI = "file:///D:/proj/Test.vane"
	const goURI = "file:///D:/proj/Test_vane.go"
	store := newTestStore(vaneURI, doc)

	req := mustMarshal(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "vane/goToVanePosition",
		"params": map[string]any{
			"uri":      goURI,
			"position": lspPos{goLine, goCol},
		},
	})

	var out bytes.Buffer
	handleGoToVanePosition(Message(req), &out, store)

	resp, err := ReadMessage(bufio.NewReader(&out))
	if err != nil {
		t.Fatalf("reading response: %v", err)
	}

	var parsed struct {
		ID     int `json:"id"`
		Result *struct {
			URI      string `json:"uri"`
			Position lspPos `json:"position"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		t.Fatalf("unmarshal response %s: %v", resp, err)
	}
	if parsed.ID != 1 {
		t.Errorf("response id = %d, want 1", parsed.ID)
	}
	if parsed.Result == nil {
		t.Fatalf("expected a non-null result, got null: %s", resp)
	}
	if parsed.Result.URI != vaneURI {
		t.Errorf("result uri = %q, want %q", parsed.Result.URI, vaneURI)
	}

	vaneLines := strings.Split(vaneSrc, "\n")
	wantLine, _ := findLine(t, vaneLines, ident)
	if parsed.Result.Position.Line != wantLine {
		t.Errorf("result line = %d, want %d", parsed.Result.Position.Line, wantLine)
	}
}

// TestHandleGoToVanePosition_UnknownDoc covers the miss path: a uri the
// store has never seen (e.g. the workspace was reloaded and precompile
// hasn't run yet) must respond with a null result, not hang or panic.
func TestHandleGoToVanePosition_UnknownDoc(t *testing.T) {
	store := newDocStore()
	req := mustMarshal(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "vane/goToVanePosition",
		"params": map[string]any{
			"uri":      "file:///D:/proj/Unknown_vane.go",
			"position": lspPos{0, 0},
		},
	})

	var out bytes.Buffer
	handleGoToVanePosition(Message(req), &out, store)

	resp, err := ReadMessage(bufio.NewReader(&out))
	if err != nil {
		t.Fatalf("reading response: %v", err)
	}
	var parsed struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		t.Fatalf("unmarshal response %s: %v", resp, err)
	}
	if string(parsed.Result) != "null" {
		t.Errorf("result = %s, want null", parsed.Result)
	}
}
