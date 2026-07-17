//go:build js && wasm

package core_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.

import (
	"strings"
	"syscall/js"
	"testing"

	"github.com/filipejohansson/vane/core"
)

// TestSetPropBlocksInnerHTMLAndOuterHTML is a security regression test. The
// generic prop passthrough (any unrecognized key falls through to el.Set)
// would otherwise let "innerHTML"/"outerHTML" silently inject raw HTML with
// no XSS warning and no "dangerous" naming, a second, undocumented path
// alongside core.DangerousInnerHTML. Both the JSX-attribute form
// (<div innerHTML={x}>, which compiles to this exact call) and calling
// core.SetProp directly must be blocked.
func TestSetPropBlocksInnerHTMLAndOuterHTML(t *testing.T) {
	for _, key := range []string{"innerHTML", "outerHTML"} {
		el := core.El("div")
		core.SetProp(el, key, "<b>injected</b>")

		raw := core.Unwrap(el)
		if got := raw.Get("children").Get("length").Int(); got != 0 {
			t.Errorf("%s: children.length = %d, want 0 (payload must not be parsed as markup)", key, got)
		}
		if got := raw.Get("innerHTML").String(); got != "" {
			t.Errorf("%s: innerHTML = %q, want empty", key, got)
		}
	}
}

func TestSetPropDisabledSetsBoolProperty(t *testing.T) {
	btn := core.El("button")
	core.SetProp(btn, "disabled", true)
	if !core.Unwrap(btn).Get("disabled").Bool() {
		t.Error("disabled = false, want true")
	}
}

func TestSetPropDisabledIgnoresNonBoolValue(t *testing.T) {
	btn := core.El("button")
	core.SetProp(btn, "disabled", "true") // wrong type, must not set anything
	if core.Unwrap(btn).Get("disabled").Bool() {
		t.Error("disabled = true, want false (non-bool value must be ignored)")
	}
}

func TestSetPropDataAttributeUsesSetAttribute(t *testing.T) {
	el := core.El("div")
	core.SetProp(el, "data-testid", "card")
	raw := core.Unwrap(el)
	if got := raw.Call("getAttribute", "data-testid").String(); got != "card" {
		t.Errorf("getAttribute(data-testid) = %q, want %q", got, "card")
	}
}

func TestSetPropAriaAttributeUsesSetAttribute(t *testing.T) {
	el := core.El("div")
	core.SetProp(el, "aria-label", "Close")
	raw := core.Unwrap(el)
	if got := raw.Call("getAttribute", "aria-label").String(); got != "Close" {
		t.Errorf("getAttribute(aria-label) = %q, want %q", got, "Close")
	}
}

// TestSetPropAriaAttributeAcceptsBoolValue covers a value type the other
// aria-* test doesn't: a native Go bool (aria-hidden={isHidden}), not a
// string. The data-/aria- branch in setPropValue does fmt.Sprint(val)
// unconditionally, which happens to stringify true/false exactly as ARIA
// expects, but that's worth pinning down explicitly rather than only ever
// exercising the string-value path.
func TestSetPropAriaAttributeAcceptsBoolValue(t *testing.T) {
	el := core.El("div")
	core.SetProp(el, "aria-hidden", true)
	raw := core.Unwrap(el)
	if got := raw.Call("getAttribute", "aria-hidden").String(); got != "true" {
		t.Errorf("aria-hidden = %q, want %q", got, "true")
	}
}

func TestSetPropAriaExpandedUpdatesReactivelyViaDynProp(t *testing.T) {
	expanded := core.NewSignal(false)
	el := core.El("button")
	core.DynProp(el, "aria-expanded", func() any {
		if expanded.Get() {
			return "true"
		}
		return "false"
	})

	raw := core.Unwrap(el)
	if got := raw.Call("getAttribute", "aria-expanded").String(); got != "false" {
		t.Fatalf("aria-expanded = %q, want %q", got, "false")
	}

	expanded.Set(true)
	waitFineEffects(t)

	if got := raw.Call("getAttribute", "aria-expanded").String(); got != "true" {
		t.Errorf("aria-expanded after signal change = %q, want %q", got, "true")
	}
}

