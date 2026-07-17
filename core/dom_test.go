//go:build js && wasm

package core_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.

import (
	"testing"

	"github.com/filipejohansson/vane/core"
)

// TestDOMHarnessSmoke validates the jsdom exec wrapper end-to-end before
// trusting any other test in this file: create an element, append text,
// mount it, and read it back.
func TestDOMHarnessSmoke(t *testing.T) {
	el := core.El("div")
	core.AppendText(el, "hello")
	raw := core.Unwrap(el)
	if got := raw.Get("textContent").String(); got != "hello" {
		t.Fatalf("textContent = %q, want %q", got, "hello")
	}
}
