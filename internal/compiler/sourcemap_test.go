package compiler_test

import (
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/filipejohansson/vane/internal/compiler"
)

// utf16ColOf independently computes the 0-based UTF-16 code-unit column of
// byteIdx within line, a second, from-scratch implementation used only to
// derive expected test values, so these tests don't just re-check the
// production colAt against itself.
func utf16ColOf(line string, byteIdx int) int {
	col := 0
	for i := 0; i < byteIdx && i < len(line); {
		r, size := utf8.DecodeRuneInString(line[i:])
		col += len(utf16.Encode([]rune{r}))
		i += size
	}
	return col
}

// braceFixture returns a single-element .vane program with one onClick={...}
// attribute expression, plus the 0-based vane line and UTF-16 column of the
// '{' that starts it - the exact position compiler.go's emitAttr anchors
// its //line directive to (attr.valuePos). Anchored on the "onClick=" marker
// specifically (not just the first '{' in the file) since wrap()'s own
// boilerplate ("func F() js.Value {") contains an unrelated '{' first.
func braceFixture(t *testing.T, attrLine string) (src string, vaneLine, vaneCol int) {
	t.Helper()
	src = wrap(attrLine)
	lines := strings.Split(src, "\n")
	const marker = "onClick="
	for i, l := range lines {
		mi := strings.Index(l, marker)
		if mi < 0 {
			continue
		}
		braceIdx := mi + len(marker)
		if braceIdx >= len(l) || l[braceIdx] != '{' {
			t.Fatalf("fixture line %q has no '{' right after %q", l, marker)
		}
		return src, i, utf16ColOf(l, braceIdx)
	}
	t.Fatalf("fixture has no %q to anchor on: %q", marker, attrLine)
	return "", 0, 0
}

func TestSourceMap_VaneToGoGoToVane_ASCIIRoundTrip(t *testing.T) {
	src, vaneLine, vaneCol := braceFixture(t, `<div onClick={handleClick}>hi</div>`)
	_, sm, err := compiler.CompileWithMap(src, "test.vane")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	goLine, goCol, ok := sm.VaneToGo(vaneLine, vaneCol)
	if !ok {
		t.Fatalf("VaneToGo(%d, %d) not ok", vaneLine, vaneCol)
	}
	// The directive anchored at the '{' always carries GoCol == 0 (a //line
	// directive only ever marks the start of the following Go line - see
	// PosEntry's doc comment), so querying at the exact anchor column must
	// return goCol == 0. A passthrough implementation that just forwards
	// vaneCol unchanged (the pre-column-mapping behavior) would instead
	// return goCol == vaneCol, a much larger number on this fixture, this
	// assertion is what actually catches that regression; the round trip
	// below alone would pass vacuously even under plain passthrough.
	if goCol != 0 {
		t.Errorf("VaneToGo(%d, %d) = goCol %d, want 0 (the '{' anchor's own column)", vaneLine, vaneCol, goCol)
	}
	backLine, backCol, ok := sm.GoToVane(goLine, goCol)
	if !ok {
		t.Fatalf("GoToVane(%d, %d) not ok", goLine, goCol)
	}
	if backLine != vaneLine || backCol != vaneCol {
		t.Errorf("round trip mismatch: started at (%d,%d), got back (%d,%d) via go (%d,%d)",
			vaneLine, vaneCol, backLine, backCol, goLine, goCol)
	}
}

func TestSourceMap_VaneToGo_ExactColumnNotJustLine(t *testing.T) {
	// Two attributes on the same vane line anchor two different //line
	// directives at two different columns; VaneToGo must resolve each
	// column to its own directive, not collapse both to whichever one was
	// emitted last for that line.
	src := wrap(`<div id="a" onClick={handleClick}>hi</div>`)
	_, sm, err := compiler.CompileWithMap(src, "test.vane")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	lines := strings.Split(src, "\n")
	var vaneLine, idCol, braceCol int
	found := false
	for i, l := range lines {
		idIdx := strings.Index(l, `"a"`)
		braceIdx := strings.Index(l, "onClick={")
		if idIdx < 0 || braceIdx < 0 {
			continue
		}
		vaneLine = i
		idCol = utf16ColOf(l, idIdx)
		braceCol = utf16ColOf(l, braceIdx+len("onClick="))
		found = true
		break
	}
	if !found {
		t.Fatalf("fixture line with both id=\"a\" and onClick={...} not found")
	}

	idGoLine, _, ok := sm.VaneToGo(vaneLine, idCol)
	if !ok {
		t.Fatalf("VaneToGo for id attr not ok")
	}
	braceGoLine, _, ok := sm.VaneToGo(vaneLine, braceCol)
	if !ok {
		t.Fatalf("VaneToGo for onClick attr not ok")
	}
	if idGoLine == braceGoLine {
		t.Errorf("expected id=\"a\" and onClick={...} to resolve to different go lines (they're different directives on the same vane line), both got %d", idGoLine)
	}
}

