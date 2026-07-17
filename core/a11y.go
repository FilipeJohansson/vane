//go:build js && wasm

package core

import (
	"syscall/js"
	"time"

	"github.com/filipejohansson/vane/internal/dom"
)

// Focus moves keyboard focus to el. No-op on a nil/empty Node.
func Focus(el Node) {
	if isNilNode(el) {
		return
	}
	Unwrap(el).Call(dom.Focus)
}

// focusableSelector matches the standard set of natively-focusable elements,
// the same list focus-trap libraries use. It excludes disabled controls and
// anything explicitly removed from the tab order (tabindex="-1").
const focusableSelector = `a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])`

// FocusTrap confines Tab/Shift+Tab focus cycling to el's focusable
// descendants, for modals, dialogs, and other overlays that must not leak
// keyboard focus to the rest of the page while open. Moves focus to el's
// first focusable descendant immediately when called.
//
// Call it once the dialog element exists in the DOM (e.g. right after a
// `ref={&dialogEl}` assignment, or inside a Portal's render function), and
// register the returned cleanup (via core.OnDispose, or an Effect's own
// cleanup) to run when the overlay closes. The cleanup removes the trap's
// listener and restores focus to whatever was focused before the trap
// engaged (the standard pattern for returning focus to the button that
// opened the dialog).

func FocusTrap(el Node) func() {
	raw := Unwrap(el)
	prevFocused := dom.Document.Get(dom.ActiveElement)

	focusables := func() js.Value {
		return raw.Call(dom.QuerySelectorAll, focusableSelector)
	}

	handler := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		event := args[0]
		if event.Get("key").String() != "Tab" {
			return nil
		}
		list := focusables()
		n := list.Get("length").Int()
		if n == 0 {
			event.Call(dom.PreventDefault)
			return nil
		}
		first := list.Index(0)
		last := list.Index(n - 1)
		active := dom.Document.Get(dom.ActiveElement)
		shift := event.Get("shiftKey").Bool()
		switch {
		case shift && active.Equal(first):
			event.Call(dom.PreventDefault)
			last.Call(dom.Focus)
		case !shift && active.Equal(last):
			event.Call(dom.PreventDefault)
			first.Call(dom.Focus)
		}
		return nil
	})
	raw.Call(dom.AddEventListener, dom.EventKeyDown, handler)

	if list := focusables(); list.Get("length").Int() > 0 {
		list.Index(0).Call(dom.Focus)
	} else {
		raw.Call(dom.Focus) // fallback: el needs tabIndex={-1} to be focusable itself
	}

	return func() {
		raw.Call(dom.RemoveEventListener, dom.EventKeyDown, handler)
		handler.Release()
		if prevFocused.Truthy() && prevFocused.Get(dom.Focus).Truthy() {
			prevFocused.Call(dom.Focus)
		}
	}
}

var announceRegion js.Value

func ensureAnnounceRegion() js.Value {
	if !announceRegion.Truthy() {
		el := dom.Document.Call(dom.CreateElement, "div")
		el.Call(dom.SetAttribute, "aria-live", "polite")
		el.Call(dom.SetAttribute, "aria-atomic", "true")
		// Visually hidden but still in the accessibility tree (the standard
		// "sr-only" pattern). display:none/visibility:hidden would instead
		// remove it from the tree, so screen readers would never announce it.
		el.Get(dom.Style).Set("cssText",
			"position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0")
		dom.Document.Get("body").Call(dom.AppendChild, el)
		announceRegion = el
	}
	return announceRegion
}

// Announce speaks msg to screen readers via a shared, visually-hidden
// aria-live region (created lazily on first use). Safe to call repeatedly
// with the same message, since the region is cleared first so screen readers
// re-announce it even when the text is identical to last time (aria-live
// only fires on an actual content change, so setting the same text twice in
// a row without clearing first would only announce once).
func Announce(msg string) {
	region := ensureAnnounceRegion()
	region.Set(dom.TextContent, "")
	time.AfterFunc(50*time.Millisecond, func() {
		region.Set(dom.TextContent, msg)
	})
}
