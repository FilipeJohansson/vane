//go:build js && wasm

package core_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.
//
// Covers DynChild, DynText, DynProp, DynStyle, the fine-grained reactive
// DOM bindings. DynList is covered separately in reactive_dom_test.go.

import (
	"strconv"
	"testing"
	"time"

	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/core/signal"
)

func waitFineEffects(t *testing.T) {
	t.Helper()
	if !signal.WaitEffects(time.Second) {
		t.Fatal("effects did not settle within timeout")
	}
}

// firstElementChild returns parent's first Element child, skipping the
// anchor comment node DynChild inserts before its rendered content.
func firstElementChild(t *testing.T, parent core.Node) string {
	t.Helper()
	raw := core.Unwrap(parent).Get("firstElementChild")
	if raw.IsNull() {
		return ""
	}
	return raw.Get("tagName").String()
}

func TestDynChildRendersInitialNodeValue(t *testing.T) {
	parent := core.El("div")
	core.DynChild(parent, func() any { return core.El("span") })

	if got := firstElementChild(t, parent); got != "SPAN" {
		t.Errorf("first element child = %q, want SPAN", got)
	}
}

func TestDynChildRendersNonNodeValueAsText(t *testing.T) {
	parent := core.El("div")
	core.DynChild(parent, func() any { return 42 })

	if got := core.Unwrap(parent).Get("textContent").String(); got != "42" {
		t.Errorf("textContent = %q, want %q", got, "42")
	}
}

func TestDynChildRendersEmptyNodeAsBlankText(t *testing.T) {
	parent := core.El("div")
	core.DynChild(parent, func() any { return core.Empty() })

	if got := core.Unwrap(parent).Get("textContent").String(); got != "" {
		t.Errorf("textContent = %q, want empty", got)
	}
}

func TestDynChildReplacesNodeOnSignalChange(t *testing.T) {
	view := core.NewSignal("a")
	parent := core.El("div")
	core.DynChild(parent, func() any { return core.Text(view.Get()) })

	if got := core.Unwrap(parent).Get("textContent").String(); got != "a" {
		t.Fatalf("initial textContent = %q, want %q", got, "a")
	}

	view.Set("b")
	waitFineEffects(t)

	if got := core.Unwrap(parent).Get("textContent").String(); got != "b" {
		t.Errorf("textContent after signal change = %q, want %q", got, "b")
	}
}

// TestDynChildOperatesOnCurrentSiblingEvenIfReplacedExternally verifies the
// comment-anchor design: DynChild tracks anchor.nextSibling, not the original
// node reference, so it keeps working even if a third-party library (e.g.
// Lucide) replaces the rendered node with a different one (e.g. <i> → <svg>).
func TestDynChildOperatesOnCurrentSiblingEvenIfReplacedExternally(t *testing.T) {
	view := core.NewSignal(1)
	parent := core.El("div")
	core.DynChild(parent, func() any {
		_ = view.Get()
		return core.El("i")
	})

	raw := core.Unwrap(parent)
	original := raw.Get("firstElementChild")
	if got := original.Get("tagName").String(); got != "I" {
		t.Fatalf("initial child tagName = %q, want I", got)
	}

	// Simulate a third-party library replacing the node in place, the same
	// way Lucide swaps <i data-lucide> for a rendered <svg>.
	replacement := core.Unwrap(core.El("svg"))
	raw.Call("replaceChild", replacement, original)

	view.Set(2) // trigger a re-run
	waitFineEffects(t)

	// DynChild must have found and removed the <svg> (current sibling), not
	// tried to operate on the stale, now-detached <i> reference.
	if got := raw.Get("firstElementChild").Get("tagName").String(); got != "I" {
		t.Errorf("firstElementChild tagName = %q, want I (DynChild should have replaced the externally-swapped <svg>)", got)
	}
	if raw.Get("children").Get("length").Int() != 1 {
		t.Errorf("children.length = %d, want 1 (no leftover duplicate nodes)", raw.Get("children").Get("length").Int())
	}
}

