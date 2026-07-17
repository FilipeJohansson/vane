//go:build js && wasm

package core_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.

import (
	"syscall/js"
	"testing"

	"github.com/filipejohansson/vane/core"
)

// dispatch fires a generic DOM event of kind on n. Safe for handlers that
// only ever read target/currentTarget (Event-category) or that read state
// off target itself (OnInput/OnChange/OnChecked, target.value/checked;
// OnSubmit, target) rather than event-specific fields.
func dispatch(t *testing.T, n core.Node, kind string) {
	t.Helper()
	event := js.Global().Get("Event").New(kind, map[string]any{"bubbles": true})
	core.Unwrap(n).Call("dispatchEvent", event)
}

// dispatchMouse fires a real MouseEvent of kind on n. MouseEvent-category
// handlers (OnClick, OnDblClick, ...) always decode clientX/clientY/button/
// modifiers, which only exist (even as zero/false defaults) on an actual
// MouseEvent instance, not a plain Event.
func dispatchMouse(t *testing.T, n core.Node, kind string) {
	t.Helper()
	event := js.Global().Get("MouseEvent").New(kind, map[string]any{"bubbles": true})
	core.Unwrap(n).Call("dispatchEvent", event)
}

func TestOnClickInvokesHandlerOnDispatch(t *testing.T) {
	btn := core.El("button")
	called := false
	core.OnClick(btn, func(e core.MouseEvent) { called = true })

	dispatchMouse(t, btn, "click")

	if !called {
		t.Error("OnClick handler was not invoked on a dispatched click event")
	}
}

func TestOnClickDecodesMouseEvent(t *testing.T) {
	btn := core.El("button")
	var got core.MouseEvent
	core.OnClick(btn, func(e core.MouseEvent) { got = e })

	event := js.Global().Get("MouseEvent").New("click", map[string]any{
		"bubbles": true, "clientX": 10, "clientY": 20, "button": 2,
		"metaKey": true, "ctrlKey": false, "shiftKey": true, "altKey": false,
	})
	core.Unwrap(btn).Call("dispatchEvent", event)

	if got.ClientX != 10 || got.ClientY != 20 || got.Button != 2 {
		t.Errorf("OnClick decoded coords/button = %v,%v,%v, want 10,20,2", got.ClientX, got.ClientY, got.Button)
	}
	if !got.Meta || got.Ctrl || !got.Shift || got.Alt {
		t.Errorf("OnClick decoded modifiers = %+v, want Meta=true Ctrl=false Shift=true Alt=false", got)
	}
}

func TestOnClickSelfAndTarget(t *testing.T) {
	parent := core.El("div")
	child := core.El("span")
	core.AppendChild(parent, child)

	var got core.MouseEvent
	core.OnClick(parent, func(e core.MouseEvent) { got = e })

	dispatchMouse(t, child, "click")

	if got.Self() {
		t.Error("MouseEvent.Self() = true for a listener on parent dispatched from child, want false")
	}
	if !core.Unwrap(got.Target()).Equal(core.Unwrap(child)) {
		t.Error("MouseEvent.Target() did not unwrap to the dispatching child")
	}
	if !core.Unwrap(got.CurrentTarget()).Equal(core.Unwrap(parent)) {
		t.Error("MouseEvent.CurrentTarget() did not unwrap to the listening parent")
	}
}

func TestOnClickNilFnDoesNotPanicOrAttachHandler(t *testing.T) {
	btn := core.El("button")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("OnClick(nil) panicked: %v", r)
		}
	}()
	core.OnClick(btn, nil)

	raw := core.Unwrap(btn)
	if onclick := raw.Get("onclick"); onclick.Truthy() {
		t.Error("OnClick(el, nil) should leave onclick unset")
	}
	// Dispatching must not panic even though no handler was attached.
	dispatchMouse(t, btn, "click")
}

func TestOnInputPassesValue(t *testing.T) {
	input := core.El("input")
	var got string
	core.OnInput(input, func(e core.InputEvent) { got = e.Value })

	core.Unwrap(input).Set("value", "hello")
	dispatch(t, input, "input")

	if got != "hello" {
		t.Errorf("OnInput received %q, want %q", got, "hello")
	}
}

