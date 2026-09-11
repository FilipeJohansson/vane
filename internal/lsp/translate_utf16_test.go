package lsp

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// wrap puts a vane syntax snippet inside a minimal .vane function body, the
// same fixture shape internal/compiler's tests use (that helper lives in an
// external _test package there and isn't importable here).
func wrap(jsx string) string {
	return "package main\nimport \"syscall/js\"\nfunc F() js.Value {\n\treturn (\n\t\t" + jsx + "\n\t)\n}\n"
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func mustUnmarshal(t *testing.T, data []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal %s: %v", data, err)
	}
}

// utf16ColOfLine independently computes the 0-based UTF-16 code-unit column
// of byteIdx within line, used only to derive expected test values, kept
// separate from the production byteToUTF16/utf16ToByte helpers under test.
func utf16ColOfLine(line string, byteIdx int) int {
	col := 0
	for i := 0; i < byteIdx && i < len(line); {
		r, size := utf8.DecodeRuneInString(line[i:])
		if r > 0xFFFF {
			col += 2
		} else {
			col++
		}
		i += size
	}
	return col
}

// findLine returns the 0-based index of the first line in lines containing
// substr, and substr's byte offset within it.
func findLine(t *testing.T, lines []string, substr string) (line, byteIdx int) {
	t.Helper()
	for i, l := range lines {
		if idx := strings.Index(l, substr); idx >= 0 {
			return i, idx
		}
	}
	t.Fatalf("fixture missing %q", substr)
	return 0, 0
}

// TestTranslateGoRangeToVane_UTF16NotByte is the regression test for the
// same UTF-16-treated-as-byte bug found in mapColumn (utf16_test.go), also
// present in translateGoRangeToVaneImpl's column-refinement step - the
// definition/hover/references navigation path, not just completion. "café "
// puts a 2-byte, 1-UTF16-unit rune before the target identifier on the vane
// line, so byte offset and UTF-16 column diverge by exactly 1.
func TestTranslateGoRangeToVane_UTF16NotByte(t *testing.T) {
	const ident = "handleClick"
	vaneSrc := wrap(`<div title="café" onClick={handleClick}>hi</div>`)
	doc := mustDoc(t, vaneSrc)

	goLine, goByte := findLine(t, doc.goLines, ident)
	goCol := byteToUTF16(doc.goLines[goLine], goByte)

	const uri = "file:///D:/proj/Test.vane"
	store := newTestStore(uri, doc)
	r := lspRange{lspPos{goLine, goCol}, lspPos{goLine, goCol + len(ident)}}
	got, ok, drop := translateGoRangeToVane(uri, r, store)
	if !ok || drop {
		t.Fatalf("translateGoRangeToVane(%v) not ok (ok=%v drop=%v)", r, ok, drop)
	}

	vaneLines := strings.Split(vaneSrc, "\n")
	vaneLine, vaneByte := findLine(t, vaneLines, ident)
	wantCol := utf16ColOfLine(vaneLines[vaneLine], vaneByte)
	if wantCol == vaneByte {
		t.Fatalf("fixture invalid: byte and UTF-16 offsets coincide (%d)", vaneByte)
	}

	if got.Start.Line != vaneLine || got.Start.Character != wantCol {
		t.Errorf("translateGoRangeToVane(%v) = %+v, want start (line %d, col %d) (byte offset would incorrectly give col %d)",
			r, got, vaneLine, wantCol, vaneByte)
	}
}

// TestDocumentHighlight_IdentAtGoLine_UTF16NotByte is the regression test
// for the exact expression used at both of
// translateDocumentHighlightResultJSON's identAt(goLine, col) call sites
// (expectedIdent's own computation, and the per-item comparison): col is
// UTF-16 (the position sent to/received from gopls), used to index
// doc.goLines, a UTF-8 byte string, utf16ToByte must run first.
//
// The go line hand-builds 5 accented characters (10 bytes, 5 UTF-16 units)
// before "handleClick" so byte offset and UTF-16 column diverge by exactly
// 5 - big enough that the wrong (byte-as-UTF16) position lands inside the
// unrelated "_vane1" token instead of merely shifting within "handleClick"
// itself (identAt walks both directions from the start column, so a small
// divergence can still resolve to the same word by accident; confirmed by
// hand-computing byte 60 of this exact fixture, which lands mid "_vane1").
func TestDocumentHighlight_IdentAtGoLine_UTF16NotByte(t *testing.T) {
	const goLine = `core.SetProp(_vane1, "title", "ééééé"); core.OnClick(_vane1, handleClick)`
	const ident = "handleClick"
	byteIdx := strings.Index(goLine, ident)
	if byteIdx < 0 {
		t.Fatalf("fixture missing %q", ident)
	}
	utf16Col := byteToUTF16(goLine, byteIdx)
	if utf16Col == byteIdx {
		t.Fatalf("fixture invalid: byte and UTF-16 offsets coincide (%d)", byteIdx)
	}

	got := identAt(goLine, utf16ToByte(goLine, utf16Col))
	if got != ident {
		t.Errorf("identAt(goLine, utf16ToByte(goLine, %d)) = %q, want %q", utf16Col, got, ident)
	}
}

// TestTranslateDiagnosticsPos_TranslatesRange covers translateDiagnosticsPos,
// which had zero test coverage despite being the proxy's most-visible
// feature (the red squiggles under a syntax/type error).
func TestTranslateDiagnosticsPos_TranslatesRange(t *testing.T) {
	const ident = "handleClick"
	vaneSrc := wrap(`<div onClick={handleClick}>hi</div>`)
	doc := mustDoc(t, vaneSrc)

	goLine, goByte := findLine(t, doc.goLines, ident)
	goCol := byteToUTF16(doc.goLines[goLine], goByte)

	const uri = "file:///D:/proj/Test.vane"
	store := newTestStore(uri, doc)

	notif := mustMarshal(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/publishDiagnostics",
		"params": map[string]any{
			"uri": uri,
			"diagnostics": []map[string]any{
				{
					"range":    lspRange{lspPos{goLine, goCol}, lspPos{goLine, goCol + len(ident)}},
					"severity": 1,
					"message":  "undefined: handleClick",
				},
			},
		},
	})

	out := translateDiagnosticsPos(Message(notif), store)

	var parsed struct {
		Params struct {
			Diagnostics []struct {
				Range   lspRange `json:"range"`
				Message string   `json:"message"`
			} `json:"diagnostics"`
		} `json:"params"`
	}
	mustUnmarshal(t, out, &parsed)
	if len(parsed.Params.Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %s", len(parsed.Params.Diagnostics), out)
	}
	d := parsed.Params.Diagnostics[0]

	vaneLine, _ := findLine(t, strings.Split(vaneSrc, "\n"), ident)
	if d.Range.Start.Line != vaneLine {
		t.Errorf("diagnostic range start line = %d, want %d (still in go coordinates: translation didn't happen)", d.Range.Start.Line, vaneLine)
	}
	if d.Message != "undefined: handleClick" {
		t.Errorf("diagnostic message changed unexpectedly: %q", d.Message)
	}
}
