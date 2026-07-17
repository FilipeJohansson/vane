//go:build js && wasm

package core_test

import (
	"testing"
	"time"

	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/core/signal"
)

// Verifies the "Accessible modal, with Portal" pattern documented on
// vane-page's Accessibility page, using a callback ref + core.NextTick.
//
// The callback fires synchronously during element construction, before the
// caller (Portal, here) has inserted the node into the live document. This
// isn't a ref limitation: .focus() itself is a no-op on a disconnected node.
// core.NextTick defers just the FocusTrap call to right after the current
// render pass finishes attaching nodes. See its doc comment in util.go for
// why this is structural to how vane (and React) build trees, not a
// workaround for something fixable by restructuring construction order.
func TestDocumentedAccessibleModalPatternWorksWithCallbackRef(t *testing.T) {
	target := core.El("div")
	appendToBody(t, core.Unwrap(target))
	open := core.NewSignal(false)
	var trapEngaged bool

	core.Portal(target, func() core.Node {
		if !open.Get() {
			return nil
		}
		btn := core.El("button")
		core.AppendText(btn, "inside dialog")
		// Equivalent to: <button ref={func(el core.Node) {
		//     var untrap func()
		//     core.OnDispose(func() {
		//         if untrap != nil { untrap() }
		//     })
		//     core.NextTick(func() {
		//         untrap = core.FocusTrap(el)
		//     })
		// }}>
		//
		// OnDispose must be called synchronously, right here, to attach to
		// Portal's per-render scope (RunScoped pops that scope the moment
		// fn() returns, registering later, e.g. from inside NextTick's
		// callback, would attach to no scope at all and silently never
		// fire). Only the FocusTrap call itself needs deferring, since el
		// isn't attached to the document until after fn() returns.
		var untrap func()
		core.OnDispose(func() {
			if untrap != nil {
				untrap()
			}
		})
		core.NextTick(func() {
			untrap = core.FocusTrap(btn)
			trapEngaged = true
		})
		return btn
	})

	trigger := core.El("button")
	core.AppendChild(target, trigger)
	core.Unwrap(trigger).Call("focus")

	open.Set(true)
	// Portal's first-ever render is still deferred to a goroutine (the
	// gotcha documented in docs/patterns.md). The poll here is for
	// THAT, not for the ref itself (which the callback already gives us
	// synchronously).
	deadline := time.Now().Add(2 * time.Second)
	for !trapEngaged && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		signal.WaitEffects(200 * time.Millisecond)
	}
	if !trapEngaged {
		t.Fatal("FocusTrap was never engaged on first open")
	}

	dialogBtn := core.Unwrap(target).Call("querySelector", "button:last-child")
	if !headDoc(t).Get("activeElement").Equal(dialogBtn) {
		t.Errorf("focus is not on the dialog button after first open (activeElement: %s)",
			headDoc(t).Get("activeElement").Get("outerHTML").String())
	}

	open.Set(false)
	signal.WaitEffects(2 * time.Second)

	if !headDoc(t).Get("activeElement").Equal(core.Unwrap(trigger)) {
		t.Error("focus was not restored to the trigger button, so OnDispose(FocusTrap(...)) did not fire when Portal's content was replaced")
	}
}