func TestOnChangePassesValue(t *testing.T) {
	sel := core.El("select")
	opt := core.El("option")
	core.Unwrap(opt).Set("value", "opt2")
	core.AppendChild(sel, opt)

	var got string
	core.OnChange(sel, func(e core.InputEvent) { got = e.Value })

	core.Unwrap(sel).Set("value", "opt2")
	dispatch(t, sel, "change")

	if got != "opt2" {
		t.Errorf("OnChange received %q, want %q", got, "opt2")
	}
}

func TestOnCheckedPassesChecked(t *testing.T) {
	checkbox := core.El("input")
	core.Unwrap(checkbox).Set("type", "checkbox")
	var got bool
	core.OnChecked(checkbox, func(e core.CheckEvent) { got = e.Checked })

	core.Unwrap(checkbox).Set("checked", true)
	dispatch(t, checkbox, "change")

	if !got {
		t.Error("OnChecked received false, want true")
	}
}

func TestOnKeyDownPassesKeyAndModifiers(t *testing.T) {
	input := core.El("input")
	var got core.KeyEvent
	core.OnKeyDown(input, func(e core.KeyEvent) { got = e })

	event := js.Global().Get("KeyboardEvent").New("keydown", map[string]any{
		"key": "Enter", "bubbles": true, "ctrlKey": true,
	})
	core.Unwrap(input).Call("dispatchEvent", event)

	if got.Key != "Enter" {
		t.Errorf("OnKeyDown received key %q, want %q", got.Key, "Enter")
	}
	if !got.Ctrl {
		t.Error("OnKeyDown did not decode Ctrl modifier")
	}
}

func TestOnKeyUpPassesKey(t *testing.T) {
	input := core.El("input")
	var got string
	core.OnKeyUp(input, func(e core.KeyEvent) { got = e.Key })

	event := js.Global().Get("KeyboardEvent").New("keyup", map[string]any{"key": "Escape", "bubbles": true})
	core.Unwrap(input).Call("dispatchEvent", event)

	if got != "Escape" {
		t.Errorf("OnKeyUp received %q, want %q", got, "Escape")
	}
}

// TestMouseEventHandlersInvokeOnDispatch covers every On* handler whose Go
// signature is func(e MouseEvent) beyond OnClick (already covered above).
// They all share the same decode-and-call shape in events.go, so one table
// drives all of them instead of copy-pasting one test per handler.
//
// OnPointerDown/Up/Move are excluded here, since jsdom doesn't wire the
// onpointer* IDL attributes to event dispatch (incomplete Pointer Events
// support), so dispatchEvent never invokes them even though our code sets
// the property correctly. See TestOnPointerHandlersSetPropertyDirectly below.
func TestMouseEventHandlersInvokeOnDispatch(t *testing.T) {
	cases := []struct {
		name      string
		eventKind string
		attach    func(el core.Node, fn func(e core.MouseEvent))
	}{
		{"OnDblClick", "dblclick", core.OnDblClick},
		{"OnMouseEnter", "mouseenter", core.OnMouseEnter},
		{"OnMouseLeave", "mouseleave", core.OnMouseLeave},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			el := core.El("div")
			called := false
			c.attach(el, func(e core.MouseEvent) { called = true })

			dispatchMouse(t, el, c.eventKind)

			if !called {
				t.Errorf("%s handler was not invoked on a dispatched %q event", c.name, c.eventKind)
			}
		})
	}
}

// TestEventHandlersInvokeOnDispatch covers every On* handler whose Go
// signature is func(e Event): generic target/currentTarget/self decode, no
// category-specific fields, so a plain Event dispatch is enough.
func TestEventHandlersInvokeOnDispatch(t *testing.T) {
	cases := []struct {
		name      string
		eventKind string
		attach    func(el core.Node, fn func(e core.Event))
	}{
		{"OnBlur", "blur", core.OnBlur},
		{"OnFocus", "focus", core.OnFocus},
		{"OnScroll", "scroll", core.OnScroll},
		{"OnDragStart", "dragstart", core.OnDragStart},
		{"OnDrop", "drop", core.OnDrop},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			el := core.El("div")
			called := false
			c.attach(el, func(e core.Event) { called = true })

			dispatch(t, el, c.eventKind)

			if !called {
				t.Errorf("%s handler was not invoked on a dispatched %q event", c.name, c.eventKind)
			}
		})
	}
}

