//go:build js && wasm

package core_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.

import (
	"syscall/js"
	"testing"
	"time"

	"github.com/filipejohansson/vane/core"
)

func appendToBody(t *testing.T, raw js.Value) {
	t.Helper()
	body := js.Global().Get("document").Get("body")
	body.Call("appendChild", raw)
	t.Cleanup(func() { body.Call("removeChild", raw) })
}

func TestFocusMovesFocusToElement(t *testing.T) {
	input := core.El("input")
	raw := core.Unwrap(input)
	appendToBody(t, raw)

	core.Focus(input)

	if !raw.Equal(js.Global().Get("document").Get("activeElement")) {
		t.Error("document.activeElement is not the focused input")
	}
}

func TestFocusOnEmptyNodeDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Focus(core.Empty()) panicked: %v", r)
		}
	}()
	core.Focus(core.Empty())
}

func TestNextTickRunsAfterCurrentCallStackReturns(t *testing.T) {
	var ran bool
	done := make(chan struct{})
	core.NextTick(func() {
		ran = true
		close(done)
	})
	if ran {
		t.Fatal("NextTick ran fn synchronously, but it must defer past the current call stack")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("NextTick never ran fn")
	}
}

func TestNextTickSeesElementAttachedByCaller(t *testing.T) {
	parent := js.Global().Get("document").Call("createElement", "div")
	appendToBody(t, parent)
	child := js.Global().Get("document").Call("createElement", "button")

	done := make(chan bool, 1)
	core.NextTick(func() {
		done <- child.Get("isConnected").Bool()
	})
	// Mirrors vane's build-then-attach order: the node exists (and any ref
	// callback on it has already run) before the caller attaches it.
	parent.Call("appendChild", child)

	select {
	case connected := <-done:
		if !connected {
			t.Error("child was not connected to the document by the time NextTick's fn ran")
		}
	case <-time.After(time.Second):
		t.Fatal("NextTick never ran fn")
	}
}

func buildFocusTrapContainer(t *testing.T) (el core.Node, container js.Value, first, second, third js.Value) {
	t.Helper()
	el = core.El("div")
	container = core.Unwrap(el)
	appendToBody(t, container)

	mk := func(label string) js.Value {
		btn := js.Global().Get("document").Call("createElement", "button")
		btn.Set("textContent", label)
		container.Call("appendChild", btn)
		return btn
	}
	first = mk("first")
	second = mk("second")
	third = mk("third")
	return
}

func dispatchKey(t *testing.T, target js.Value, key string, shift bool) {
	t.Helper()
	event := js.Global().Get("KeyboardEvent").New("keydown", map[string]any{
		"key": key, "shiftKey": shift, "bubbles": true, "cancelable": true,
	})
	target.Call("dispatchEvent", event)
}

func TestFocusTrapMovesInitialFocusToFirstFocusable(t *testing.T) {
	el, _, first, _, _ := buildFocusTrapContainer(t)
	cleanup := core.FocusTrap(el)
	defer cleanup()

	if !first.Equal(js.Global().Get("document").Get("activeElement")) {
		t.Error("initial focus was not moved to the first focusable descendant")
	}
}

func TestFocusTrapWrapsFromLastToFirstOnTab(t *testing.T) {
	el, container, first, _, third := buildFocusTrapContainer(t)
	cleanup := core.FocusTrap(el)
	defer cleanup()

	third.Call("focus")
	dispatchKey(t, container, "Tab", false)

	if !first.Equal(js.Global().Get("document").Get("activeElement")) {
		t.Error("Tab on the last element did not wrap focus to the first")
	}
}

func TestFocusTrapWrapsFromFirstToLastOnShiftTab(t *testing.T) {
	el, container, first, _, third := buildFocusTrapContainer(t)
	cleanup := core.FocusTrap(el)
	defer cleanup()

	first.Call("focus")
	dispatchKey(t, container, "Tab", true)

	if !third.Equal(js.Global().Get("document").Get("activeElement")) {
		t.Error("Shift+Tab on the first element did not wrap focus to the last")
	}
}

func TestFocusTrapDoesNotInterceptNonTabKeys(t *testing.T) {
	el, container, _, second, _ := buildFocusTrapContainer(t)
	cleanup := core.FocusTrap(el)
	defer cleanup()

	second.Call("focus")
	dispatchKey(t, container, "Enter", false)

	if !second.Equal(js.Global().Get("document").Get("activeElement")) {
		t.Error("a non-Tab key moved focus, but FocusTrap should only intercept Tab")
	}
}

