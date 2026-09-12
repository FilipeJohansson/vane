//go:build js && wasm

package core

import (
	"syscall/js"

	"github.com/filipejohansson/vane/core/signal"
	"github.com/filipejohansson/vane/internal/dom"
)

// Portal renders fn() into a DOM element outside the current component hierarchy.
// target is a CSS selector string or a js.Value element.
// The target is resolved after the parent component's DOM is mounted, so Portal
// targets can be elements created by the parent's own vane return.
// fn() runs in a dedicated reactive Effect so signal reads inside fn() subscribe
// Portal's own Effect rather than the parent's, preventing unwanted re-mounts.
// Multiple portals to the same target append (concatenate) their content.
// Returns an empty placeholder in the caller's DOM position.
// Content is removed automatically when the calling component unmounts.
func Portal(target any, fn func() Node) Node {
	if fn == nil {
		Warn("core.Portal called with a nil render function, rendering nothing")
		return Empty()
	}

	var current js.Value
	var portalScope *signal.Scope
	var disposePortal func()
	disposed := false

	// Register cleanup while the caller's component scope is active. The first
	// render below is asynchronous and must not decide ownership from a later
	// goroutine context.
	signal.RegisterDispose(func() {
		disposed = true
		if disposePortal != nil {
			disposePortal()
		}
		if portalScope != nil {
			portalScope.Dispose()
			portalScope = nil
		}
		if current.Truthy() {
			current.Call("remove")
		}
	})

	// Start the reactive Effect in a goroutine so that the parent's DOM is
	// fully mounted before we do querySelector. In WASM cooperative
	// scheduling this runs after the current goroutine yields (i.e. after
	// core.Mount appends the parent's DOM to the document).
	go func() {
		if disposed {
			return
		}

		// Own Effect: signal reads inside fn() track this Effect, not the parent's.
		disposePortal = signal.Effect(func() {
			if portalScope != nil {
				portalScope.Dispose()
			}
			portalScope = signal.RunScoped(func() {
				node := fn()

				// A nil node (e.g. a closed modal) means there's genuinely
				// nothing to place yet, so dest isn't resolved here at all -
				// resolving it unconditionally would burn Warn's one-per-
				// message dedup slot on a false "target not found" before
				// the caller ever tries to show anything, masking a real
				// failure later.
				if isNilNode(node) {
					if current.Truthy() {
						// A destination was resolved on some earlier run
						// (there was content then); keep a placeholder there
						// so this portal's position among sibling portals
						// appended to the same target survives if it renders
						// real content again later. If dest has since
						// disappeared, there's nothing to put a placeholder
						// into, so just drop the old node.
						if dest := resolvePortalTarget(target); dest.Truthy() {
							empty := dom.Document.Call(dom.CreateTextNode, "")
							current.Call(dom.ReplaceWith, empty)
							current = empty
						} else {
							current.Call("remove")
							current = js.Value{}
						}
					}
					return
				}

				// Resolved on every run with real content, not once and
				// cached, so a target missing on an earlier run doesn't stop
				// fn() from being retried later - a target that shows up
				// after the fact self-heals. Warn (deduped per message in
				// resolvePortalTarget) only fires when there's real content
				// with nowhere to put it, i.e. exactly when a user action
				// needed it.
				dest := resolvePortalTarget(target)
				if !dest.Truthy() {
					return
				}

				raw := Unwrap(node)
				if current.Truthy() {
					current.Call(dom.ReplaceWith, raw)
				} else {
					dest.Call(dom.AppendChild, raw)
				}
				current = raw
			})
		})
	}()

	return Empty()
}

func resolvePortalTarget(target any) js.Value {
	switch v := target.(type) {
	case string:
		dest := dom.Document.Call(dom.QuerySelector, v)
		if dest.IsNull() || dest.IsUndefined() {
			Warn("core.Portal: target not found: " + v)
			return js.Value{}
		}
		return dest
	case Node:
		return Unwrap(v)
	case js.Value:
		return v
	default:
		Warn("core.Portal: target must be a string selector, core.Node, or js.Value")
		return js.Value{}
	}
}
