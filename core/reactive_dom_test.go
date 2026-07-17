//go:build js && wasm

package core_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.

import (
	"syscall/js"
	"testing"

	"github.com/filipejohansson/vane/core"
)

// jsGlobalConsole returns the global console object, for tests that need to
// intercept core.Warn's console.warn calls.
func jsGlobalConsole(t *testing.T) js.Value {
	t.Helper()
	return js.Global().Get("console")
}

// jsFuncCapture returns a js.Func that appends the first argument (as a
// string) to *calls each time it's invoked, used to spy on console.warn.
func jsFuncCapture(t *testing.T, calls *[]string) js.Func {
	t.Helper()
	f := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			*calls = append(*calls, args[0].String())
		}
		return nil
	})
	t.Cleanup(f.Release)
	return f
}

func childTexts(t *testing.T, parent core.Node) []string {
	t.Helper()
	raw := core.Unwrap(parent)
	children := raw.Get("childNodes")
	n := children.Get("length").Int()
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, children.Index(i).Get("textContent").String())
	}
	return out
}

// TestDynListKeyLengthMismatchFallsBackToUnkeyed verifies that an explicit key
// function returning a different number of keys than nodes doesn't panic
// (indexing newNodes[i] out of range). It degrades to unkeyed rendering for
// that update instead, and every node still renders.
func TestDynListKeyLengthMismatchFallsBackToUnkeyed(t *testing.T) {
	parent := core.El("ul")

	nodes := func() []core.Node {
		return []core.Node{core.Text("a"), core.Text("b"), core.Text("c")}
	}
	badKeys := func() []string { return []string{"only-one-key"} } // 1 key, 3 nodes

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("DynList panicked on key/node length mismatch: %v", r)
		}
	}()
	core.DynList(parent, nodes, badKeys)

	got := childTexts(t, parent)
	want := []string{"", "a", "b", "c", ""} // start/end marker text nodes are empty
	if len(got) != len(want) {
		t.Fatalf("childTexts = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("child[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestDynListMixedKeyedUnkeyedRendersAll verifies that when some nodes have
// key={...} and others don't, every node still renders. The old behavior
// silently dropped the unkeyed ones (key == "" was treated as "skip").
func TestDynListMixedKeyedUnkeyedRendersAll(t *testing.T) {
	parent := core.El("ul")

	keyed := core.El("li")
	core.Unwrap(keyed).Set("key", "k1")
	core.AppendText(keyed, "keyed")

	unkeyed := core.El("li")
	core.AppendText(unkeyed, "unkeyed")

	nodes := func() []core.Node { return []core.Node{keyed, unkeyed} }
	core.DynList(parent, nodes)

	raw := core.Unwrap(parent)
	liCount := raw.Call("querySelectorAll", "li").Get("length").Int()
	if liCount != 2 {
		t.Fatalf("rendered %d <li> elements, want 2 (unkeyed sibling must not be dropped)", liCount)
	}
}

// TestPortalNilFnRendersEmptyInsteadOfPanicking verifies core.Portal no
// longer panics when called with a nil render function.
func TestPortalNilFnRendersEmptyInsteadOfPanicking(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("core.Portal panicked on nil fn: %v", r)
		}
	}()
	result := core.Portal("#does-not-matter", nil)
	if core.Unwrap(result).Truthy() && !core.Unwrap(result).IsUndefined() {
		// Empty() wraps js.Undefined(): Truthy() is false for it, so this
		// branch only fires if Portal returned something unexpected.
		t.Errorf("expected an empty Node, got a truthy value")
	}
}

// TestWarnDedupesPerDistinctMessage verifies core.Warn logs each distinct
// message once, but different messages both get through (not a single
// global "warned once ever" flag).
func TestWarnDedupesPerDistinctMessage(t *testing.T) {
	var calls []string
	raw := jsGlobalConsole(t)
	orig := raw.Get("warn")
	raw.Set("warn", jsFuncCapture(t, &calls))
	defer raw.Set("warn", orig)

	msgA := "wasmtest: dedup message A " + t.Name()
	msgB := "wasmtest: dedup message B " + t.Name()

	core.Warn(msgA)
	core.Warn(msgA) // duplicate, must not log again
	core.Warn(msgB) // distinct message, must log

	if len(calls) != 2 {
		t.Fatalf("console.warn called %d times, want 2 (one per distinct message): %v", len(calls), calls)
	}
}