// TestTouchEventHandlersInvokeOnDispatch covers OnTouchStart/OnTouchEnd
// (func(e TouchEvent)). jsdom's TouchEvent constructor support isn't
// guaranteed (see internal_docs/testing.md), so this dispatches a plain
// Event with a manually attached empty touches list, exercising newTouchEvent's
// decode loop (0 iterations) without depending on TouchEvent/Touch constructors.
func TestTouchEventHandlersInvokeOnDispatch(t *testing.T) {
	cases := []struct {
		name      string
		eventKind string
		attach    func(el core.Node, fn func(e core.TouchEvent))
	}{
		{"OnTouchStart", "touchstart", core.OnTouchStart},
		{"OnTouchEnd", "touchend", core.OnTouchEnd},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			el := core.El("div")
			called := false
			c.attach(el, func(e core.TouchEvent) { called = true })

			event := js.Global().Get("Event").New(c.eventKind, map[string]any{"bubbles": true})
			event.Set("touches", js.Global().Get("Array").New())
			core.Unwrap(el).Call("dispatchEvent", event)

			if !called {
				t.Errorf("%s handler was not invoked on a dispatched %q event", c.name, c.eventKind)
			}
		})
	}
}

func TestTouchEventDecodesTouches(t *testing.T) {
	el := core.El("div")
	var got core.TouchEvent
	core.OnTouchStart(el, func(e core.TouchEvent) { got = e })

	touch := js.Global().Get("Object").New()
	touch.Set("clientX", 7)
	touch.Set("clientY", 8)
	event := js.Global().Get("Event").New("touchstart", map[string]any{"bubbles": true})
	event.Set("touches", js.Global().Get("Array").New(touch))
	core.Unwrap(el).Call("dispatchEvent", event)

	if len(got.Touches) != 1 || got.Touches[0].ClientX != 7 || got.Touches[0].ClientY != 8 {
		t.Errorf("OnTouchStart decoded Touches = %+v, want one point {7,8}", got.Touches)
	}
}

// TestOnPointerHandlersSetPropertyDirectly covers OnPointerDown/Up/Move by
// invoking the attached JS function directly instead of via dispatchEvent,
// since jsdom doesn't dispatch through onpointer* properties (see comment above),
// so this is the only way to verify these three under this harness. A real
// browser (gap #4 in next-steps.md) would be needed to test the dispatch path.
// The handler is invoked with a real MouseEvent so newMouseEvent's decode
// (clientX/clientY/button/modifiers) has real fields to read.
func TestOnPointerHandlersSetPropertyDirectly(t *testing.T) {
	cases := []struct {
		name     string
		propName string
		attach   func(el core.Node, fn func(e core.MouseEvent))
	}{
		{"OnPointerDown", "onpointerdown", core.OnPointerDown},
		{"OnPointerUp", "onpointerup", core.OnPointerUp},
		{"OnPointerMove", "onpointermove", core.OnPointerMove},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			el := core.El("div")
			called := false
			c.attach(el, func(e core.MouseEvent) { called = true })

			handler := core.Unwrap(el).Get(c.propName)
			if !handler.Truthy() {
				t.Fatalf("%s did not set %s", c.name, c.propName)
			}
			fakeEvent := js.Global().Get("MouseEvent").New("pointerevent", map[string]any{"bubbles": true})
			handler.Invoke(fakeEvent)

			if !called {
				t.Errorf("%s handler was not invoked when %s was called directly", c.name, c.propName)
			}
		})
	}
}

// TestMouseEventHandlersNilFnDoesNotPanic covers the shared "if fn == nil
// { return }" guard across every func(e MouseEvent) handler.
func TestMouseEventHandlersNilFnDoesNotPanic(t *testing.T) {
	attaches := []func(el core.Node, fn func(e core.MouseEvent)){
		core.OnClick, core.OnDblClick, core.OnMouseEnter, core.OnMouseLeave,
		core.OnPointerDown, core.OnPointerUp, core.OnPointerMove,
	}
	for _, attach := range attaches {
		el := core.El("div")
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("handler panicked on nil fn: %v", r)
				}
			}()
			attach(el, nil)
		}()
	}
}