// TestDynChildReplacesAllNodesOfMultiRootContent regression-tests a bug found
// while adding core.DangerousInnerHTML: the old single-comment-anchor design
// only ever removed anchor.nextSibling (one node) on re-run. If fn() returns
// multi-root content (e.g. a Fragment, or DangerousInnerHTML parsing markup
// with several top-level tags), every node after the first was orphaned in
// the DOM on each reactive update instead of being replaced.
func TestDynChildReplacesAllNodesOfMultiRootContent(t *testing.T) {
	html := core.NewSignal("<p>one</p><p>two</p>")
	parent := core.El("div")
	core.DynChild(parent, func() any {
		return core.DangerousInnerHTML(html.Get())
	})

	raw := core.Unwrap(parent)
	if got := raw.Get("children").Get("length").Int(); got != 2 {
		t.Fatalf("initial children.length = %d, want 2", got)
	}

	html.Set("<p>three</p><p>four</p>")
	waitFineEffects(t)

	if got := raw.Get("children").Get("length").Int(); got != 2 {
		t.Fatalf("children.length after update = %d, want 2 (found orphaned nodes): %s", got, raw.Get("outerHTML").String())
	}
	if got := raw.Get("children").Index(0).Get("textContent").String(); got != "three" {
		t.Errorf("children[0].textContent = %q, want %q", got, "three")
	}
	if got := raw.Get("children").Index(1).Get("textContent").String(); got != "four" {
		t.Errorf("children[1].textContent = %q, want %q", got, "four")
	}
}

func TestDynTextUpdatesDataInPlaceWithoutReplacingNode(t *testing.T) {
	count := core.NewSignal(0)
	parent := core.El("span")
	core.DynText(parent, func() string { return "count: " + strconv.Itoa(count.Get()) })

	raw := core.Unwrap(parent)
	textNode := raw.Get("firstChild")
	if got := textNode.Get("data").String(); got != "count: 0" {
		t.Fatalf("initial data = %q, want %q", got, "count: 0")
	}

	count.Set(1)
	waitFineEffects(t)

	if got := raw.Get("firstChild").Get("data").String(); got != "count: 1" {
		t.Errorf("data after signal change = %q, want %q", got, "count: 1")
	}
	// Same node instance, DynText updates .data in place, no replacement.
	if !raw.Get("firstChild").Equal(textNode) {
		t.Error("DynText replaced the text node instead of updating .data in place")
	}
}

func TestDynPropUpdatesReactively(t *testing.T) {
	active := core.NewSignal(false)
	el := core.El("div")
	core.DynProp(el, "className", func() any {
		if active.Get() {
			return "active"
		}
		return "inactive"
	})

	if got := core.Unwrap(el).Get("className").String(); got != "inactive" {
		t.Fatalf("initial className = %q, want %q", got, "inactive")
	}

	active.Set(true)
	waitFineEffects(t)

	if got := core.Unwrap(el).Get("className").String(); got != "active" {
		t.Errorf("className after signal change = %q, want %q", got, "active")
	}
}

func TestDynStyleAppliesReactivelyAndClearsPreviousFields(t *testing.T) {
	hue := core.NewSignal("red")
	el := core.El("div")
	core.DynStyle(el, func() core.Style {
		if hue.Get() == "" {
			return core.Style{} // no Color set at all
		}
		return core.Style{Color: hue.Get(), Display: "flex"}
	})

	style := core.Unwrap(el).Get("style")
	if got := style.Get("color").String(); got != "red" {
		t.Fatalf("initial color = %q, want %q", got, "red")
	}

	hue.Set("") // next run sets no fields at all
	waitFineEffects(t)

	// DynStyle clears cssText before each re-run, so a field the new Style
	// leaves unset must actually disappear, not keep the previous value.
	if got := style.Get("color").String(); got != "" {
		t.Errorf("color = %q, want empty (DynStyle must clear stale fields on re-run)", got)
	}
	if got := style.Get("display").String(); got != "" {
		t.Errorf("display = %q, want empty", got)
	}
}
