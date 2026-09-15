//go:build js && wasm

package core

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"sync/atomic"
	"syscall/js"

	"github.com/filipejohansson/vane/core/signal"
	"github.com/filipejohansson/vane/internal/dom"
)

// dynListDOMOps counts DynList's own replaceChild/insertBefore/removeChild
// calls made by the keyed reconciliation path specifically (never the
// unkeyed rebuild path, which always touches every node by design - see
// DynList's own doc comment). Exposed read-only via export_test.go; tests
// use it to confirm a swap/reorder in a large keyed list touches only what
// actually moved, not the whole list.
var dynListDOMOps atomic.Int64

func init() {
	signal.EffectPanicHandler = func(r any) {
		js.Global().Get("console").Call("error", fmt.Sprintf("[vane] effect panic: %v", r))
	}
	signal.LoopWatchdogHandler = func(msg string) {
		js.Global().Get("console").Call("error", "[vane] "+msg)
	}
	signal.WarnHandler = Warn
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

// IdentityNode wraps n as a single-element slice. The `render` DynList's
// compatibility instantiation uses for callers building `[]Node` directly -
// there's nothing left to construct, the item already is the Node.
func IdentityNode(n Node) []Node { return []Node{n} }

// dynListEntry is one currently-mounted keyed item's persisted state,
// surviving unchanged across re-renders until its key's value changes or
// disappears. nodes is never empty - see renderNodesForKey.
type dynListEntry[T any] struct {
	nodes []js.Value
	value T
	scope *signal.Scope
}

// renderNodesRaw runs render(t) and unwraps the result to raw js.Values,
// dropping any nil Node - a nil is simply omitted, never inserted.
func renderNodesRaw[T any](render func(T) []Node, t T) []js.Value {
	rendered := render(t)
	out := make([]js.Value, 0, len(rendered))
	for _, n := range rendered {
		if isNilNode(n) {
			continue
		}
		out = append(out, Unwrap(n))
	}
	return out
}

// renderNodesForKey is renderNodesRaw, but a key with zero real nodes still
// gets one comment-node placeholder - the reorder pass needs a real anchor
// per key, and a comment stays invisible to :empty/children.length.
func renderNodesForKey[T any](render func(T) []Node, t T) []js.Value {
	ns := renderNodesRaw(render, t)
	if len(ns) == 0 {
		ns = []js.Value{dom.Document.Call(dom.CreateComment, "")}
	}
	return ns
}

// DynList appends a reactive, keyed list of children to parent. render is
// called once per new key and again only when a persisting key's value
// changes (reflect.DeepEqual) - render returns every top-level node one item
// produces, usually one but not always, tracked and moved as a unit per key.
//
// keyFn resolves each item's identity. Empty for every item (or keyFn nil):
// the list is unkeyed, rebuilt from scratch each change. Empty for only some
// items: those items get no persistence, rendered fresh each update and
// diffed by position, like a React list child with no key. The same
// non-empty key on two items warns and falls back to a full unkeyed rebuild
// for that update.
//
// NodePropertyKey/IdentityNode adapt a `.vane` `{items()...}` spread's
// already-built core.Node values into this same signature - see their own
// doc comments.
//
// Reordering uses a longest-increasing-subsequence pass over persisting
// keys' old-vs-new positions: only keys outside it physically move.
func DynList[T any](parent Node, items func() []T, keyFn func(T) string, render func(T) []Node) {
	p := Unwrap(parent)
	start := dom.Document.Call(dom.CreateTextNode, "")
	end := dom.Document.Call(dom.CreateTextNode, "")
	p.Call(dom.AppendChild, start)
	p.Call(dom.AppendChild, end)

	live := make(map[string]*dynListEntry[T]) // key -> persisted keyed-path state
	order := []string{}                       // current DOM order of live's real keys, for the next reorder pass

	// Owns a fully-unkeyed/duplicate-fallback render's effects; disposed and
	// rebuilt every such render, separate from live's own per-key scopes.
	var unkeyedScope *signal.Scope

	// Owns items() itself plus every keyless item's render this run - both
	// are torn down and rebuilt fresh every time, so a keyless item needs no
	// scope bookkeeping of its own.
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
				for _, n := range renderNodesRaw(render, t) {
					p.Call("insertBefore", n, end)
				}
			}
		})
	}

	signal.Effect(func() {
		if itemsScope != nil {
			itemsScope.Dispose()
		}
		var newItems []T
		newKeys := []string{}
		keylessNodes := make(map[int][]js.Value) // index -> its rendered nodes, for key == "" items only
		itemsScope = signal.RunScoped(func() {
			newItems = items()
			newKeys = make([]string, len(newItems))
			if keyFn != nil {
				for i, t := range newItems {
					newKeys[i] = keyFn(t)
					if newKeys[i] == "" {
						keylessNodes[i] = renderNodesForKey(render, t)
					}
				}
			}
		})

		keyedCount := 0
		for _, k := range newKeys {
			if k != "" {
				keyedCount++
			}
		}
		if keyedCount == 0 {
			unkeyedRebuild(newItems)
			return
		}
		if seen := make(map[string]bool, keyedCount); true {
			for _, k := range newKeys {
				if k == "" {
					continue
				}
				if seen[k] {
					Warn(fmt.Sprintf("core.DynList: key function returned %q for more than one item, falling back to unkeyed rendering for this update", k))
					unkeyedRebuild(newItems)
					return
				}
				seen[k] = true
			}
		}

		// Real keyed path: every non-empty key is distinct. A keyless ("")
		// item participates in this same pass (reordered alongside its
		// keyed siblings below) but is never looked up in or stored to
		// live - keylessNodes above already rendered it fresh this run.
		newLive := make(map[string]*dynListEntry[T], keyedCount)
		newNodesPerItem := make([][]js.Value, len(newItems))
		for i, t := range newItems {
			k := newKeys[i]
			if k == "" {
				newNodesPerItem[i] = keylessNodes[i]
				continue
			}
			if existing, ok := live[k]; ok {
				delete(live, k)
				if reflect.DeepEqual(existing.value, t) {
					// Unchanged: skip render entirely - no DOM touch, no new
					// effects, the existing subtree and its scope survive.
					newLive[k] = existing
					newNodesPerItem[i] = existing.nodes
					continue
				}
				// Changed: rebuild this key's subtree, previous scope
				// disposed, neighbors untouched. New nodes go in right
				// before the old group, which is then removed.
				existing.scope.Dispose()
				var ns []js.Value
				scope := signal.RunScoped(func() {
					ns = renderNodesForKey(render, t)
				})
				anchor := existing.nodes[0]
				for _, n := range ns {
					p.Call("insertBefore", n, anchor)
					dynListDOMOps.Add(1)
				}
				for _, old := range existing.nodes {
					p.Call(dom.RemoveChild, old)
					dynListDOMOps.Add(1)
				}
				newLive[k] = &dynListEntry[T]{nodes: ns, value: t, scope: scope}
				newNodesPerItem[i] = ns
			} else {
				// New key.
				var ns []js.Value
				scope := signal.RunScoped(func() {
					ns = renderNodesForKey(render, t)
				})
				newLive[k] = &dynListEntry[T]{nodes: ns, value: t, scope: scope}
				newNodesPerItem[i] = ns
			}
		}
		// Whatever's left in live had its key removed from the list entirely.
		for _, e := range live {
			for _, n := range e.nodes {
				p.Call(dom.RemoveChild, n)
				dynListDOMOps.Add(1)
			}
			e.scope.Dispose()
		}

		// Reorder: keys already in the longest run in the right relative
		// order (per their OLD position) never move; only keys outside it
		// get physically repositioned. A keyless item has no old position
		// (refIndices -1), so it's always repositioned too.
		oldIndexOf := make(map[string]int, len(order))
		for i, k := range order {
			oldIndexOf[k] = i
		}
		refIndices := make([]int, len(newKeys))
		for i, k := range newKeys {
			if k == "" {
				refIndices[i] = -1
				continue
			}
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
			nodes := newNodesPerItem[i]
			if stablePos >= 0 && stable[stablePos] == i {
				stablePos--
				if len(nodes) > 0 {
					ref = nodes[0]
				}
				continue
			}
			for j := len(nodes) - 1; j >= 0; j-- {
				p.Call("insertBefore", nodes[j], ref)
				dynListDOMOps.Add(1)
				ref = nodes[j]
			}
		}

		live = newLive
		order = order[:0]
		for _, k := range newKeys {
			if k != "" {
				order = append(order, k)
			}
		}
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
		lo := sort.Search(len(tails), func(mid int) bool { return refIndices[tails[mid]] >= v })
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
