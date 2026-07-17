//go:build js && wasm

package core_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.

import (
	"testing"

	"github.com/filipejohansson/vane/core"
)

func TestDangerousInnerHTMLParsesMarkupIntoRealNodes(t *testing.T) {
	parent := core.El("div")
	core.AppendChild(parent, core.DangerousInnerHTML("<b>bold</b><i>italic</i>"))

	raw := core.Unwrap(parent)
	if got := raw.Get("children").Get("length").Int(); got != 2 {
		t.Fatalf("children.length = %d, want 2", got)
	}
	if got := raw.Call("querySelector", "b").Get("textContent").String(); got != "bold" {
		t.Errorf("<b> textContent = %q, want %q", got, "bold")
	}
	if got := raw.Call("querySelector", "i").Get("textContent").String(); got != "italic" {
		t.Errorf("<i> textContent = %q, want %q", got, "italic")
	}
}

func TestDangerousInnerHTMLLeavesNoWrapperElement(t *testing.T) {
	parent := core.El("div")
	core.AppendChild(parent, core.DangerousInnerHTML("<span>only child</span>"))

	raw := core.Unwrap(parent)
	// Only the parsed <span> should be present, no <template>, no
	// intermediate wrapper node from the parsing mechanism.
	if got := raw.Get("children").Get("length").Int(); got != 1 {
		t.Fatalf("children.length = %d, want 1 (no wrapper element)", got)
	}
	if got := raw.Get("firstElementChild").Get("tagName").String(); got != "SPAN" {
		t.Errorf("firstElementChild.tagName = %q, want SPAN", got)
	}
}

func TestDangerousInnerHTMLMixesWithOtherVaneSiblings(t *testing.T) {
	// The whole point of this over a plain innerHTML prop: raw markup as one
	// child among several others, without wiping out its siblings.
	parent := core.El("div")
	core.AppendChild(parent, core.El("h1"))
	core.AppendChild(parent, core.DangerousInnerHTML("<p>raw paragraph</p>"))
	core.AppendChild(parent, core.El("footer"))

	raw := core.Unwrap(parent)
	if got := raw.Get("children").Get("length").Int(); got != 3 {
		t.Fatalf("children.length = %d, want 3 (h1, p, footer)", got)
	}
	tags := []string{"H1", "P", "FOOTER"}
	for i, want := range tags {
		if got := raw.Get("children").Index(i).Get("tagName").String(); got != want {
			t.Errorf("children[%d].tagName = %q, want %q", i, got, want)
		}
	}
}

func TestDangerousInnerHTMLSupportsMultipleRootNodes(t *testing.T) {
	parent := core.El("div")
	core.AppendChild(parent, core.DangerousInnerHTML("<p>one</p><p>two</p><p>three</p>"))

	raw := core.Unwrap(parent)
	if got := raw.Get("children").Get("length").Int(); got != 3 {
		t.Errorf("children.length = %d, want 3 (multiple root-level tags must all be inserted)", got)
	}
}

func TestDangerousInnerHTMLEmptyStringRendersNothing(t *testing.T) {
	parent := core.El("div")
	core.AppendChild(parent, core.DangerousInnerHTML(""))

	raw := core.Unwrap(parent)
	if got := raw.Get("childNodes").Get("length").Int(); got != 0 {
		t.Errorf("childNodes.length = %d, want 0", got)
	}
}

// TestDangerousInnerHTMLScriptTagsAreParsedButInert documents a real,
// easy-to-miss footgun: <script> parsed via innerHTML is inert. It's
// present in the DOM but never executes (a documented quirk of HTML
// parsing, not a vane-specific safety net). This does NOT mean the function
// is safe against XSS: event handler attributes (onerror=, onclick=, etc.)
// on other elements still execute normally.
func TestDangerousInnerHTMLScriptTagsAreParsedButInert(t *testing.T) {
	parent := core.El("div")
	core.AppendChild(parent, core.DangerousInnerHTML(`<script>window.__vane_test_marker = true;</script>`))

	raw := core.Unwrap(parent)
	if !raw.Call("querySelector", "script").Truthy() {
		t.Fatal("expected a <script> element to be present in the DOM (inert, but still parsed)")
	}
}
