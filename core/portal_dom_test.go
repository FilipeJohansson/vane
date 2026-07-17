//go:build js && wasm

package core_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.
//
// core.Portal resolves its target and mounts content from inside a goroutine
// (deliberately, so the caller's own DOM is attached first, see the comment
// in portal.go). That means Portal's effect on the DOM is not visible
// immediately after the call returns; tests must poll for it.

import (
	"strings"
	"syscall/js"
	"testing"
	"time"

	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/core/signal"
)

// waitForChildText polls until target's textContent equals want, or fails
// the test after a timeout. Needed because Portal mounts asynchronously.
func waitForChildText(t *testing.T, target js.Value, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if target.Get("textContent").String() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("target.textContent did not become %q within timeout (stuck at %q)", want, target.Get("textContent").String())
}

func newAttachedDiv(t *testing.T, id string) js.Value {
	t.Helper()
	el := js.Global().Get("document").Call("createElement", "div")
	el.Set("id", id)
	body := js.Global().Get("document").Get("body")
	body.Call("appendChild", el)
	t.Cleanup(func() { body.Call("removeChild", el) })
	return el
}

func TestPortalRendersIntoSelectorTarget(t *testing.T) {
	target := newAttachedDiv(t, "portal-root-selector")

	core.Portal("#portal-root-selector", func() core.Node {
		return core.Text("portal content")
	})

	waitForChildText(t, target, "portal content")
}

func TestPortalRendersIntoNodeTarget(t *testing.T) {
	target := core.El("div")

	core.Portal(target, func() core.Node {
		return core.Text("portal content")
	})

	waitForChildText(t, core.Unwrap(target), "portal content")
}

func TestPortalRendersIntoRawJSValueTarget(t *testing.T) {
	target := js.Global().Get("document").Call("createElement", "div")

	core.Portal(target, func() core.Node {
		return core.Text("portal content")
	})

	waitForChildText(t, target, "portal content")
}

func TestPortalTargetNotFoundDoesNotPanic(t *testing.T) {
	// Resolution happens in a goroutine, so a panic there would crash the
	// whole test binary, not just fail this test, so this mainly guards
	// against a regression that removes the not-found guard in
	// resolvePortalTarget. Just needs to survive past the wait.
	result := core.Portal("#does-not-exist-anywhere", func() core.Node {
		return core.Text("should never mount")
	})
	if core.Unwrap(result).Truthy() {
		t.Error("Portal should return an empty placeholder in the caller's position")
	}
	time.Sleep(50 * time.Millisecond) // let the goroutine run and give up quietly
}

// TestPortalSelfHealsWhenTargetAppearsAfterAReactiveRerun guards against a
// real gap: the target used to be resolved once, outside the Effect, so a
// selector missing at mount gave up for good, no matter what later
// triggered fn() to re-run (e.g. a modal's own open button/signal) - one
// console warning at mount, then permanent silence, including the exact
// moment a user action needed the portal to actually render. Now target
// resolution happens inside the Effect on every run, so once the element
// shows up in the DOM, the next signal-driven re-run finds it.
func TestPortalSelfHealsWhenTargetAppearsAfterAReactiveRerun(t *testing.T) {
	label := core.NewSignal("first")

	core.Portal("#portal-root-self-heal", func() core.Node {
		return core.Text(label.Get())
	})
	time.Sleep(50 * time.Millisecond) // let the goroutine attempt (and fail) the first resolution

	target := newAttachedDiv(t, "portal-root-self-heal")

	// The Effect only re-runs on a signal change it's subscribed to (it
	// subscribed to label via fn(), even though the render had nowhere to
	// go), so trigger one now that the target exists.
	label.Set("second")
	waitForChildText(t, target, "second")
}

// TestPortalWarnsOnlyWhenThereIsRealContentWithNoTarget guards the other
// half of the same fix: dest used to be resolved (and thus warned about)
// unconditionally, even while fn() returned nil (e.g. a closed modal at
// mount). That burned Warn's one-per-message dedup slot on a false alarm
// before the caller ever tried to show anything, so the real failure -
// clicking a modal's own open button with a missing target - then logged
// nothing at all, since the message had already fired once for no reason.
// dest resolution (and its warning) must be skipped entirely while there's
// no content, and only happen once there's something real to place.
func TestPortalWarnsOnlyWhenThereIsRealContentWithNoTarget(t *testing.T) {
	console := jsGlobalConsole(t)
	orig := console.Get("warn")
	var calls []string
	console.Set("warn", jsFuncCapture(t, &calls))
	t.Cleanup(func() { console.Set("warn", orig) })

	open := core.NewSignal(false)
	core.Portal("#portal-root-missing-until-open", func() core.Node {
		if !open.Get() {
			return nil
		}
		return core.Text("dialog content")
	})
	time.Sleep(50 * time.Millisecond) // let the goroutine run its first (closed, nil) render

	if len(calls) != 0 {
		t.Fatalf("Portal warned before there was any content to place: %v", calls)
	}

	// The equivalent of clicking a modal's own open button.
	open.Set(true)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(calls) == 0 {
		time.Sleep(5 * time.Millisecond)
	}

	if len(calls) != 1 {
		t.Fatalf("expected exactly one warn once there was real content and nowhere to put it, got %v", calls)
	}
	if !strings.Contains(calls[0], "target not found") {
		t.Errorf("warn message = %q, want it to mention target not found", calls[0])
	}
}

func TestPortalMultiplePortalsToSameTargetConcatenate(t *testing.T) {
	target := newAttachedDiv(t, "portal-root-multi")

	core.Portal("#portal-root-multi", func() core.Node { return core.Text("first ") })
	core.Portal("#portal-root-multi", func() core.Node { return core.Text("second") })

	waitForChildText(t, target, "first second")
}

func TestPortalReactsToSignalChangeViaOwnEffect(t *testing.T) {
	target := newAttachedDiv(t, "portal-root-reactive")
	label := core.NewSignal("one")

	core.Portal("#portal-root-reactive", func() core.Node {
		return core.Text(label.Get())
	})
	waitForChildText(t, target, "one")

	label.Set("two")
	waitForChildText(t, target, "two")
}

func TestPortalCleanupRemovesContentOnScopeDispose(t *testing.T) {
	target := newAttachedDiv(t, "portal-root-cleanup")

	scope := signal.RunScoped(func() {
		core.Portal("#portal-root-cleanup", func() core.Node {
			return core.Text("mounted")
		})
	})
	waitForChildText(t, target, "mounted")

	scope.Dispose()

	if got := target.Get("textContent").String(); got != "" {
		t.Errorf("textContent after scope disposed = %q, want empty (Portal content must be removed on unmount)", got)
	}
}
