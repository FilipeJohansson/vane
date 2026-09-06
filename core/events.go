//go:build js && wasm

package core

import (
	"sync/atomic"
	"syscall/js"

	"github.com/filipejohansson/vane/core/signal"
	"github.com/filipejohansson/vane/internal/dom"
)

// liveCallbacks counts js.Func callbacks currently bound to the DOM via
// bindHandler/bindWindowListener, incremented when one is created and
// decremented at the single point where it's released.
var liveCallbacks atomic.Int64

// bindHandler assigns f to el[prop] and releases it automatically when the
// enclosing Scope disposes (see signal.RegisterDispose), the shape shared by
// every element-level On* handler below.
func bindHandler(el Node, prop string, f js.Func) {
	Unwrap(el).Set(prop, f)
	liveCallbacks.Add(1)
	signal.RegisterDispose(func() {
		f.Release()
		liveCallbacks.Add(-1)
	})
}

// bindWindowListener adds f as a window-level event listener and removes it
// (in addition to releasing it) when the enclosing Scope disposes.
func bindWindowListener(event string, f js.Func, opts ...ListenerOpts) {
	args := []any{event, f}
	if len(opts) > 0 && opts[0].Passive {
		args = append(args, map[string]any{"passive": true})
	}
	dom.Window.Call(dom.AddEventListener, args...)
	liveCallbacks.Add(1)
	signal.RegisterDispose(func() {
		dom.Window.Call(dom.RemoveEventListener, event, f)
		f.Release()
		liveCallbacks.Add(-1)
	})
}

// OnClick attaches a click handler to el.
func OnClick(el Node, fn func(e MouseEvent)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnClick, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newMouseEvent(args[0]))
		}
		return nil
	}))
}

// OnInput attaches an input handler to el, passing the current value string.
func OnInput(el Node, fn func(e InputEvent)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnInput, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newInputEvent(args[0]))
		}
		return nil
	}))
}

// OnChange attaches a change handler, passing the current value string.
// Use for <select> and text inputs when you want the committed value rather than every keystroke.
func OnChange(el Node, fn func(e InputEvent)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnChange, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newInputEvent(args[0]))
		}
		return nil
	}))
}

// OnChecked attaches a change handler for checkboxes/radios, passing the current checked state.
func OnChecked(el Node, fn func(e CheckEvent)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnChange, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newCheckEvent(args[0]))
		}
		return nil
	}))
}

// OnSubmit attaches a submit handler to a <form>, calling preventDefault automatically.
func OnSubmit(el Node, fn func(e SubmitEvent)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnSubmit, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			args[0].Call(dom.PreventDefault)
			fn(newSubmitEvent(args[0]))
		}
		return nil
	}))
}

// OnKeyDown attaches a keydown handler.
func OnKeyDown(el Node, fn func(e KeyEvent)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnKeyDown, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newKeyEvent(args[0]))
		}
		return nil
	}))
}

// OnKeyUp attaches a keyup handler.
func OnKeyUp(el Node, fn func(e KeyEvent)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnKeyUp, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newKeyEvent(args[0]))
		}
		return nil
	}))
}

// OnBlur attaches a blur handler.
func OnBlur(el Node, fn func(e Event)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnBlur, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newEvent(args[0]))
		}
		return nil
	}))
}

// OnFocus attaches a focus handler.
func OnFocus(el Node, fn func(e Event)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnFocus, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newEvent(args[0]))
		}
		return nil
	}))
}

// OnDblClick attaches a double-click handler.
func OnDblClick(el Node, fn func(e MouseEvent)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnDblClick, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newMouseEvent(args[0]))
		}
		return nil
	}))
}

// OnMouseEnter attaches a mouseenter handler.
func OnMouseEnter(el Node, fn func(e MouseEvent)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnMouseEnter, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newMouseEvent(args[0]))
		}
		return nil
	}))
}

// OnMouseLeave attaches a mouseleave handler.
func OnMouseLeave(el Node, fn func(e MouseEvent)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnMouseLeave, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newMouseEvent(args[0]))
		}
		return nil
	}))
}

// OnScroll attaches a scroll handler.
func OnScroll(el Node, fn func(e Event)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnScroll, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newEvent(args[0]))
		}
		return nil
	}))
}

// OnPointerDown attaches a pointerdown handler.
func OnPointerDown(el Node, fn func(e MouseEvent)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnPointerDown, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newMouseEvent(args[0]))
		}
		return nil
	}))
}

// OnPointerUp attaches a pointerup handler.
func OnPointerUp(el Node, fn func(e MouseEvent)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnPointerUp, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newMouseEvent(args[0]))
		}
		return nil
	}))
}

// OnPointerMove attaches a pointermove handler.
func OnPointerMove(el Node, fn func(e MouseEvent)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnPointerMove, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newMouseEvent(args[0]))
		}
		return nil
	}))
}

// OnTouchStart attaches a touchstart handler.
func OnTouchStart(el Node, fn func(e TouchEvent)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnTouchStart, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newTouchEvent(args[0]))
		}
		return nil
	}))
}

// OnTouchEnd attaches a touchend handler.
func OnTouchEnd(el Node, fn func(e TouchEvent)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnTouchEnd, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newTouchEvent(args[0]))
		}
		return nil
	}))
}

// OnDragStart attaches a dragstart handler.
func OnDragStart(el Node, fn func(e Event)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnDragStart, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newEvent(args[0]))
		}
		return nil
	}))
}

// OnDrop attaches a drop handler.
func OnDrop(el Node, fn func(e Event)) {
	if fn == nil {
		return
	}
	bindHandler(el, dom.OnDrop, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newEvent(args[0]))
		}
		return nil
	}))
}

// ListenerOpts configures OnWindowEvent. The zero value (all false) matches
// addEventListener's own defaults.
type ListenerOpts struct {
	// Passive marks the listener as never calling preventDefault, letting the
	// browser optimize scroll/touch handling instead of waiting on it.
	Passive bool
}

// OnWindowEvent attaches fn to a window-level event (e.g. "scroll",
// "resize"), removed automatically via OnDispose when the enclosing scope
// tears down. Element-level events go through the On* props (OnClick,
// OnScroll, etc.); this is for events that only ever fire on window, never
// bubbling through any element in the tree.
func OnWindowEvent(event string, fn func(e Event), opts ...ListenerOpts) {
	if fn == nil {
		return
	}
	bindWindowListener(event, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newEvent(args[0]))
		}
		return nil
	}), opts...)
}

// OnWindowKeyDown attaches fn to window-level keydown, receiving a decoded
// KeyEvent on every keystroke anywhere in the document, removed
// automatically via OnDispose when the enclosing scope tears down. Use this
// for global shortcuts (e.g. Ctrl/Cmd+K): check e.Key/e.Ctrl/e.Meta/etc.
// inside fn, there's no per-combination variant.
func OnWindowKeyDown(fn func(e KeyEvent)) {
	if fn == nil {
		return
	}
	bindWindowListener(dom.EventKeyDown, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newKeyEvent(args[0]))
		}
		return nil
	}))
}

// OnWindowMouseMove attaches fn to window-level mousemove, receiving a
// decoded MouseEvent on every move, removed automatically via OnDispose when
// the enclosing scope tears down. There's no element-scoped OnMouseMove
// prop: the only need for it so far (Weathervane's pointer-tilt effect)
// tracks the pointer across the whole viewport, not one element.
func OnWindowMouseMove(fn func(e MouseEvent)) {
	if fn == nil {
		return
	}
	bindWindowListener(dom.EventMouseMove, js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newMouseEvent(args[0]))
		}
		return nil
	}))
}