// TestRolePropertyAssignmentReflectsRealAttribute is an accessibility audit
// finding, not just a regression test. Unlike aria-*/data-* (explicitly
// routed to setAttribute in setPropValue), "role" falls through the generic
// property-assignment path (el.Set("role", v)). This only produces a real,
// screen-reader-visible role="..." attribute because "role" happens to be a
// browser-reflected IDL property, the same shape of bug we found and fixed
// for <meta property="og:..."> (which is NOT reflected). Confirms plain
// role="dialog" vane syntax and core.HTMLProps{Role: "dialog"} (which does
// use setAttribute explicitly) both end up with the identical, correct
// result. If a future browser/jsdom version stops reflecting "role", this
// test is what will catch it.
func TestRolePropertyAssignmentReflectsRealAttribute(t *testing.T) {
	el := core.El("div")
	core.SetProp(el, "role", "dialog")
	raw := core.Unwrap(el)
	if !raw.Call("hasAttribute", "role").Bool() {
		t.Fatal("role attribute not present: core.SetProp's generic property-assignment path no longer reflects role (see HTMLProps.Apply's explicit setAttribute path as the fix if this ever fails)")
	}
	if got := raw.Call("getAttribute", "role").String(); got != "dialog" {
		t.Errorf("role attribute = %q, want %q", got, "dialog")
	}
}

func TestSetPropStringValue(t *testing.T) {
	el := core.El("div")
	core.SetProp(el, "className", "card")
	if got := core.Unwrap(el).Get("className").String(); got != "card" {
		t.Errorf("className = %q, want %q", got, "card")
	}
}

// TestSetPropWarnsOnDivergentAttributeName verifies that writing the HTML
// attribute spelling (e.g. "class") instead of the real DOM property name
// ("className") triggers a core.Warn pointing at the correct name, since the
// generic passthrough would otherwise silently create a useless expando
// property and do nothing.
func TestSetPropWarnsOnDivergentAttributeName(t *testing.T) {
	var calls []string
	raw := jsGlobalConsole(t)
	orig := raw.Get("warn")
	raw.Set("warn", jsFuncCapture(t, &calls))
	defer raw.Set("warn", orig)

	el := core.El("div")
	core.SetProp(el, "class", "card "+t.Name()) // unique per test so dedup doesn't hide it

	if len(calls) != 1 {
		t.Fatalf("console.warn called %d times, want 1: %v", len(calls), calls)
	}
	if !strings.Contains(calls[0], "className") {
		t.Errorf("warning = %q, want it to mention %q", calls[0], "className")
	}
	if got := core.Unwrap(el).Get("className").String(); got != "" {
		t.Errorf("className = %q, want empty (the real property was never set)", got)
	}
}

// TestSetPropNoWarningForCorrectPropertyName is the negative counterpart:
// using the real property name must never warn.
func TestSetPropNoWarningForCorrectPropertyName(t *testing.T) {
	var calls []string
	raw := jsGlobalConsole(t)
	orig := raw.Get("warn")
	raw.Set("warn", jsFuncCapture(t, &calls))
	defer raw.Set("warn", orig)

	el := core.El("label")
	core.SetProp(el, "htmlFor", "field-"+t.Name())

	if len(calls) != 0 {
		t.Errorf("console.warn called %d times, want 0: %v", len(calls), calls)
	}
}

func TestSetPropBoolValueGeneric(t *testing.T) {
	checkbox := core.El("input")
	core.SetProp(checkbox, "checked", true)
	if !core.Unwrap(checkbox).Get("checked").Bool() {
		t.Error("checked = false, want true")
	}
}

func TestSetPropIntValueIsStringified(t *testing.T) {
	el := core.El("div")
	core.SetProp(el, "tabIndex", 5)
	// setPropValue's default int case stringifies via strconv.Itoa before
	// el.Set. tabIndex is a numeric-reflected IDL attribute, so the DOM
	// coerces the assigned string back to a number on read, and this
	// round-trip (not a literal string readback) is the actual behavior
	// callers rely on.
	raw := core.Unwrap(el)
	if got := raw.Get("tabIndex").Int(); got != 5 {
		t.Errorf("tabIndex = %d, want 5", got)
	}
}

func TestSetPropValueSetsInputValue(t *testing.T) {
	input := core.El("input")
	core.SetProp(input, "value", "hello")
	if got := core.Unwrap(input).Get("value").String(); got != "hello" {
		t.Errorf("value = %q, want %q", got, "hello")
	}
}

func TestSetPropValuePreservesCursorOnFocusedElement(t *testing.T) {
	input := core.El("input")
	raw := core.Unwrap(input)

	// Must be attached to the real document to become document.activeElement.
	docBody := js.Global().Get("document").Get("body")
	docBody.Call("appendChild", raw)
	t.Cleanup(func() { docBody.Call("removeChild", raw) })

	raw.Call("focus")
	raw.Set("value", "hello world")
	raw.Set("selectionStart", 5)
	raw.Set("selectionEnd", 5)

	core.SetProp(input, "value", "hello there world")

	if got := raw.Get("value").String(); got != "hello there world" {
		t.Fatalf("value = %q, want %q", got, "hello there world")
	}
	if got := raw.Get("selectionStart").Int(); got != 5 {
		t.Errorf("selectionStart = %d, want 5 (cursor position must be preserved)", got)
	}
}

