//go:build js && wasm

package core_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.
//
// jsdom has no layout engine, so BoundingRect/Viewport/ScrollY/
// DocumentHeight/ScrollIntoView return zeroed or stubbed values here (see
// wasm_test_exec.js's scrollTo stub and testing.md's layout-engine gap).
// These tests verify the wrapper calls through and returns the right shape,
// not real layout numbers.

import (
	"syscall/js"
	"testing"

	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/core/signal"
)

func TestOnWindowEventInvokesHandlerOnDispatch(t *testing.T) {
	called := false
	scope := signal.RunScoped(func() {
		core.OnWindowEvent("vane-test-event", func(e core.Event) { called = true })
	})
	t.Cleanup(scope.Dispose)

	event := js.Global().Get("Event").New("vane-test-event")
	js.Global().Get("window").Call("dispatchEvent", event)

	if !called {
		t.Error("OnWindowEvent handler was not invoked on a dispatched event")
	}
}

func TestOnWindowEventRemovesListenerOnDispose(t *testing.T) {
	called := false
	scope := signal.RunScoped(func() {
		core.OnWindowEvent("vane-test-event-2", func(e core.Event) { called = true })
	})
	scope.Dispose()

	event := js.Global().Get("Event").New("vane-test-event-2")
	js.Global().Get("window").Call("dispatchEvent", event)

	if called {
		t.Error("OnWindowEvent handler fired after its scope was disposed")
	}
}

func TestOnWindowEventNilFnDoesNotPanicOrAttach(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("OnWindowEvent(nil) panicked: %v", r)
		}
	}()
	scope := signal.RunScoped(func() {
		core.OnWindowEvent("vane-test-event-3", nil)
	})
	t.Cleanup(scope.Dispose)

	event := js.Global().Get("Event").New("vane-test-event-3")
	js.Global().Get("window").Call("dispatchEvent", event) // must not panic
}

func TestOnWindowMouseMovePassesClientCoordinates(t *testing.T) {
	var gotX, gotY float64
	called := false
	scope := signal.RunScoped(func() {
		core.OnWindowMouseMove(func(e core.MouseEvent) {
			called = true
			gotX, gotY = e.ClientX, e.ClientY
		})
	})
	t.Cleanup(scope.Dispose)

	event := js.Global().Get("MouseEvent").New("mousemove", map[string]any{"clientX": 42, "clientY": 17})
	js.Global().Get("window").Call("dispatchEvent", event)

	if !called {
		t.Fatal("OnWindowMouseMove handler was not invoked on a dispatched mousemove event")
	}
	if gotX != 42 || gotY != 17 {
		t.Errorf("OnWindowMouseMove received (%v, %v), want (42, 17)", gotX, gotY)
	}
}

func TestOnWindowMouseMoveNilFnDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("OnWindowMouseMove(nil) panicked: %v", r)
		}
	}()
	scope := signal.RunScoped(func() {
		core.OnWindowMouseMove(nil)
	})
	t.Cleanup(scope.Dispose)

	event := js.Global().Get("MouseEvent").New("mousemove", map[string]any{"clientX": 1, "clientY": 1})
	js.Global().Get("window").Call("dispatchEvent", event) // must not panic
}

func TestOnWindowKeyDownDecodesKeyAndModifiers(t *testing.T) {
	var got core.KeyEvent
	called := false
	scope := signal.RunScoped(func() {
		core.OnWindowKeyDown(func(e core.KeyEvent) {
			called = true
			got = e
		})
	})
	t.Cleanup(scope.Dispose)

	event := js.Global().Get("KeyboardEvent").New("keydown", map[string]any{
		"key": "k", "ctrlKey": true, "metaKey": false, "shiftKey": false, "altKey": false,
	})
	js.Global().Get("window").Call("dispatchEvent", event)

	if !called {
		t.Fatal("OnWindowKeyDown handler was not invoked on a dispatched keydown event")
	}
	if got.Key != "k" || !got.Ctrl || got.Meta || got.Shift || got.Alt {
		t.Errorf("OnWindowKeyDown decoded %+v, want Key=k Ctrl=true, rest false", got)
	}
}

func TestOnWindowKeyDownRemovesListenerOnDispose(t *testing.T) {
	called := false
	scope := signal.RunScoped(func() {
		core.OnWindowKeyDown(func(e core.KeyEvent) { called = true })
	})
	scope.Dispose()

	event := js.Global().Get("KeyboardEvent").New("keydown", map[string]any{"key": "a"})
	js.Global().Get("window").Call("dispatchEvent", event)

	if called {
		t.Error("OnWindowKeyDown handler fired after its scope was disposed")
	}
}

func TestOnWindowKeyDownNilFnDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("OnWindowKeyDown(nil) panicked: %v", r)
		}
	}()
	scope := signal.RunScoped(func() {
		core.OnWindowKeyDown(nil)
	})
	t.Cleanup(scope.Dispose)

	event := js.Global().Get("KeyboardEvent").New("keydown", map[string]any{"key": "a"})
	js.Global().Get("window").Call("dispatchEvent", event) // must not panic
}

func TestBoundingRectNilNodeReturnsZeroRect(t *testing.T) {
	got := core.BoundingRect(nil)
	want := core.Rect{}
	if got != want {
		t.Errorf("BoundingRect(nil) = %+v, want zero Rect", got)
	}
}

func TestBoundingRectRealNodeDoesNotPanic(t *testing.T) {
	el := core.El("div")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("BoundingRect panicked: %v", r)
		}
	}()
	core.BoundingRect(el) // jsdom has no layout engine, just checking it doesn't crash
}

