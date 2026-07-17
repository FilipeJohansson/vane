//go:build js && wasm

package core

// Internal (package core, not core_test) tests for the unexported
// new*Event decoders in event_types.go - these aren't reachable from
// outside the package, but are worth locking down now since the planned
// On* signature migration (internal_docs/typed-events-plan.md) will wire
// every handler through them.

import (
	"syscall/js"
	"testing"
)

// decodeOnDispatch attaches a real addEventListener on el so f runs while
// the native event's currentTarget is still live (the DOM only populates
// currentTarget for the duration of the listener call), then dispatches
// kind on dispatchOn and returns the decoded value f produced.
func decodeOnDispatch[T any](el, dispatchOn Node, kind string, raw js.Value, f func(js.Value) T) T {
	var got T
	handler := js.FuncOf(func(_ js.Value, args []js.Value) any {
		got = f(args[0])
		return nil
	})
	defer handler.Release()
	Unwrap(el).Call("addEventListener", kind, handler)
	Unwrap(dispatchOn).Call("dispatchEvent", raw)
	return got
}

func TestNewMouseEventDecodesFields(t *testing.T) {
	parent := El("div")
	child := El("span")
	AppendChild(parent, child)

	raw := js.Global().Get("MouseEvent").New("click", map[string]any{
		"bubbles": true, "clientX": 12, "clientY": 34, "button": 1,
		"metaKey": true, "ctrlKey": false, "shiftKey": true, "altKey": false,
	})
	e := decodeOnDispatch(parent, child, "click", raw, newMouseEvent)

	if e.ClientX != 12 || e.ClientY != 34 || e.Button != 1 {
		t.Errorf("newMouseEvent coords/button = %v,%v,%v, want 12,34,1", e.ClientX, e.ClientY, e.Button)
	}
	if !e.Meta || e.Ctrl || !e.Shift || e.Alt {
		t.Errorf("newMouseEvent modifiers = %+v, want Meta=true Ctrl=false Shift=true Alt=false", e)
	}
	if e.Self() {
		t.Error("newMouseEvent.Self() = true for a listener on parent dispatched from child, want false")
	}
	if !Unwrap(e.Target()).Equal(Unwrap(child)) {
		t.Error("newMouseEvent.Target() did not unwrap to the dispatching child")
	}
	if !Unwrap(e.CurrentTarget()).Equal(Unwrap(parent)) {
		t.Error("newMouseEvent.CurrentTarget() did not unwrap to the listening parent")
	}
}

func TestNewEventTargetAndSelf(t *testing.T) {
	el := El("div")
	raw := js.Global().Get("Event").New("blur", map[string]any{"bubbles": true})
	e := decodeOnDispatch(el, el, "blur", raw, newEvent)

	if !e.Self() {
		t.Error("newEvent.Self() = false for a listener and dispatch target being the same element")
	}
	if !Unwrap(e.Target()).Equal(Unwrap(el)) {
		t.Error("newEvent.Target() did not unwrap to el")
	}
}

func TestNewInputEventDecodesValue(t *testing.T) {
	input := El("input")
	Unwrap(input).Set("value", "hello")
	raw := js.Global().Get("Event").New("input", map[string]any{"bubbles": true})
	Unwrap(input).Call("dispatchEvent", raw)

	if got := newInputEvent(raw).Value; got != "hello" {
		t.Errorf("newInputEvent.Value = %q, want \"hello\"", got)
	}
}

func TestNewCheckEventDecodesChecked(t *testing.T) {
	cb := El("input")
	Unwrap(cb).Set("checked", true)
	raw := js.Global().Get("Event").New("change", map[string]any{"bubbles": true})
	Unwrap(cb).Call("dispatchEvent", raw)

	if got := newCheckEvent(raw).Checked; !got {
		t.Error("newCheckEvent.Checked = false, want true")
	}
}

func TestNewTouchEventDecodesTouches(t *testing.T) {
	touch := js.Global().Get("Object").New()
	touch.Set("clientX", 5)
	touch.Set("clientY", 6)
	touches := js.Global().Get("Array").New(touch)
	raw := js.ValueOf(map[string]any{"touches": touches, "target": js.Null(), "currentTarget": js.Null()})

	e := newTouchEvent(raw)
	if len(e.Touches) != 1 || e.Touches[0].ClientX != 5 || e.Touches[0].ClientY != 6 {
		t.Errorf("newTouchEvent.Touches = %+v, want one point {5,6}", e.Touches)
	}
}
