//go:build js && wasm

package core

import (
	"fmt"
	"strconv"
	"syscall/js"

	"github.com/filipejohansson/vane/core/signal"
	"github.com/filipejohansson/vane/internal/dom"
)

func init() {
	signal.EffectPanicHandler = func(r any) {
		js.Global().Get("console").Call("error", fmt.Sprintf("[vane] effect panic: %v", r))
	}
	signal.LoopWatchdogHandler = func(msg string) {
		js.Global().Get("console").Call("error", "[vane] "+msg)
	}
}

// DynChild appends a reactive child to parent.
// Re-runs fn when its signal deps change, replacing the previous content.
// Manages a child Scope to dispose nested effects created by fn on each re-run.
//
// Works for both primitive expressions ({count.Get()}) and component calls that
// may return different elements based on signal state ({errNode()}), including
// multi-root content (e.g. core.Fragment or core.DangerousInnerHTML producing
// several top-level nodes).
func DynChild(parent Node, fn func() any) {
	p := Unwrap(parent)
	// Bracket the managed content with a pair of comment anchors instead of
	// tracking a single child element reference, for two reasons:
	//   - Third-party libs (e.g. Lucide) may replace the child element with a
	//     different node (e.g. <i> → <svg>). Tracking the original reference
	//     causes replaceWith to no-op on the detached node.
	//   - fn() may return multi-root content (a Fragment/DocumentFragment
	//     with several children). A single anchor can only track one
	//     "nextSibling", so removing the old content on re-run would leave
	//     every node after the first orphaned in the DOM.
	// Each re-run removes everything currently between start and end,
	// whatever is there, regardless of who put it there, then inserts the
	// new content before end.
	start := dom.Document.Call(dom.CreateComment, "vane")
	end := dom.Document.Call(dom.CreateComment, "/vane")
	p.Call(dom.AppendChild, start)
	p.Call(dom.AppendChild, end)
	var childScope *signal.Scope

	signal.Effect(func() {
		if childScope != nil {
			childScope.Dispose()
		}
		childScope = signal.RunScoped(func() {
			result := fn()
			var newNode js.Value
			switch v := result.(type) {
			case Node:
				if isNilNode(v) {
					newNode = dom.Document.Call(dom.CreateTextNode, "")
				} else {
					newNode = Unwrap(v)
				}
			default:
				newNode = dom.Document.Call(dom.CreateTextNode, stringify(result))
			}
			parentNode := start.Get("parentNode")
			if parentNode.IsNull() || parentNode.IsUndefined() {
				return
			}
			for {
				next := start.Get("nextSibling")
				if next.IsNull() || next.IsUndefined() || next.Equal(end) {
					break
				}
				parentNode.Call(dom.RemoveChild, next)
			}
			parentNode.Call("insertBefore", newNode, end)
		})
	})
}