func TestViewportMatchesWindowDimensions(t *testing.T) {
	w, h := core.Viewport()
	wantW := js.Global().Get("innerWidth").Float()
	wantH := js.Global().Get("innerHeight").Float()
	if w != wantW || h != wantH {
		t.Errorf("Viewport() = (%v, %v), want (%v, %v)", w, h, wantW, wantH)
	}
}

func TestScrollYMatchesWindowScrollY(t *testing.T) {
	got := core.ScrollY()
	want := js.Global().Get("scrollY").Float()
	if got != want {
		t.Errorf("ScrollY() = %v, want %v", got, want)
	}
}

func TestDocumentHeightMatchesDocumentElement(t *testing.T) {
	scrollH, clientH := core.DocumentHeight()
	docEl := js.Global().Get("document").Get("documentElement")
	if scrollH != docEl.Get("scrollHeight").Float() {
		t.Errorf("DocumentHeight() scrollHeight = %v, want %v", scrollH, docEl.Get("scrollHeight").Float())
	}
	if clientH != docEl.Get("clientHeight").Float() {
		t.Errorf("DocumentHeight() clientHeight = %v, want %v", clientH, docEl.Get("clientHeight").Float())
	}
}

func TestScrollIntoViewNilNodeDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ScrollIntoView(nil) panicked: %v", r)
		}
	}()
	core.ScrollIntoView(nil, true)
}

func TestScrollIntoViewRealNodeDoesNotPanic(t *testing.T) {
	el := core.El("div")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ScrollIntoView panicked: %v", r)
		}
	}()
	core.ScrollIntoView(el, false)
}

func TestScrollToDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ScrollTo panicked: %v", r)
		}
	}()
	core.ScrollTo(0, true) // wasm_test_exec.js stubs window.scrollTo to a no-op
}

func TestGetElementByIDFindsAttachedElement(t *testing.T) {
	raw := js.Global().Get("document").Call("createElement", "div")
	raw.Call("setAttribute", "id", "vane-test-lookup")
	js.Global().Get("document").Get("body").Call("appendChild", raw)
	t.Cleanup(func() { raw.Call("remove") })

	n, ok := core.GetElementByID("vane-test-lookup")
	if !ok {
		t.Fatal("GetElementByID did not find an attached element with a matching id")
	}
	if !core.Unwrap(n).Equal(raw) {
		t.Error("GetElementByID returned a Node wrapping a different element")
	}
}

func TestGetElementByIDMissingIDReturnsFalse(t *testing.T) {
	_, ok := core.GetElementByID("vane-test-does-not-exist")
	if ok {
		t.Error("GetElementByID(missing id) returned ok=true, want false")
	}
}

func TestWriteClipboardTextReturnsErrorWhenClipboardUnavailable(t *testing.T) {
	// jsdom does not implement the Clipboard API, navigator.clipboard is
	// undefined, so this exercises the synchronous feature-detection path.
	if err := core.WriteClipboardText("hello"); err == nil {
		t.Error("WriteClipboardText returned nil error under jsdom, where navigator.clipboard is unavailable")
	}
}

func TestLocalStorageRoundTrip(t *testing.T) {
	const key = "vane-test-storage-key"
	t.Cleanup(func() { js.Global().Get("localStorage").Call("removeItem", key) })

	if _, ok := core.LocalStorage().Get(key); ok {
		t.Fatal("LocalStorage().Get on an unset key returned ok=true")
	}

	core.LocalStorage().Set(key, "hello")
	got, ok := core.LocalStorage().Get(key)
	if !ok {
		t.Fatal("LocalStorage().Get after Set returned ok=false")
	}
	if got != "hello" {
		t.Errorf("LocalStorage().Get() = %q, want %q", got, "hello")
	}
}

func TestSetRootAttributeSetsAttributeOnHTML(t *testing.T) {
	t.Cleanup(func() { js.Global().Get("document").Get("documentElement").Call("removeAttribute", "data-vane-test") })

	core.SetRootAttribute("data-vane-test", "value")

	got := js.Global().Get("document").Get("documentElement").Call("getAttribute", "data-vane-test").String()
	if got != "value" {
		t.Errorf("documentElement's data-vane-test = %q, want %q", got, "value")
	}
}

func TestSetRootStyleSetsAndClearsInlineStyle(t *testing.T) {
	docEl := js.Global().Get("document").Get("documentElement")
	t.Cleanup(func() { docEl.Get("style").Call("removeProperty", "overflow") })

	core.SetRootStyle("overflow", "hidden")
	if got := docEl.Get("style").Call("getPropertyValue", "overflow").String(); got != "hidden" {
		t.Errorf("documentElement style overflow = %q, want %q", got, "hidden")
	}

	core.SetRootStyle("overflow", "")
	if got := docEl.Get("style").Call("getPropertyValue", "overflow").String(); got != "" {
		t.Errorf("documentElement style overflow after clearing = %q, want empty", got)
	}
}

func TestWindowReturnsTheGlobalWindowObject(t *testing.T) {
	if !core.Window().Equal(js.Global().Get("window")) {
		t.Error("core.Window() did not return the same object as js.Global().Get(\"window\")")
	}
}

func TestDocumentReturnsTheGlobalDocumentObject(t *testing.T) {
	if !core.Document().Equal(js.Global().Get("document")) {
		t.Error("core.Document() did not return the same object as js.Global().Get(\"document\")")
	}
}
