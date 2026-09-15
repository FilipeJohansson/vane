package compiler

import "testing"

// hintForOffset is unexported, so this test lives in package compiler itself
// rather than compiler_test (compiler_test.go/sourcemap_test.go test the
// public API; this file is for internals with no public wrapper). Codegen
// doesn't consume a matched hint yet; this only verifies the lookup itself.
func TestHintForOffset(t *testing.T) {
	em := &emitter{
		hints: []ForTypeHint{
			{Offset: 10, Type: "ToDo"},
			{Offset: 42, Type: "otherpkg.Row"},
		},
	}

	if h, ok := em.hintForOffset(42); !ok || h.Type != "otherpkg.Row" {
		t.Fatalf("hintForOffset(42) = %+v, %v, want {42, otherpkg.Row}, true", h, ok)
	}
	if h, ok := em.hintForOffset(10); !ok || h.Type != "ToDo" {
		t.Fatalf("hintForOffset(10) = %+v, %v, want {10, ToDo}, true", h, ok)
	}
	if _, ok := em.hintForOffset(99); ok {
		t.Fatal("hintForOffset(99) = true, want false (no hint at that offset)")
	}
	if _, ok := (&emitter{}).hintForOffset(0); ok {
		t.Fatal("hintForOffset on an emitter with no hints = true, want false")
	}
}

// TestIndexForKeyword covers indexForKeyword directly, since its own single
// call site (emitForCtrl's forward search for a keyed {for}'s "for"
// keyword) can never actually exercise the comment/string-skipping or
// identifier-boundary paths: isControlFlow's own gate only recognizes a
// block as a for-loop when its trimmed content literally starts with
// "for ", so nothing but whitespace can ever precede the real keyword by
// the time this search runs. Tested here as the general-purpose scanning
// utility it is, independent of that one caller's own narrower reality.
func TestIndexForKeyword(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantIdx int
		wantOK  bool
	}{
		{"immediate match", "for _, t := range items", 0, true},
		{"leading comment", "/* wait for update */ for _, t := range items", 22, true},
		{"leading line comment", "// for later\nfor _, t := range items", 13, true},
		{"leading string with for in it", `"before for" + for _, t := range items`, 15, true},
		{"identifier substring not a match", "forceUpdate(); x := 1", 0, false},
		{"not present at all", "if x { y() }", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			idx, ok := indexForKeyword(tc.src)
			if ok != tc.wantOK || (ok && idx != tc.wantIdx) {
				t.Errorf("indexForKeyword(%q) = (%d, %v), want (%d, %v)", tc.src, idx, ok, tc.wantIdx, tc.wantOK)
			}
		})
	}
}
