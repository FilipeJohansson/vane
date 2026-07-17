//go:build js && wasm

package core

import (
	"syscall/js"

	"github.com/filipejohansson/vane/internal/dom"
)

// El creates a DOM element with the given tag name.
func El(tag string) Node {
	return wrap(dom.Document.Call(dom.CreateElement, tag))
}

// Text creates a DOM text node.
func Text(s string) Node {
	return wrap(dom.Document.Call(dom.CreateTextNode, s))
}

// Nodes normalizes a mixed list of children into []Node.
// Accepts string (converted to text node), Node, and []Node.
// Use in components that want to accept plain strings alongside DOM nodes:
//
//	func Button(props ButtonProps, children ...any) core.Node {
//	    nodes := core.Nodes(children...)
//	    ...
//	}
func Nodes(items ...any) []Node {
	out := make([]Node, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case Node:
			out = append(out, v)
		case string:
			out = append(out, Text(v))
		case []Node:
			out = append(out, v...)
		case []any:
			out = append(out, Nodes(v...)...)
		}
	}
	return out
}

// Spread converts a variadic children argument into []Node for use in
// {children...} spreads. Handles both []Node and []any (the latter allows
// component authors to declare children ...any so callers can pass plain strings).
func Spread(children any) []Node {
	switch v := children.(type) {
	case []Node:
		return v
	case []any:
		return Nodes(v...)
	}
	return nil
}

// Empty returns a Node representing "no element".
// Use in conditional components instead of returning nil.
func Empty() Node {
	return wrap(js.Undefined())
}

// Fragment groups multiple nodes without a wrapper DOM element.
// When appended to a real element the browser moves Fragment's children in place,
// leaving no extra node in the final DOM.
func Fragment(children ...Node) Node {
	frag := dom.Document.Call(dom.CreateDocumentFragment)
	for _, c := range children {
		if !isNilNode(c) {
			frag.Call(dom.AppendChild, Unwrap(c))
		}
	}
	return wrap(frag)
}

// AppendChild appends child to parent. No-ops on zero/null/undefined children.
func AppendChild(parent, child Node) {
	if isNilNode(child) {
		return
	}
	Unwrap(parent).Call(dom.AppendChild, Unwrap(child))
}

// AppendText appends a static text node to parent.
func AppendText(parent Node, text string) {
	Unwrap(parent).Call(dom.AppendChild, Unwrap(Text(text)))
}
