//go:build js && wasm

package core_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.
//
// This file exists to answer one question empirically (see the root
// WORK.md, "core/signal: hook determinístico de depois que os efeitos
// rodaram"): does core.NextTick (a plain queueMicrotask deferral) happen to
// run after core/signal's effect scheduler has flushed the Effects
// triggered by a preceding Signal.Set, or before?
//
// core/signal's flushEffects runs on its own goroutine (see
// core/signal/signal.go's enqueue, "go flushEffects()"), spawned but not
// necessarily resumed synchronously with the caller. If Go's js/wasm runtime
// resumes a goroutine that can't continue inline via a macrotask
// (setTimeout) rather than a microtask, a microtask-based NextTick would run
// first, seeing stale state - meaning NextTick cannot be used as a
// "wait for this Set's effects to finish" primitive. These tests are the
// deciding evidence for whether a new core/signal primitive is needed.
import (
	"syscall/js"
	"testing"
	"time"

	"github.com/filipejohansson/vane/core"
)

func TestNextTickTimingRelativeToEffectFlush(t *testing.T) {
	sig := core.NewSignal(0)
	seen := 0
	core.Effect(func() func() {
		seen = sig.Get()
		return nil
	})
	if seen != 0 {
		t.Fatalf("initial effect run: seen = %d, want 0", seen)
	}

	sig.Set(1)
	// Diagnostic only (not the real assertion): is the effect flush actually
	// synchronous with Set() in practice, in this simple, non-reentrant case?
	t.Logf("seen immediately after Set(1), before scheduling anything = %d", seen)

	done := make(chan int, 1)
	core.NextTick(func() {
		done <- seen
	})

	select {
	case gotInNextTick := <-done:
		t.Logf("seen inside NextTick's callback = %d (1 = the effect flush had already happened, 0 = it had not)", gotInNextTick)
		if gotInNextTick != 1 {
			t.Errorf("NextTick's callback ran BEFORE the effect flush completed (seen = %d, want 1) - "+
				"confirms NextTick cannot be used to wait for a Signal.Set's effects to finish; "+
				"see the root WORK.md for what to build instead", gotInNextTick)
		}
	case <-time.After(time.Second):
		t.Fatal("NextTick never ran fn")
	}
}

// TestNextTickSeesDOMMutationFromEffect mirrors the router's actual
// scroll-to-anchor scenario: a Set() whose dependent Effect mutates the DOM
// (creates an element with an id, standing in for a newly-mounted route's
// scroll target) - not just a plain Go variable like the test above.
func TestNextTickSeesDOMMutationFromEffect(t *testing.T) {
	sig := core.NewSignal(0)
	body := js.Global().Get("document").Get("body")

	core.Effect(func() func() {
		if sig.Get() == 0 {
			return nil
		}
		el := core.El("div")
		core.SetProp(el, "id", "mutation-target")
		body.Call("appendChild", core.Unwrap(el))
		return nil
	})

	sig.Set(1)

	done := make(chan bool, 1)
	core.NextTick(func() {
		_, found := core.GetElementByID("mutation-target")
		done <- found
	})

	select {
	case found := <-done:
		t.Cleanup(func() {
			if el, ok := core.GetElementByID("mutation-target"); ok {
				body.Call("removeChild", core.Unwrap(el))
			}
		})
		if !found {
			t.Error("NextTick ran before the Effect's DOM mutation was applied - element not found")
		}
	case <-time.After(time.Second):
		t.Fatal("NextTick never ran fn")
	}
}

// TestNextTickSeesCascadingReentrantEffects tests the harder case: an Effect
// that, while it's running (still inside flushEffects' loop), creates a
// second Effect AND immediately Sets the signal that second Effect depends
// on - mirroring Router mounting a Layout, whose own inner Effect reacts to
// path changes during that same mount. flushEffects loops until its queue is
// completely empty, so this reentrant work should still be fully drained
// within the same flush before NextTick's callback can see it - if a single
// NextTick only happened to work for the single, non-reentrant case above,
// this is where a multi-round cascade would break it.
func TestNextTickSeesCascadingReentrantEffects(t *testing.T) {
	outer := core.NewSignal(0)
	inner := core.NewSignal("first")
	var innerSeen string

	core.Effect(func() func() {
		if outer.Get() == 0 {
			return nil
		}
		core.Effect(func() func() {
			innerSeen = inner.Get()
			return nil
		})
		// inner's dependent Effect above already ran once (Effect runs fn
		// immediately on creation); this Set enqueues a second, reentrant
		// round on the SAME flush that's currently processing this very
		// Effect.
		inner.Set("second")
		return nil
	})

	outer.Set(1)

	done := make(chan string, 1)
	core.NextTick(func() {
		done <- innerSeen
	})

	select {
	case got := <-done:
		if got != "second" {
			t.Errorf("NextTick ran before the reentrant cascade settled - innerSeen = %q, want %q", got, "second")
		}
	case <-time.After(time.Second):
		t.Fatal("NextTick never ran fn")
	}
}