func TestFocusTrapCleanupRestoresPreviousFocus(t *testing.T) {
	trigger := js.Global().Get("document").Call("createElement", "button")
	trigger.Set("textContent", "open")
	js.Global().Get("document").Get("body").Call("appendChild", trigger)
	t.Cleanup(func() { js.Global().Get("document").Get("body").Call("removeChild", trigger) })
	trigger.Call("focus")

	el, _, _, _, _ := buildFocusTrapContainer(t)
	cleanup := core.FocusTrap(el)

	if trigger.Equal(js.Global().Get("document").Get("activeElement")) {
		t.Fatal("FocusTrap did not move focus away from the trigger")
	}

	cleanup()

	if !trigger.Equal(js.Global().Get("document").Get("activeElement")) {
		t.Error("cleanup did not restore focus to the element focused before the trap engaged")
	}
}

func TestFocusTrapCleanupRemovesKeydownListener(t *testing.T) {
	el, container, first, _, third := buildFocusTrapContainer(t)
	cleanup := core.FocusTrap(el)
	cleanup()

	third.Call("focus")
	dispatchKey(t, container, "Tab", false)

	if first.Equal(js.Global().Get("document").Get("activeElement")) {
		t.Error("Tab still wraps focus after cleanup, listener was not removed")
	}
}

func TestAnnounceCreatesVisuallyHiddenLiveRegion(t *testing.T) {
	core.Announce("hello")

	region := js.Global().Get("document").Call("querySelector", "[aria-live]")
	if !region.Truthy() {
		t.Fatal("no [aria-live] region found in the document")
	}
	if got := region.Call("getAttribute", "aria-live").String(); got != "polite" {
		t.Errorf("aria-live = %q, want %q", got, "polite")
	}
	// Visually hidden but not display:none/visibility:hidden. Those would
	// remove it from the accessibility tree, and screen readers would
	// never see it.
	clip := region.Get("style").Get("clip").String()
	if clip == "" {
		t.Error("live region is not visually hidden via the clip technique")
	}
}

func TestAnnounceSetsRegionTextAfterDelay(t *testing.T) {
	core.Announce("first message")
	time.Sleep(150 * time.Millisecond)

	region := js.Global().Get("document").Call("querySelector", "[aria-live]")
	if got := region.Get("textContent").String(); got != "first message" {
		t.Errorf("textContent = %q, want %q", got, "first message")
	}
}

func TestAnnounceReusesSameRegionAcrossCalls(t *testing.T) {
	core.Announce("call one")
	time.Sleep(150 * time.Millisecond)

	regions := js.Global().Get("document").Call("querySelectorAll", "[aria-live]")
	countBefore := regions.Get("length").Int()

	core.Announce("call two")
	time.Sleep(150 * time.Millisecond)

	regionsAfter := js.Global().Get("document").Call("querySelectorAll", "[aria-live]")
	if got := regionsAfter.Get("length").Int(); got != countBefore {
		t.Errorf("[aria-live] region count changed from %d to %d, but Announce must reuse one shared region, not create a new one per call", countBefore, got)
	}

	region := js.Global().Get("document").Call("querySelector", "[aria-live]")
	if got := region.Get("textContent").String(); got != "call two" {
		t.Errorf("textContent = %q, want %q", got, "call two")
	}
}

// TestAnnounceClearsBeforeReannouncingSameMessage guards the accessibility.md
// claim that calling Announce twice with the same message still triggers a
// re-announcement, since aria-live only fires on an actual content change,
// so a same-text call must clear the region before writing the text back.
func TestAnnounceClearsBeforeReannouncingSameMessage(t *testing.T) {
	core.Announce("repeat me")
	time.Sleep(150 * time.Millisecond)

	region := js.Global().Get("document").Call("querySelector", "[aria-live]")
	if got := region.Get("textContent").String(); got != "repeat me" {
		t.Fatalf("textContent = %q, want %q", got, "repeat me")
	}

	core.Announce("repeat me")
	time.Sleep(10 * time.Millisecond) // before the 50ms set-delay fires

	if got := region.Get("textContent").String(); got != "" {
		t.Errorf("textContent right after re-Announce = %q, want empty (must clear before re-setting, even for the same message)", got)
	}

	time.Sleep(150 * time.Millisecond)
	if got := region.Get("textContent").String(); got != "repeat me" {
		t.Errorf("textContent after delay = %q, want %q", got, "repeat me")
	}
}
