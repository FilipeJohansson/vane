//go:build js && wasm

package core

import "github.com/filipejohansson/vane/internal/dom"

// HTMLProps holds common HTML attributes shared by all elements.
// Embed this in component-specific props structs to inherit standard fields.
//
//	type LinkProps struct {
//	    core.HTMLProps
//	    To string
//	}
type HTMLProps struct {
	Class    string
	ID       string
	Style    string
	Role     string
	TabIndex int // 0 = not set (default tab order); use -1 to remove from tab order
	Hidden   bool
	OnClick  func(MouseEvent)
	OnFocus  func(Event)
	OnBlur   func(Event)
	// OnMouseEnter, OnMouseLeave, OnDblClick, OnKeyDown, and OnKeyUp are the
	// other events that fire on any element regardless of tag (unlike
	// OnInput/OnChange/OnChecked/OnSubmit, which only make sense on form
	// controls and belong on a form-specific props struct instead).
	OnMouseEnter func(MouseEvent)
	OnMouseLeave func(MouseEvent)
	OnDblClick   func(MouseEvent)
	OnKeyDown    func(KeyEvent)
	OnKeyUp      func(KeyEvent)
	// Extra holds arbitrary key/value attributes (e.g. data-*, aria-*).
	Extra map[string]string
}

// ElWithProps creates a tag element, applies p onto it, and appends children.
// Shorthand for the El + Apply + AppendChild sequence components composing
// HTMLProps (or any embedding props struct) would otherwise repeat by hand.
// Custom attrs beyond the fixed HTMLProps fields (href, aria-*, data-*, ...)
// go through p.Extra, applied via setAttribute with no key restriction.
func ElWithProps(tag string, p HTMLProps, children ...Node) Node {
	el := El(tag)
	p.Apply(el)
	for _, c := range children {
		AppendChild(el, c)
	}
	return el
}

// Apply sets the HTMLProps fields onto el.
func (p HTMLProps) Apply(el Node) {
	raw := Unwrap(el)
	if p.Class != "" {
		raw.Set(dom.ClassName, p.Class)
	}
	if p.ID != "" {
		raw.Set(dom.Id, p.ID)
	}
	if p.Style != "" {
		raw.Set(dom.Style, p.Style)
	}
	if p.Role != "" {
		raw.Call(dom.SetAttribute, "role", p.Role)
	}
	if p.TabIndex != 0 {
		raw.Set("tabIndex", p.TabIndex)
	}
	if p.Hidden {
		raw.Set("hidden", true)
	}
	for k, v := range p.Extra {
		raw.Call(dom.SetAttribute, k, v)
	}
	OnClick(el, p.OnClick)
	OnFocus(el, p.OnFocus)
	OnBlur(el, p.OnBlur)
	OnMouseEnter(el, p.OnMouseEnter)
	OnMouseLeave(el, p.OnMouseLeave)
	OnDblClick(el, p.OnDblClick)
	OnKeyDown(el, p.OnKeyDown)
	OnKeyUp(el, p.OnKeyUp)
}
