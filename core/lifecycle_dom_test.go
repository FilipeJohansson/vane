//go:build js && wasm

package core_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.
//
// These cover the end-to-end lifecycle claim: mount -> create effects,
// listeners, and reactive bindings -> unmount -> verify every one of those
// resources is gone, not just the DOM nodes.

import (
	"testing"
	"time"

	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/core/signal"
)

// TestMountThenUnmountDisposesEffectListenerAndBinding mounts a node with an
// effect, a click listener, and a reactive text binding all owned by the
// same Scope, then disposes that Scope and verifies each resource actually
// stops: the effect no longer runs, the listener is released, and the
// binding no longer updates the DOM.
func TestMountThenUnmountDisposesEffectListenerAndBinding(t *testing.T) {
	label := core.NewSignal("first")
	effectRuns := 0
	clickCount := 0

	span := core.El("span")
	btn := core.El("button")

	callbacksBefore := core.DebugLiveCallbacks()
	effectsBefore := signal.LiveEffectCount()

	scope := signal.RunScoped(func() {
		signal.Effect(func() {
			_ = label.Get()
			effectRuns++
		})
		core.OnClick(btn, func(core.MouseEvent) { clickCount++ })
		core.DynText(span, func() string { return label.Get() })
	})

	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after mount")
	}
	if effectRuns != 1 {
		t.Fatalf("effectRuns = %d after mount, want 1", effectRuns)
	}
	if got := core.Unwrap(span).Get("textContent").String(); got != "first" {
		t.Fatalf("span textContent = %q after mount, want %q", got, "first")
	}
	if got := core.DebugLiveCallbacks(); got != callbacksBefore+1 {
		t.Fatalf("DebugLiveCallbacks() = %d after mount, want %d", got, callbacksBefore+1)
	}

	scope.Dispose()

	if got := signal.LiveEffectCount(); got != effectsBefore {
		t.Fatalf("LiveEffectCount() = %d after unmount, want %d (mounted effects were not disposed)", got, effectsBefore)
	}
	if got := core.DebugLiveCallbacks(); got != callbacksBefore {
		t.Fatalf("DebugLiveCallbacks() = %d after unmount, want %d (the click listener was not released)", got, callbacksBefore)
	}

	label.Set("second")
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the post-unmount signal change")
	}
	if effectRuns != 1 {
		t.Errorf("effectRuns = %d after unmount + signal change, want 1 (the effect re-ran after dispose)", effectRuns)
	}
	if got := core.Unwrap(span).Get("textContent").String(); got != "first" {
		t.Errorf("span textContent = %q after unmount + signal change, want %q (the binding kept reacting after dispose)", got, "first")
	}
}

// TestOnDisposeRunsOnUnmount verifies core.OnDispose's contract: the
// registered cleanup runs exactly once, when the enclosing Scope disposes,
// never before and never again on a repeated Dispose call.
func TestOnDisposeRunsOnUnmount(t *testing.T) {
	runs := 0

	scope := signal.RunScoped(func() {
		core.OnDispose(func() { runs++ })
	})

	if runs != 0 {
		t.Fatalf("runs = %d before dispose, want 0", runs)
	}

	scope.Dispose()
	if runs != 1 {
		t.Fatalf("runs = %d after first Dispose, want 1", runs)
	}

	scope.Dispose() // idempotent: must not run again
	if runs != 1 {
		t.Fatalf("runs = %d after second Dispose, want 1 (OnDispose cleanup ran more than once)", runs)
	}
}