func TestSetPropValueSkipsWriteWhenAlreadyMatching(t *testing.T) {
	input := core.El("input")
	raw := core.Unwrap(input)
	raw.Set("value", "same")

	// Should be a no-op, must not panic or error when called with a value
	// that already matches (this is the fast path that avoids disrupting
	// native browser input like IME composition/autocomplete).
	core.SetProp(input, "value", "same")

	if got := raw.Get("value").String(); got != "same" {
		t.Errorf("value = %q, want %q", got, "same")
	}
}

func TestSetStyleAppliesFields(t *testing.T) {
	el := core.El("div")
	core.SetStyle(el, core.Style{
		Display:         "flex",
		Color:           "red",
		BackgroundColor: "blue",
	})

	style := core.Unwrap(el).Get("style")
	if got := style.Get("display").String(); got != "flex" {
		t.Errorf("style.display = %q, want %q", got, "flex")
	}
	if got := style.Get("color").String(); got != "red" {
		t.Errorf("style.color = %q, want %q", got, "red")
	}
	if got := style.Get("backgroundColor").String(); got != "blue" {
		t.Errorf("style.backgroundColor = %q, want %q", got, "blue")
	}
}

func TestSetStyleSkipsEmptyFields(t *testing.T) {
	el := core.El("div")
	core.SetStyle(el, core.Style{Color: "red"}) // Display left empty

	style := core.Unwrap(el).Get("style")
	if got := style.Get("display").String(); got != "" {
		t.Errorf("style.display = %q, want empty (unset fields must not be applied)", got)
	}
}

// TestSetStyleAppliesOneFieldFromEachCategory spot-checks Style.apply's long
// field list (box model, layout, flexbox, grid, typography, background,
// border, effects, ~80 fields total) by setting one representative field
// per category. Style.apply is unexported and only reachable via
// SetProp/DynStyle; exhaustively testing every field would be repetitive
// given they all follow the identical "if non-empty, style.Set(cssKey, v)"
// pattern, so this catches a wrong CSS key name or a category wired to the
// wrong struct field without one test per field.
func TestSetStyleAppliesOneFieldFromEachCategory(t *testing.T) {
	el := core.El("div")
	core.SetStyle(el, core.Style{
		MaxWidth:            "600px",          // box model
		Position:            "absolute",       // layout
		JustifyContent:      "center",         // flexbox
		GridTemplateColumns: "1fr 1fr",        // grid
		FontWeight:          "bold",           // typography
		BackgroundImage:     "url(x.png)",     // background
		BorderRadius:        "8px",            // border
		BoxShadow:           "0 1px 2px #000", // effects
	})

	style := core.Unwrap(el).Get("style")
	cases := map[string]string{
		"maxWidth":            "600px",
		"position":            "absolute",
		"justifyContent":      "center",
		"gridTemplateColumns": "1fr 1fr",
		"fontWeight":          "bold",
		"backgroundImage":     `url("x.png")`,
		"borderRadius":        "8px",
		"boxShadow":           "0 1px 2px #000",
	}
	for cssKey, want := range cases {
		if got := style.Get(cssKey).String(); got != want {
			t.Errorf("style.%s = %q, want %q", cssKey, got, want)
		}
	}
}

