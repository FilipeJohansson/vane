//go:build js && wasm

package core

import (
	"fmt"
	"reflect"
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
	// tracking a single child element reference: a third-party lib may swap
	// the element for a different node, and fn() may return multi-root
	// content a single reference can't track. Each re-run removes everything
	// between start and end and inserts the new content before end.
	start := dom.Document.Call(dom.CreateComment, "vane")
	end := dom.Document.Call(dom.CreateComment, "/vane")
	p.Call(dom.AppendChild, start)
	p.Call(dom.AppendChild, end)
	var childScope *signal.Scope
	signal.RegisterDispose(func() {
		if childScope != nil {
			childScope.Dispose()
			childScope = nil
		}
	})

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

// NodePropertyKey reads a Node's own `"key"` JS property (set via `key={expr}`
// in vane syntax, or by hand via `Unwrap(n).Set("key", ...)`), returning ""
// if absent. This is the `keyFn` DynList's compatibility instantiation uses
// for callers building `[]Node` directly instead of raw item values - see
// IdentityNode and DynList's own doc comment.
func NodePropertyKey(n Node) string {
	if isNilNode(n) {
		return ""
	}
	switch k := Unwrap(n).Get("key"); k.Type() {
	case js.TypeString:
		return k.String()
	case js.TypeNumber:
		return strconv.FormatFloat(k.Float(), 'f', -1, 64)
	default:
		return ""
	}
}

// IdentityNode returns n unchanged. The `render` DynList's compatibility
// instantiation uses for callers building `[]Node` directly - there's
// nothing left to construct, the item already is the Node.
func IdentityNode(n Node) Node { return n }

// dynListEntry is one currently-mounted keyed item's persisted state, kept
// across DynList's own driving effect re-runs so an unchanged key's DOM
// node, and the effects/listeners its own render call created, survive a
// re-render untouched.
type dynListEntry[T any] struct {
	node  js.Value
	value T
	scope *signal.Scope
}

// DynList appends a reactive, keyed list of children to parent, between two
// marker nodes. Re-runs items() when its signal deps change; render is
// called once per new key and again whenever a persisting key's value
// changes (per reflect.DeepEqual) - never for an unchanged key, so that
// key's own DOM subtree and the effects/listeners its render call created
// stay untouched across a re-render.
//
// keyFn resolves each item's identity. If keyFn returns "" for every item
// (or keyFn is nil), the list is unkeyed: rebuilt from scratch on every
// change, no per-item persistence, matching a plain []Node/[]any list with
// no stable identity. If keyFn returns "" for only some items, or returns
// the same key for two different items in the same render, DynList warns
// and falls back to a full unkeyed rebuild for that update, rather than
// silently dropping or misattributing an item.
//
// For callers building already-constructed core.Node values rather than raw
// item data (e.g. a `.vane` `{items()...}` spread with no key/render sugar
// available to compile against), NodePropertyKey/IdentityNode adapt today's
// `key={expr}`-on-the-node convention into this same signature - see their
// own doc comments. That instantiation never applies the DeepEqual skip
// (comparing already-built Node values isn't a meaningful "did the data
// change" check), only the reorder behavior below.
//
// Reordering is resolved via a longest-increasing-subsequence pass over
// persisting keys' old-vs-new positions: only keys outside that subsequence
// are physically moved, so a swap or a single insertion in a large list
// touches only what actually moved, not the whole list.
func DynList[T any](parent Node, items func() []T, keyFn func(T) string, render func(T) Node) {
	p := Unwrap(parent)
	start := dom.Document.Call(dom.CreateTextNode, "")
	end := dom.Document.Call(dom.CreateTextNode, "")
	p.Call(dom.AppendChild, start)
	p.Call(dom.AppendChild, end)

	live := make(map[string]*dynListEntry[T]) // key -> persisted keyed-path state
	order := []string{}                       // current DOM order of live's keys, for the next reorder pass

	// unkeyedScope owns every effect/listener a fully-unkeyed (or
	// mixed/duplicate-fallback) render created; disposed and rebuilt on
	// every such render, mirroring the pre-generic DynList's single
	// whole-list scope. Kept separate from live's own per-key scopes, which
	// persist independently across renders instead.
	var unkeyedScope *signal.Scope

	// itemsScope owns whatever items() itself creates directly - a caller
	// building already-complete core.Node values (this DynList's
	// NodePropertyKey/IdentityNode compatibility instantiation, or any
	// hand-written caller not going through the generated render path) may
	// attach effects/listeners while constructing each item, not inside a
	// separate render call. Disposed and rebuilt around every items() call,
	// every render, regardless of which path is taken afterward - mirrors
	// the pre-generic DynList wrapping its whole `fn()` call in a scope.
	var itemsScope *signal.Scope

	signal.RegisterDispose(func() {
		if unkeyedScope != nil {
			unkeyedScope.Dispose()
		}
		if itemsScope != nil {
			itemsScope.Dispose()
		}
		for _, e := range live {
			e.scope.Dispose()
		}
	})

	removeAllChildren := func() {
		for {
			next := start.Get("nextSibling")
			if next.IsNull() || next.IsUndefined() || next.Equal(end) {
				break
			}
			p.Call(dom.RemoveChild, next)
		}
	}

	unkeyedRebuild := func(newItems []T) {
		if unkeyedScope != nil {
			unkeyedScope.Dispose()
		}
		for k, e := range live {
			e.scope.Dispose()
			delete(live, k)
		}
		order = order[:0]
		removeAllChildren()
		unkeyedScope = signal.RunScoped(func() {
			for _, t := range newItems {
				n := Unwrap(render(t))
				if isNilRaw(n) {
					n = dom.Document.Call(dom.CreateTextNode, "")
				}
				p.Call("insertBefore", n, end)
			}
		})
	}

	signal.Effect(func() {
		if itemsScope != nil {
			itemsScope.Dispose()
		}
		var newItems []T
		itemsScope = signal.RunScoped(func() {
			newItems = items()
		})

		newKeys := make([]string, len(newItems))
		keyedCount := 0
		if keyFn != nil {
			for i, t := range newItems {
				k := keyFn(t)
				newKeys[i] = k
				if k != "" {
					keyedCount++
				}
			}
		}

		switch {
		case keyedCount == 0:
			unkeyedRebuild(newItems)
			return
		case keyedCount != len(newItems):
			Warn("core.DynList: some items resolved a key and others didn't, falling back to unkeyed rendering for this update")
			unkeyedRebuild(newItems)
			return
		}
		if seen := make(map[string]bool, len(newKeys)); true {
			for _, k := range newKeys {
				if seen[k] {
					Warn(fmt.Sprintf("core.DynList: key function returned %q for more than one item, falling back to unkeyed rendering for this update", k))
					unkeyedRebuild(newItems)
					return
				}
				seen[k] = true
			}
		}

		// Real keyed path: every item has a distinct, non-empty key.
		newLive := make(map[string]*dynListEntry[T], len(newItems))
		newNodes := make([]js.Value, len(newItems))
		for i, t := range newItems {
			k := newKeys[i]
			if existing, ok := live[k]; ok {
				delete(live, k)
				if reflect.DeepEqual(existing.value, t) {
					// Unchanged: skip render entirely - no DOM touch, no new
					// effects, the existing subtree and its scope survive.
					newLive[k] = existing
					newNodes[i] = existing.node
					continue
				}
				// Changed: this key's own subtree is rebuilt, its previous
				// scope disposed - neighbors are never touched.
				existing.scope.Dispose()
				var n js.Value
				scope := signal.RunScoped(func() {
					n = Unwrap(render(t))
					if isNilRaw(n) {
						n = dom.Document.Call(dom.CreateTextNode, "")
					}
				})
				p.Call("replaceChild", n, existing.node)
				newLive[k] = &dynListEntry[T]{node: n, value: t, scope: scope}
				newNodes[i] = n
			} else {
				// New key.
				var n js.Value
				scope := signal.RunScoped(func() {
					n = Unwrap(render(t))
					if isNilRaw(n) {
						n = dom.Document.Call(dom.CreateTextNode, "")
					}
				})
				newLive[k] = &dynListEntry[T]{node: n, value: t, scope: scope}
				newNodes[i] = n
			}
		}
		// Whatever's left in live had its key removed from the list entirely.
		for _, e := range live {
			p.Call(dom.RemoveChild, e.node)
			e.scope.Dispose()
		}

		// Reorder: keys already in the longest run that's already in the
		// right relative order (per their OLD position) never move: only
		// keys outside that run get physically repositioned. A changed key
		// was already replaceChild'd in place above, so it only needs an
		// extra move here if its relative position also changed.
		oldIndexOf := make(map[string]int, len(order))
		for i, k := range order {
			oldIndexOf[k] = i
		}
		refIndices := make([]int, len(newKeys))
		for i, k := range newKeys {
			if oi, ok := oldIndexOf[k]; ok {
				refIndices[i] = oi
			} else {
				refIndices[i] = -1 // new key: never part of the stable run
			}
		}
		stable := stableIndices(refIndices)

		ref := end
		stablePos := len(stable) - 1
		for i := len(newKeys) - 1; i >= 0; i-- {
			if stablePos >= 0 && stable[stablePos] == i {
				stablePos--
				ref = newNodes[i]
				continue
			}
			p.Call("insertBefore", newNodes[i], ref)
			ref = newNodes[i]
		}

		live = newLive
		order = newKeys
	})
}

// stableIndices returns, in ascending order, the indices into refIndices
// whose values form one longest strictly increasing subsequence - the
// elements DynList's reorder pass can leave physically untouched. A value
// of -1 (a brand new key, absent from the previous render) never
// participates: it always needs inserting, so it's never part of the
// "already in order" run.
func stableIndices(refIndices []int) []int {
	n := len(refIndices)
	if n == 0 {
		return nil
	}
	tails := make([]int, 0, n)  // tails[k] = index into refIndices ending the best length-(k+1) run found so far
	prev := make([]int, n)      // prev[i] = index into refIndices of i's predecessor in its own run, or -1
	for i, v := range refIndices {
		if v < 0 {
			prev[i] = -1
			continue
		}
		lo, hi := 0, len(tails)
		for lo < hi {
			mid := (lo + hi) / 2
			if refIndices[tails[mid]] < v {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		if lo > 0 {
			prev[i] = tails[lo-1]
		} else {
			prev[i] = -1
		}
		if lo == len(tails) {
			tails = append(tails, i)
		} else {
			tails[lo] = i
		}
	}
	if len(tails) == 0 {
		return nil
	}
	result := make([]int, len(tails))
	k := tails[len(tails)-1]
	for i := len(tails) - 1; i >= 0; i-- {
		result[i] = k
		k = prev[k]
	}
	return result
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
