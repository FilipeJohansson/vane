package compiler

import "testing"

// hintForOffset is unexported, so this test lives in package compiler itself
// rather than compiler_test - the only white-box test in this package
// (compiler_test.go/sourcemap_test.go both test the public API). Codegen
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