// DynList appends a reactive list of children to parent between two marker nodes.
// Re-runs fn when its signal deps change.
//
// Keyed reconciliation is automatic: if the nodes returned by fn have a "key"
// property set (via key={expr} in vane syntax), DynList reconciles by key,
// only moving/removing/inserting what changed. Without keys, the list is rebuilt
// from scratch on every change.
//
// An explicit keyFn can also be passed for cases where nodes are built without vane syntax.
func DynList(parent Node, fn func() []Node, keyFns ...func() []string) {
	p := Unwrap(parent)
	start := dom.Document.Call(dom.CreateTextNode, "")
	end := dom.Document.Call(dom.CreateTextNode, "")
	p.Call(dom.AppendChild, start)
	p.Call(dom.AppendChild, end)

	live := make(map[string]js.Value) // key → DOM node (keyed path only)
	var childScope *signal.Scope      // scope for nested effects (unkeyed path only)

	// explicitKeyFn is set when caller provides a key function directly.
	var explicitKeyFn func() []string
	if len(keyFns) > 0 && keyFns[0] != nil {
		explicitKeyFn = keyFns[0]
	}
	if len(keyFns) > 1 {
		Warn("core.DynList: more than one key function passed, only the first is used")
	}

	signal.Effect(func() {
		wrapped := fn()
		newNodes := make([]js.Value, len(wrapped))
		for i, n := range wrapped {
			if !isNilNode(n) {
				newNodes[i] = Unwrap(n)
			}
		}

		// Resolve keys: explicit keyFn takes priority, then node .key property.
		var newKeys []string
		if explicitKeyFn != nil {
			newKeys = explicitKeyFn()
			if len(newKeys) != len(newNodes) {
				Warn(fmt.Sprintf(
					"core.DynList: key function returned %d keys for %d nodes, falling back to unkeyed rendering for this update",
					len(newKeys), len(newNodes)))
				newKeys = nil
			}
		} else {
			keys := make([]string, len(newNodes))
			keyedCount, presentCount := 0, 0
			for i, n := range newNodes {
				if isNilRaw(n) {
					continue
				}
				presentCount++
				switch k := n.Get("key"); k.Type() {
				case js.TypeString:
					keys[i] = k.String()
					keyedCount++
				case js.TypeNumber:
					keys[i] = strconv.FormatFloat(k.Float(), 'f', -1, 64)
					keyedCount++
				}
			}
			switch {
			case keyedCount == 0:
				// No keys anywhere, plain unkeyed list, the common case.
			case keyedCount == presentCount:
				newKeys = keys
			default:
				// Some nodes have key={...}, others don't. Under the old logic the
				// unkeyed ones would silently disappear from the DOM (treated as ""
				// keys and skipped). Fall back to unkeyed rendering instead, so every
				// node still renders, just without keyed reconciliation this update.
				Warn("core.DynList: some nodes have key={...} and others don't, falling back to unkeyed rendering for this update")
			}
		}

		if newKeys == nil {
			// Unkeyed: dispose nested effects and rebuild.
			if childScope != nil {
				childScope.Dispose()
			}
			// If keyed nodes existed before this update, they get removed from the
			// DOM below but live would still hold stale refs to them. Clear it here
			// so a keyed item added later doesn't hit a removeChild panic.
			for k := range live {
				delete(live, k)
			}
			for {
				next := start.Get("nextSibling")
				if next.IsNull() || next.IsUndefined() || next.Equal(end) {
					break
				}
				p.Call(dom.RemoveChild, next)
			}
			childScope = signal.RunScoped(func() {
				for _, n := range newNodes {
					if !isNilRaw(n) {
						p.Call("insertBefore", n, end)
					}
				}
			})
			return
		}

		// Keyed: remove stale, insert/reorder right-to-left.
		incoming := make(map[string]struct{}, len(newKeys))
		for _, k := range newKeys {
			if k != "" {
				incoming[k] = struct{}{}
			}
		}
		for k, n := range live {
			if _, ok := incoming[k]; !ok {
				p.Call(dom.RemoveChild, n)
				delete(live, k)
			}
		}

		ref := end
		for i := len(newKeys) - 1; i >= 0; i-- {
			k := newKeys[i]
			n := newNodes[i]
			if isNilRaw(n) || k == "" {
				continue
			}
			if existing, exists := live[k]; exists {
				// Swap in the new node, since it carries current state (className, text,
				// etc.) even though the key matches the old one.
				prev := ref.Get("previousSibling")
				if prev.Truthy() && prev.Equal(existing) {
					p.Call("replaceChild", n, existing)
				} else {
					p.Call(dom.RemoveChild, existing)
					p.Call("insertBefore", n, ref)
				}
				live[k] = n
				ref = n
			} else {
				p.Call("insertBefore", n, ref)
				live[k] = n
				ref = n
			}
		}
	})
}

// DynText appends a reactive text node to parent.
// More efficient than DynChild for text-only bindings, since it updates .data in place
// with no DOM node replacement or child scope overhead.
func DynText(parent Node, fn func() string) {
	p := Unwrap(parent)
	node := dom.Document.Call(dom.CreateTextNode, "")
	p.Call(dom.AppendChild, node)
	signal.Effect(func() {
		node.Set("data", fn())
	})
}

// DynProp sets a property reactively. fn re-runs when its signal deps change.
func DynProp(el Node, key string, fn func() any) {
	raw := Unwrap(el)
	signal.Effect(func() {
		setPropValue(raw, key, fn())
	})
}

// DynStyle applies a Style struct reactively.
// Clears all inline styles on each re-run so that fields set to "" in the
// new value actually disappear rather than keeping the old value.
func DynStyle(el Node, fn func() Style) {
	raw := Unwrap(el)
	signal.Effect(func() {
		s := raw.Get(dom.Style)
		s.Set("cssText", "")
		fn().apply(s)
	})
}

// Mount renders fn() into the DOM element with the given ID.
func Mount(id string, fn func() Node) {
	el := dom.Document.Call(dom.GetElementById, id)
	if el.IsNull() || el.IsUndefined() {
		panic("vane: element #" + id + " not found")
	}
	el.Call(dom.AppendChild, Unwrap(fn()))
}

// Try renders fn() safely. If fn panics, renders fallback(recovered value) instead.
func Try(fn func() Node, fallback func(err any) Node) (result Node) {
	defer func() {
		if r := recover(); r != nil {
			result = fallback(r)
		}
	}()
	return fn()
}
