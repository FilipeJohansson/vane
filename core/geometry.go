//go:build js && wasm

package core

import "github.com/filipejohansson/vane/internal/dom"

// GetElementByID looks up an element by its id attribute anywhere in the
// document, not just inside the subtree vane mounted, and returns it as a
// Node, or false if no element has that id. The escape hatch for content
// vane rendered but never kept a Node/ref for, e.g. locating a heading by
// its id for a scroll-spy or "jump to section" link.
func GetElementByID(id string) (Node, bool) {
	el := dom.Document.Call(dom.GetElementById, id)
	if isNilRaw(el) {
		return nil, false
	}
	return wrap(el), true
}

// Rect is a element's position and size, in CSS pixels relative to the
// viewport, matching the fields getBoundingClientRect returns.
type Rect struct {
	Top    float64
	Right  float64
	Bottom float64
	Left   float64
	Width  float64
	Height float64
}

// BoundingRect returns el's current position and size relative to the
// viewport. Returns the zero Rect for a nil/empty Node.
func BoundingRect(el Node) Rect {
	if isNilNode(el) {
		return Rect{}
	}
	r := Unwrap(el).Call("getBoundingClientRect")
	return Rect{
		Top:    r.Get("top").Float(),
		Right:  r.Get("right").Float(),
		Bottom: r.Get("bottom").Float(),
		Left:   r.Get("left").Float(),
		Width:  r.Get("width").Float(),
		Height: r.Get("height").Float(),
	}
}

// Viewport returns the browser window's inner width/height, in CSS pixels.
func Viewport() (width, height float64) {
	return dom.Window.Get("innerWidth").Float(), dom.Window.Get("innerHeight").Float()
}

// ScrollY returns how far the page has scrolled vertically, in pixels.
func ScrollY() float64 {
	return dom.Window.Get("scrollY").Float()
}

// DocumentHeight returns the document's full scrollable height and the
// visible (viewport) height, e.g. to compute a scroll-percentage as
// ScrollY() / (scrollHeight - clientHeight).
func DocumentHeight() (scrollHeight, clientHeight float64) {
	docEl := dom.Document.Get(dom.DocumentElement)
	return docEl.Get("scrollHeight").Float(), docEl.Get("clientHeight").Float()
}

// ScrollIntoView scrolls el into the visible viewport, smoothly if smooth is
// true. No-op on a nil/empty Node.
func ScrollIntoView(el Node, smooth bool) {
	if isNilNode(el) {
		return
	}
	behavior := "auto"
	if smooth {
		behavior = "smooth"
	}
	Unwrap(el).Call("scrollIntoView", map[string]any{"behavior": behavior, "block": "start"})
}

// ScrollTo scrolls the window to the given vertical position, in pixels,
// smoothly if smooth is true.
func ScrollTo(top float64, smooth bool) {
	behavior := "auto"
	if smooth {
		behavior = "smooth"
	}
	dom.Window.Call("scrollTo", map[string]any{"top": top, "behavior": behavior})
}

// SetRootAttribute sets an attribute on the root <html> element, the one
// element vane never mounts into and so has no Node for (Mount always
// targets an element inside <body>). Used for page-wide state that CSS
// reads via a selector on <html>, e.g. a data-theme attribute driving
// custom properties.
func SetRootAttribute(name, value string) {
	dom.Document.Get(dom.DocumentElement).Call(dom.SetAttribute, name, value)
}

// SetRootStyle sets a single inline CSS property on the root <html> element,
// the same no-Node-for-<html> situation SetRootAttribute solves for
// attributes. An empty value removes the property, restoring the default.
// Typical use: locking page scroll (overflow) while a modal or mobile menu
// is open.
func SetRootStyle(prop, value string) {
	dom.Document.Get(dom.DocumentElement).Get(dom.Style).Call("setProperty", prop, value)
}
