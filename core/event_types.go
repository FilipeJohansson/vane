//go:build js && wasm

package core

import (
	"syscall/js"

	"github.com/filipejohansson/vane/internal/dom"
)

// baseEvent is embedded into every typed event struct below, hiding the
// underlying syscall/js.Value the same way core.Node hides it for the DOM
// tree (core/node.go) - no On* handler hands application code a raw
// js.Value, typed event structs included.
type baseEvent struct{ raw js.Value }

func (e baseEvent) PreventDefault()  { e.raw.Call(dom.PreventDefault) }
func (e baseEvent) StopPropagation() { e.raw.Call(dom.StopPropagation) }

// Event is the generic decoded event: which element it actually landed on
// vs. which element the listener is attached to. Used for handlers whose
// native event carries no category-specific data beyond that (focus, blur,
// scroll, drag/drop).
type Event struct {
	baseEvent
	target, currentTarget js.Value
}

func (e Event) Target() Node        { return wrap(e.target) }
func (e Event) CurrentTarget() Node { return wrap(e.currentTarget) }
func (e Event) Self() bool          { return e.target.Equal(e.currentTarget) }

func newEvent(v js.Value) Event {
	return Event{baseEvent{v}, v.Get("target"), v.Get("currentTarget")}
}

// KeyEvent carries the data a keyboard handler commonly needs: which key,
// which modifiers. Used by both element-scoped and window-scoped keydown/keyup.
type KeyEvent struct {
	baseEvent
	Key                    string
	Meta, Ctrl, Shift, Alt bool
}

func newKeyEvent(v js.Value) KeyEvent {
	return KeyEvent{
		baseEvent{v},
		v.Get("key").String(),
		v.Get("metaKey").Bool(), v.Get("ctrlKey").Bool(),
		v.Get("shiftKey").Bool(), v.Get("altKey").Bool(),
	}
}

// MouseEvent carries the data a mouse/pointer handler commonly needs:
// coordinates, button, modifiers, and (like Event) which element the click
// actually landed on vs. the listener's own element.
type MouseEvent struct {
	baseEvent
	ClientX, ClientY       float64
	Button                 int
	Meta, Ctrl, Shift, Alt bool
	target, currentTarget  js.Value
}

func (e MouseEvent) Target() Node        { return wrap(e.target) }
func (e MouseEvent) CurrentTarget() Node { return wrap(e.currentTarget) }
func (e MouseEvent) Self() bool          { return e.target.Equal(e.currentTarget) }

func newMouseEvent(v js.Value) MouseEvent {
	return MouseEvent{
		baseEvent{v},
		v.Get("clientX").Float(), v.Get("clientY").Float(), v.Get("button").Int(),
		v.Get("metaKey").Bool(), v.Get("ctrlKey").Bool(), v.Get("shiftKey").Bool(), v.Get("altKey").Bool(),
		v.Get("target"), v.Get("currentTarget"),
	}
}

// InputEvent carries the current field value, decoded once, for OnInput/OnChange.
type InputEvent struct {
	baseEvent
	Value string
}

func newInputEvent(v js.Value) InputEvent {
	return InputEvent{baseEvent{v}, v.Get("target").Get("value").String()}
}

// CheckEvent carries the current checked state for OnChecked.
type CheckEvent struct {
	baseEvent
	Checked bool
}

func newCheckEvent(v js.Value) CheckEvent {
	return CheckEvent{baseEvent{v}, v.Get("target").Get("checked").Bool()}
}

// SubmitEvent is passed to OnSubmit. preventDefault is still called
// automatically before fn runs, so fn doesn't have to remember to call it;
// e is passed anyway for Target/StopPropagation.
type SubmitEvent struct {
	baseEvent
	target js.Value
}

func (e SubmitEvent) Target() Node { return wrap(e.target) }

func newSubmitEvent(v js.Value) SubmitEvent {
	return SubmitEvent{baseEvent{v}, v.Get("target")}
}

// TouchPoint is one entry from a TouchEvent's touches list.
type TouchPoint struct{ ClientX, ClientY float64 }

// TouchEvent carries every active touch point for OnTouchStart/OnTouchEnd.
type TouchEvent struct {
	baseEvent
	Touches               []TouchPoint
	target, currentTarget js.Value
}

func (e TouchEvent) Target() Node        { return wrap(e.target) }
func (e TouchEvent) CurrentTarget() Node { return wrap(e.currentTarget) }

func newTouchEvent(v js.Value) TouchEvent {
	list := v.Get("touches")
	n := list.Get("length").Int()
	points := make([]TouchPoint, n)
	for i := 0; i < n; i++ {
		t := list.Index(i)
		points[i] = TouchPoint{t.Get("clientX").Float(), t.Get("clientY").Float()}
	}
	return TouchEvent{baseEvent{v}, points, v.Get("target"), v.Get("currentTarget")}
}