// TestEventHandlersNilFnDoesNotPanic covers the nil-fn guard across every
// func(e Event) handler.
func TestEventHandlersNilFnDoesNotPanic(t *testing.T) {
	attaches := []func(el core.Node, fn func(e core.Event)){
		core.OnBlur, core.OnFocus, core.OnScroll, core.OnDragStart, core.OnDrop,
	}
	for _, attach := range attaches {
		el := core.El("div")
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("handler panicked on nil fn: %v", r)
				}
			}()
			attach(el, nil)
		}()
	}
}

// TestTouchEventHandlersNilFnDoesNotPanic covers OnTouchStart/OnTouchEnd's
// func(e TouchEvent) nil guard.
func TestTouchEventHandlersNilFnDoesNotPanic(t *testing.T) {
	attaches := []func(el core.Node, fn func(e core.TouchEvent)){core.OnTouchStart, core.OnTouchEnd}
	for _, attach := range attaches {
		el := core.El("div")
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("handler panicked on nil fn: %v", r)
				}
			}()
			attach(el, nil)
		}()
	}
}

// TestInputEventHandlersNilFnDoesNotPanic covers the nil-fn guard for the
// func(e InputEvent) handlers (OnInput, OnChange).
func TestInputEventHandlersNilFnDoesNotPanic(t *testing.T) {
	attaches := []func(el core.Node, fn func(e core.InputEvent)){core.OnInput, core.OnChange}
	for _, attach := range attaches {
		el := core.El("input")
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("handler panicked on nil fn: %v", r)
				}
			}()
			attach(el, nil)
		}()
		dispatch(t, el, "input") // must not panic even with no handler attached
	}
}

// TestOnCheckedNilFnDoesNotPanic covers OnChecked's func(e CheckEvent) nil guard.
func TestOnCheckedNilFnDoesNotPanic(t *testing.T) {
	el := core.El("input")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("OnChecked(nil) panicked: %v", r)
		}
	}()
	core.OnChecked(el, nil)
	dispatch(t, el, "change")
}

// TestKeyEventHandlersNilFnDoesNotPanic covers OnKeyDown/OnKeyUp's
// func(e KeyEvent) nil guard.
func TestKeyEventHandlersNilFnDoesNotPanic(t *testing.T) {
	attaches := []func(el core.Node, fn func(e core.KeyEvent)){core.OnKeyDown, core.OnKeyUp}
	for _, attach := range attaches {
		el := core.El("input")
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("handler panicked on nil fn: %v", r)
				}
			}()
			attach(el, nil)
		}()
	}
}

// TestOnSubmitNilFnDoesNotPanic covers OnSubmit's func(e SubmitEvent) nil guard
// (excluded from the shared Event table since it's dispatched separately
// alongside its preventDefault behavior test below).
func TestOnSubmitNilFnDoesNotPanic(t *testing.T) {
	form := core.El("form")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("OnSubmit(nil) panicked: %v", r)
		}
	}()
	core.OnSubmit(form, nil)
	dispatch(t, form, "submit")
}

func TestOnSubmitCallsPreventDefaultAndPassesTarget(t *testing.T) {
	form := core.El("form")
	called := false
	var got core.SubmitEvent
	core.OnSubmit(form, func(e core.SubmitEvent) { called = true; got = e })

	event := js.Global().Get("Event").New("submit", map[string]any{"bubbles": true, "cancelable": true})
	core.Unwrap(form).Call("dispatchEvent", event)

	if !called {
		t.Error("OnSubmit handler was not invoked")
	}
	if !event.Get("defaultPrevented").Bool() {
		t.Error("OnSubmit did not call preventDefault")
	}
	if !core.Unwrap(got.Target()).Equal(core.Unwrap(form)) {
		t.Error("SubmitEvent.Target() did not unwrap to the form")
	}
}