func TestSourceMap_UTF16ColumnNotByteOffset(t *testing.T) {
	// "café" puts a 2-byte, 1-UTF16-unit rune (é) before the '{' anchor, so
	// the '{' byte offset and its UTF-16 column diverge by exactly 1. If
	// colAt (used when the directive was emitted) were byte-based instead of
	// UTF-16, VaneToGo at the true UTF-16 column would land one column
	// short of the real anchor and the round trip below would fail.
	attrLine := `<div title="café" onClick={handleClick}>hi</div>`
	line := "\t\t" + attrLine // wrap()'s own indent before the fixture content
	byteIdx := strings.IndexByte(line, '{')
	utf16Idx := utf16ColOf(line, byteIdx)
	if utf16Idx == byteIdx {
		t.Fatalf("fixture invalid: byte and UTF-16 offsets coincide (%d), doesn't exercise the divergence", byteIdx)
	}

	src, vaneLine, vaneCol := braceFixture(t, attrLine)
	if vaneCol != utf16Idx {
		t.Fatalf("braceFixture returned col %d, want %d", vaneCol, utf16Idx)
	}
	_, sm, err := compiler.CompileWithMap(src, "test.vane")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	goLine, goCol, ok := sm.VaneToGo(vaneLine, vaneCol)
	if !ok {
		t.Fatalf("VaneToGo(%d, %d) not ok", vaneLine, vaneCol)
	}
	// See TestSourceMap_VaneToGoGoToVane_ASCIIRoundTrip: goCol == 0 is what
	// actually proves real UTF-16-aware translation happened here, not just
	// that the round trip below is internally consistent, a passthrough
	// implementation would round-trip fine too while returning the wrong
	// (byte- or rune-based) column throughout.
	if goCol != 0 {
		t.Errorf("VaneToGo(%d, %d) = goCol %d, want 0 (the '{' anchor's own column)", vaneLine, vaneCol, goCol)
	}
	backLine, backCol, ok := sm.GoToVane(goLine, goCol)
	if !ok || backLine != vaneLine || backCol != vaneCol {
		t.Errorf("round trip at the true UTF-16 column failed: got back (%d,%d,%v), want (%d,%d,true)",
			backLine, backCol, ok, vaneLine, vaneCol)
	}
}

func TestSourceMap_SurrogatePairBeforeColumn(t *testing.T) {
	// An emoji outside the Basic Multilingual Plane costs 2 UTF-16 code
	// units (a surrogate pair) but only 1 rune - the classic off-by-one a
	// rune-index-based (rather than UTF-16-based) implementation gets wrong.
	attrLine := `<div title="🚀x" onClick={handleClick}>hi</div>`
	line := "\t\t" + attrLine // wrap()'s own indent before the fixture content
	byteIdx := strings.IndexByte(line, '{')
	runeIdx := utf8.RuneCountInString(line[:byteIdx])
	utf16Idx := utf16ColOf(line, byteIdx)
	if utf16Idx == runeIdx {
		t.Fatalf("fixture invalid: UTF-16 and rune offsets coincide (%d), doesn't exercise the surrogate-pair divergence", runeIdx)
	}

	src, vaneLine, vaneCol := braceFixture(t, attrLine)
	if vaneCol != utf16Idx {
		t.Fatalf("braceFixture returned col %d, want UTF-16 col %d (rune col would incorrectly be %d)", vaneCol, utf16Idx, runeIdx)
	}
	_, sm, err := compiler.CompileWithMap(src, "test.vane")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	goLine, goCol, ok := sm.VaneToGo(vaneLine, vaneCol)
	if !ok {
		t.Fatalf("VaneToGo(%d, %d) not ok", vaneLine, vaneCol)
	}
	// See TestSourceMap_VaneToGoGoToVane_ASCIIRoundTrip: goCol == 0 is what
	// actually proves real UTF-16-aware translation happened here, not just
	// that the round trip below is internally consistent - a passthrough
	// implementation would round-trip fine too while returning the wrong
	// (byte- or rune-based) column throughout.
	if goCol != 0 {
		t.Errorf("VaneToGo(%d, %d) = goCol %d, want 0 (the '{' anchor's own column)", vaneLine, vaneCol, goCol)
	}
	backLine, backCol, ok := sm.GoToVane(goLine, goCol)
	if !ok || backLine != vaneLine || backCol != vaneCol {
		t.Errorf("round trip past a surrogate pair failed: got back (%d,%d,%v), want (%d,%d,true)",
			backLine, backCol, ok, vaneLine, vaneCol)
	}
}

func TestSourceMap_VaneToGo_EmptyVaneLine(t *testing.T) {
	sm := compiler.SourceMap{}
	if _, _, ok := sm.VaneToGo(0, 0); ok {
		t.Errorf("expected ok=false for an empty SourceMap")
	}
	if _, _, ok := sm.GoToVane(0, 0); ok {
		t.Errorf("expected ok=false for an empty SourceMap")
	}
}

func TestSourceMap_VaneToGo_ColumnZeroAndLineStart(t *testing.T) {
	src, vaneLine, _ := braceFixture(t, `<div onClick={handleClick}>hi</div>`)
	_, sm, err := compiler.CompileWithMap(src, "test.vane")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	// Column 0 on a mapped line must still resolve (no panic, ok=true),
	// even though it precedes the attribute's own anchor column.
	goLine, _, ok := sm.VaneToGo(vaneLine, 0)
	if !ok {
		t.Fatalf("VaneToGo(%d, 0) not ok", vaneLine)
	}
	if goLine < 0 {
		t.Errorf("VaneToGo(%d, 0) returned negative goLine %d", vaneLine, goLine)
	}
}