func TestHTMLPropsApplySetsAllFields(t *testing.T) {
	el := core.El("div")
	var clicked, focused, blurred, mouseEntered, mouseLeft, dblClicked bool
	var keyDown, keyUp string
	props := core.HTMLProps{
		Class:        "card",
		ID:           "card-1",
		Style:        "color:red",
		Role:         "button",
		TabIndex:     3,
		Hidden:       true,
		OnClick:      func(core.MouseEvent) { clicked = true },
		OnFocus:      func(core.Event) { focused = true },
		OnBlur:       func(core.Event) { blurred = true },
		OnMouseEnter: func(core.MouseEvent) { mouseEntered = true },
		OnMouseLeave: func(core.MouseEvent) { mouseLeft = true },
		OnDblClick:   func(core.MouseEvent) { dblClicked = true },
		OnKeyDown:    func(e core.KeyEvent) { keyDown = e.Key },
		OnKeyUp:      func(e core.KeyEvent) { keyUp = e.Key },
		Extra:        map[string]string{"data-testid": "card"},
	}
	props.Apply(el)

	raw := core.Unwrap(el)
	if got := raw.Get("className").String(); got != "card" {
		t.Errorf("className = %q, want %q", got, "card")
	}
	if got := raw.Get("id").String(); got != "card-1" {
		t.Errorf("id = %q, want %q", got, "card-1")
	}
	if got := raw.Get("style").Get("cssText").String(); got != "color: red;" {
		t.Errorf("style.cssText = %q, want %q", got, "color: red;")
	}
	if got := raw.Call("getAttribute", "role").String(); got != "button" {
		t.Errorf("role attribute = %q, want %q", got, "button")
	}
	if got := raw.Get("tabIndex").Int(); got != 3 {
		t.Errorf("tabIndex = %d, want 3", got)
	}
	if !raw.Get("hidden").Bool() {
		t.Error("hidden = false, want true")
	}
	if got := raw.Call("getAttribute", "data-testid").String(); got != "card" {
		t.Errorf("data-testid attribute = %q, want %q", got, "card")
	}

	dispatchMouse(t, el, "click")
	dispatch(t, el, "focus")
	dispatch(t, el, "blur")
	dispatchMouse(t, el, "mouseenter")
	dispatchMouse(t, el, "mouseleave")
	dispatchMouse(t, el, "dblclick")
	if !clicked || !focused || !blurred || !mouseEntered || !mouseLeft || !dblClicked {
		t.Errorf("clicked=%v focused=%v blurred=%v mouseEntered=%v mouseLeft=%v dblClicked=%v, want all true",
			clicked, focused, blurred, mouseEntered, mouseLeft, dblClicked)
	}

	downEvent := js.Global().Get("KeyboardEvent").New("keydown", map[string]any{"key": "Enter", "bubbles": true})
	core.Unwrap(el).Call("dispatchEvent", downEvent)
	upEvent := js.Global().Get("KeyboardEvent").New("keyup", map[string]any{"key": "Escape", "bubbles": true})
	core.Unwrap(el).Call("dispatchEvent", upEvent)
	if keyDown != "Enter" {
		t.Errorf("OnKeyDown received %q, want %q", keyDown, "Enter")
	}
	if keyUp != "Escape" {
		t.Errorf("OnKeyUp received %q, want %q", keyUp, "Escape")
	}
}

func TestHTMLPropsApplyLeavesUnsetFieldsUntouched(t *testing.T) {
	el := core.El("div")
	core.HTMLProps{}.Apply(el)

	raw := core.Unwrap(el)
	if got := raw.Get("className").String(); got != "" {
		t.Errorf("className = %q, want empty", got)
	}
	if raw.Call("hasAttribute", "role").Bool() {
		t.Error("role attribute set, want absent (zero-value HTMLProps must not touch it)")
	}
}

// TestHTMLPropsApplyTabIndexNegativeOne covers the documented "remove from
// tab order" use of TabIndex. The zero-value check (`if p.TabIndex != 0`)
// must not accidentally treat -1 as "unset".
func TestHTMLPropsApplyTabIndexNegativeOne(t *testing.T) {
	el := core.El("div")
	core.HTMLProps{TabIndex: -1}.Apply(el)

	if got := core.Unwrap(el).Get("tabIndex").Int(); got != -1 {
		t.Errorf("tabIndex = %d, want -1", got)
	}
}

func TestElWithPropsAppliesPropsAndAppendsChildren(t *testing.T) {
	var clicked bool
	el := core.ElWithProps("button", core.HTMLProps{
		Class:   "btn",
		OnClick: func(core.MouseEvent) { clicked = true },
	}, core.Text("Send"))

	raw := core.Unwrap(el)
	if got := raw.Get("tagName").String(); got != "BUTTON" {
		t.Errorf("tagName = %q, want %q", got, "BUTTON")
	}
	if got := raw.Get("className").String(); got != "btn" {
		t.Errorf("className = %q, want %q", got, "btn")
	}
	if got := raw.Get("textContent").String(); got != "Send" {
		t.Errorf("textContent = %q, want %q", got, "Send")
	}

	dispatchMouse(t, el, "click")
	if !clicked {
		t.Error("clicked = false, want true")
	}
}

// TestElWithPropsAppliesExtraAttrs covers using HTMLProps.Extra through
// ElWithProps for attrs with no dedicated field, e.g. an anchor's href.
func TestElWithPropsAppliesExtraAttrs(t *testing.T) {
	el := core.ElWithProps("a", core.HTMLProps{
		Extra: map[string]string{
			"href":   "#/foo",
			"target": "_blank",
		},
	}, core.Text("Go"))

	raw := core.Unwrap(el)
	if got := raw.Call("getAttribute", "href").String(); got != "#/foo" {
		t.Errorf("href attribute = %q, want %q", got, "#/foo")
	}
	if got := raw.Call("getAttribute", "target").String(); got != "_blank" {
		t.Errorf("target attribute = %q, want %q", got, "_blank")
	}
}
