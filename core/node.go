//go:build js && wasm

package core

import (
	"syscall/js"

	"github.com/filipejohansson/vane/internal/dom"
)

// Node is vane's runtime-agnostic tree node. Components return Node instead of
// js.Value so the same component code can eventually target a non-browser
// renderer (e.g. a future server-side HTML renderer) without touching call sites.
//
// Node is a sealed interface, so only this package can implement it. Code that
// needs to reach the underlying browser value (third-party JS interop, direct
// DOM queries) must go through Unwrap.
type Node interface {
	unwrap() js.Value
}

// domNode is the only Node implementation today: a thin wrapper around a
// browser js.Value. A future server renderer would add a second implementation
// that builds an HTML string tree instead.
type domNode struct{ v js.Value }

func (n domNode) unwrap() js.Value { return n.v }

// wrap adapts a raw js.Value into a Node. Internal use only. User code never
// constructs a Node directly, it comes from core.El, core.Text, etc.
func wrap(v js.Value) Node { return domNode{v} }

// Unwrap returns the underlying js.Value for n. Use this as an escape hatch
// for direct DOM/JS interop (third-party libraries, manual querySelector,
// event objects) that core doesn't wrap itself. A nil Node unwraps to
// js.Undefined().
func Unwrap(n Node) js.Value {
	if n == nil {
		return js.Undefined()
	}
	return n.unwrap()
}

// Window returns the browser's window object, the same escape hatch Unwrap
// provides for element Nodes, for a window-level JS call core has no typed
// wrapper for. window is never part of the tree vane mounts into, so it has
// no Node of its own for Unwrap to reach.
func Window() js.Value {
	return dom.Window
}

// Document returns the browser's document object, the same escape hatch as
// Window. For a specific element already in the mounted tree, prefer a ref
// or GetElementByID over querying through Document.
func Document() js.Value {
	return dom.Document
}
