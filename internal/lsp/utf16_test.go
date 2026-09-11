package lsp

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestUTF16ByteConversion_ASCII(t *testing.T) {
	line := "core.OnClick"
	for col := 0; col <= len(line); col++ {
		if b := utf16ToByte(line, col); b != col {
			t.Errorf("utf16ToByte(%q, %d) = %d, want %d (ASCII line, 1:1)", line, col, b, col)
		}
		if u := byteToUTF16(line, col); u != col {
			t.Errorf("byteToUTF16(%q, %d) = %d, want %d", line, col, u, col)
		}
	}
}

func TestUTF16ByteConversion_MultiByteNotSurrogate(t *testing.T) {
	line := "café" // é = U+00E9: 2 bytes, 1 UTF-16 unit. Byte layout: c a f é(2 bytes) = len 5.
	if got := utf16ToByte(line, 3); got != 3 {
		t.Errorf("utf16ToByte(%q, 3) = %d, want 3 (right before é)", line, got)
	}
	if got := utf16ToByte(line, 4); got != 5 {
		t.Errorf("utf16ToByte(%q, 4) = %d, want 5 (end of line, past é's 2 bytes)", line, got)
	}
	if got := byteToUTF16(line, 5); got != 4 {
		t.Errorf("byteToUTF16(%q, 5) = %d, want 4", line, got)
	}
}

func TestUTF16ByteConversion_SurrogatePair(t *testing.T) {
	line := "🚀x" // U+1F680: 4 bytes, 2 UTF-16 units (a surrogate pair). Byte layout: 🚀(4 bytes) x = len 5.
	if got := utf16ToByte(line, 2); got != 4 {
		t.Errorf("utf16ToByte(%q, 2) = %d, want 4 (right after the surrogate pair)", line, got)
	}
	if got := byteToUTF16(line, 4); got != 2 {
		t.Errorf("byteToUTF16(%q, 4) = %d, want 2", line, got)
	}
	// A UTF-16 column landing inside the surrogate pair (col 1) has no exact
	// byte boundary; must not panic and must stay within the line's bounds.
	if got := utf16ToByte(line, 1); got < 0 || got > len(line) {
		t.Errorf("utf16ToByte(%q, 1) = %d, out of range [0,%d]", line, got, len(line))
	}
}

// TestMapColumn_UTF16VaneColumn_NonASCIIPrefix is the regression test for the
// real bug found while migrating column mapping to the market-standard
// UTF-16 design: mapColumn indexed doc.vaneLines[vaneLine] with the raw LSP
// column (always UTF-16) as if it were a byte offset. "café " puts a 2-byte,
// 1-UTF16-unit rune before the target identifier, so the two diverge by
// exactly 1, before the utf16ToByte fix, this landed identAt one byte
// short of "handleClick" (inside the preceding '{'), matched nothing, and
// mapColumn fell through to its no-op fallback instead of finding the
// identifier in the generated Go line.
func TestMapColumn_UTF16VaneColumn_NonASCIIPrefix(t *testing.T) {
	vaneLine := `café {handleClick}`
	const ident = "handleClick"
	byteIdx := strings.Index(vaneLine, ident)
	if byteIdx < 0 {
		t.Fatalf("fixture missing %q: %q", ident, vaneLine)
	}
	utf16Col := 0
	for i := 0; i < byteIdx; {
		r, size := utf8.DecodeRuneInString(vaneLine[i:])
		if r > 0xFFFF {
			utf16Col += 2
		} else {
			utf16Col++
		}
		i += size
	}
	if utf16Col == byteIdx {
		t.Fatalf("fixture invalid: byte offset and UTF-16 column coincide (%d), doesn't exercise the divergence", byteIdx)
	}

	doc := &document{
		vaneLines: []string{vaneLine},
		goLines:   []string{"core.OnClick(_vane1, " + ident + ")"},
	}
	gl, gc := mapColumn(doc, 0, utf16Col, 0, 0)
	if gl != 0 {
		t.Fatalf("mapColumn moved to unexpected go line %d", gl)
	}
	goByteCol := utf16ToByte(doc.goLines[0], gc)
	if !strings.HasPrefix(doc.goLines[0][goByteCol:], ident) {
		t.Errorf("mapColumn(vaneCol=%d) = goCol %d, which doesn't point at %q in go line %q",
			utf16Col, gc, ident, doc.goLines[0])
	}
}
