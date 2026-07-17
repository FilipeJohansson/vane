//go:build js && wasm

package core_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.

import (
	"testing"

	"github.com/filipejohansson/vane/core"
)

func tagName(t *testing.T, n core.Node) string {
	t.Helper()
	return core.Unwrap(n).Get("tagName").String()
}

func TestElCreatesElementWithTag(t *testing.T) {
	el := core.El("button")
	if got := tagName(t, el); got != "BUTTON" {
		t.Errorf("tagName = %q, want %q", got, "BUTTON")
	}
}

func TestTextCreatesTextNode(t *testing.T) {
	n := core.Text("hello")
	raw := core.Unwrap(n)
	const textNodeType = 3 // Node.TEXT_NODE
	if got := raw.Get("nodeType").Int(); got != textNodeType {
		t.Errorf("nodeType = %d, want %d (TEXT_NODE)", got, textNodeType)
	}
	if got := raw.Get("textContent").String(); got != "hello" {
		t.Errorf("textContent = %q, want %q", got, "hello")
	}
}

func TestAppendChildAppendsInOrder(t *testing.T) {
	parent := core.El("ul")
	core.AppendChild(parent, core.El("li"))
	core.AppendChild(parent, core.El("li"))

	raw := core.Unwrap(parent)
	if got := raw.Get("children").Get("length").Int(); got != 2 {
		t.Fatalf("children.length = %d, want 2", got)
	}
}

func TestAppendChildNoOpsOnEmptyNode(t *testing.T) {
	parent := core.El("div")
	core.AppendChild(parent, core.Empty())

	raw := core.Unwrap(parent)
	if got := raw.Get("childNodes").Get("length").Int(); got != 0 {
		t.Errorf("childNodes.length = %d, want 0 (Empty() must no-op)", got)
	}
}

func TestAppendTextAppendsTextNode(t *testing.T) {
	parent := core.El("p")
	core.AppendText(parent, "world")

	raw := core.Unwrap(parent)
	if got := raw.Get("textContent").String(); got != "world" {
		t.Errorf("textContent = %q, want %q", got, "world")
	}
}

func TestFragmentGroupsChildrenWithoutWrapper(t *testing.T) {
	frag := core.Fragment(core.El("span"), core.El("span"))
	parent := core.El("div")
	core.AppendChild(parent, frag)

	// A DocumentFragment's children move into parent on append, no wrapper
	// node remains, parent should have exactly the fragment's 2 children.
	raw := core.Unwrap(parent)
	if got := raw.Get("children").Get("length").Int(); got != 2 {
		t.Errorf("children.length = %d, want 2 (fragment should leave no wrapper)", got)
	}
}

func TestFragmentSkipsNilNodes(t *testing.T) {
	frag := core.Fragment(core.El("span"), core.Empty(), core.El("span"))
	parent := core.El("div")
	core.AppendChild(parent, frag)

	raw := core.Unwrap(parent)
	if got := raw.Get("children").Get("length").Int(); got != 2 {
		t.Errorf("children.length = %d, want 2 (Empty() inside Fragment must be skipped)", got)
	}
}

func TestNodesNormalizesMixedInput(t *testing.T) {
	nodes := core.Nodes("plain string", core.El("span"), []core.Node{core.El("i"), core.El("b")})
	if len(nodes) != 4 {
		t.Fatalf("len(nodes) = %d, want 4", len(nodes))
	}
	if tag := tagName(t, nodes[1]); tag != "SPAN" {
		t.Errorf("nodes[1] tag = %q, want SPAN", tag)
	}
	if tag := tagName(t, nodes[2]); tag != "I" {
		t.Errorf("nodes[2] tag = %q, want I", tag)
	}
}

func TestNodesConvertsStringToText(t *testing.T) {
	nodes := core.Nodes("hi")
	if len(nodes) != 1 {
		t.Fatalf("len(nodes) = %d, want 1", len(nodes))
	}
	if got := core.Unwrap(nodes[0]).Get("textContent").String(); got != "hi" {
		t.Errorf("textContent = %q, want %q", got, "hi")
	}
}

func TestSpreadHandlesNodeSliceAndAnySlice(t *testing.T) {
	fromNodeSlice := core.Spread([]core.Node{core.El("a"), core.El("b")})
	if len(fromNodeSlice) != 2 {
		t.Errorf("Spread([]core.Node) len = %d, want 2", len(fromNodeSlice))
	}

	fromAnySlice := core.Spread([]any{"text", core.El("a")})
	if len(fromAnySlice) != 2 {
		t.Errorf("Spread([]any) len = %d, want 2", len(fromAnySlice))
	}
}

func TestSpreadReturnsNilForUnsupportedType(t *testing.T) {
	if got := core.Spread(42); got != nil {
		t.Errorf("Spread(42) = %v, want nil", got)
	}
}
