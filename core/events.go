//go:build js && wasm

package core

import (
	"syscall/js"

	"github.com/filipejohansson/vane/core/signal"
	"github.com/filipejohansson/vane/internal/dom"
)

// OnClick attaches a click handler to el.
func OnClick(el Node, fn func(e MouseEvent)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newMouseEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnClick, f)
	signal.RegisterDispose(f.Release)
}

// OnInput attaches an input handler to el, passing the current value string.
func OnInput(el Node, fn func(e InputEvent)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newInputEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnInput, f)
	signal.RegisterDispose(f.Release)
}

// OnChange attaches a change handler, passing the current value string.
// Use for <select> and text inputs when you want the committed value rather than every keystroke.
func OnChange(el Node, fn func(e InputEvent)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newInputEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnChange, f)
	signal.RegisterDispose(f.Release)
}

// OnChecked attaches a change handler for checkboxes/radios, passing the current checked state.
func OnChecked(el Node, fn func(e CheckEvent)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newCheckEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnChange, f)
	signal.RegisterDispose(f.Release)
}

// OnSubmit attaches a submit handler to a <form>, calling preventDefault automatically.
func OnSubmit(el Node, fn func(e SubmitEvent)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			args[0].Call(dom.PreventDefault)
			fn(newSubmitEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnSubmit, f)
	signal.RegisterDispose(f.Release)
}

// OnKeyDown attaches a keydown handler.
func OnKeyDown(el Node, fn func(e KeyEvent)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newKeyEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnKeyDown, f)
	signal.RegisterDispose(f.Release)
}

// OnKeyUp attaches a keyup handler.
func OnKeyUp(el Node, fn func(e KeyEvent)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newKeyEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnKeyUp, f)
	signal.RegisterDispose(f.Release)
}

// OnBlur attaches a blur handler.
func OnBlur(el Node, fn func(e Event)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnBlur, f)
	signal.RegisterDispose(f.Release)
}

// OnFocus attaches a focus handler.
func OnFocus(el Node, fn func(e Event)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnFocus, f)
	signal.RegisterDispose(f.Release)
}

// OnDblClick attaches a double-click handler.
func OnDblClick(el Node, fn func(e MouseEvent)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newMouseEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnDblClick, f)
	signal.RegisterDispose(f.Release)
}

// OnMouseEnter attaches a mouseenter handler.
func OnMouseEnter(el Node, fn func(e MouseEvent)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newMouseEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnMouseEnter, f)
	signal.RegisterDispose(f.Release)
}

// OnMouseLeave attaches a mouseleave handler.
func OnMouseLeave(el Node, fn func(e MouseEvent)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newMouseEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnMouseLeave, f)
	signal.RegisterDispose(f.Release)
}

// OnScroll attaches a scroll handler.
func OnScroll(el Node, fn func(e Event)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnScroll, f)
	signal.RegisterDispose(f.Release)
}

// OnPointerDown attaches a pointerdown handler.
func OnPointerDown(el Node, fn func(e MouseEvent)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newMouseEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnPointerDown, f)
	signal.RegisterDispose(f.Release)
}

// OnPointerUp attaches a pointerup handler.
func OnPointerUp(el Node, fn func(e MouseEvent)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newMouseEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnPointerUp, f)
	signal.RegisterDispose(f.Release)
}

// OnPointerMove attaches a pointermove handler.
func OnPointerMove(el Node, fn func(e MouseEvent)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newMouseEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnPointerMove, f)
	signal.RegisterDispose(f.Release)
}

// OnTouchStart attaches a touchstart handler.
func OnTouchStart(el Node, fn func(e TouchEvent)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newTouchEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnTouchStart, f)
	signal.RegisterDispose(f.Release)
}

// OnTouchEnd attaches a touchend handler.
func OnTouchEnd(el Node, fn func(e TouchEvent)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newTouchEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnTouchEnd, f)
	signal.RegisterDispose(f.Release)
}

// OnDragStart attaches a dragstart handler.
func OnDragStart(el Node, fn func(e Event)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnDragStart, f)
	signal.RegisterDispose(f.Release)
}

// OnDrop attaches a drop handler.
func OnDrop(el Node, fn func(e Event)) {
	if fn == nil {
		return
	}
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newEvent(args[0]))
		}
		return nil
	})
	Unwrap(el).Set(dom.OnDrop, f)
	signal.RegisterDispose(f.Release)
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
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newEvent(args[0]))
		}
		return nil
	})
	args := []any{event, f}
	if len(opts) > 0 && opts[0].Passive {
		args = append(args, map[string]any{"passive": true})
	}
	dom.Window.Call(dom.AddEventListener, args...)
	signal.RegisterDispose(func() {
		dom.Window.Call(dom.RemoveEventListener, event, f)
		f.Release()
	})
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
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newKeyEvent(args[0]))
		}
		return nil
	})
	dom.Window.Call(dom.AddEventListener, dom.EventKeyDown, f)
	signal.RegisterDispose(func() {
		dom.Window.Call(dom.RemoveEventListener, dom.EventKeyDown, f)
		f.Release()
	})
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
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(newMouseEvent(args[0]))
		}
		return nil
	})
	dom.Window.Call(dom.AddEventListener, dom.EventMouseMove, f)
	signal.RegisterDispose(func() {
		dom.Window.Call(dom.RemoveEventListener, dom.EventMouseMove, f)
		f.Release()
	})
}
