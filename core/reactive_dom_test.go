//go:build js && wasm

package core_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.

import (
	"fmt"
	"strconv"
	"syscall/js"
	"testing"
	"time"

	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/core/signal"
)

// jsGlobalConsole returns the global console object, for tests that need to
// intercept core.Warn's console.warn calls.
func jsGlobalConsole(t *testing.T) js.Value {
	t.Helper()
	return js.Global().Get("console")
}

// jsFuncCapture returns a js.Func that appends the first argument (as a
// string) to *calls each time it's invoked, used to spy on console.warn.
func jsFuncCapture(t *testing.T, calls *[]string) js.Func {
	t.Helper()
	f := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			*calls = append(*calls, args[0].String())
		}
		return nil
	})
	t.Cleanup(f.Release)
	return f
}

func childTexts(t *testing.T, parent core.Node) []string {
	t.Helper()
	raw := core.Unwrap(parent)
	children := raw.Get("childNodes")
	n := children.Get("length").Int()
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, children.Index(i).Get("textContent").String())
	}
	return out
}

// TestDynListDuplicateKeysWarnsAndFallsBackToUnkeyed verifies that a keyFn
// returning the same key for two different items doesn't panic or silently
// drop/overwrite one of them - it degrades to unkeyed rendering for that
// update instead, and every item still renders. Replaces the old
// TestDynListKeyLengthMismatchFallsBackToUnkeyed: that test's own scenario
// (an explicit key callback returning a different number of keys than
// nodes) can't happen anymore now that keyFn is called once per item by
// construction, always 1:1 - this is the closest real equivalent under the
// new signature, guarding the same "malformed key input can't crash or lose
// an item" property.
func TestDynListDuplicateKeysWarnsAndFallsBackToUnkeyed(t *testing.T) {
	parent := core.El("ul")

	items := func() []string { return []string{"a", "b", "c"} }
	dupeKey := func(string) string { return "same-key-for-everyone" }
	render := func(s string) core.Node { return core.Text(s) }

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("DynList panicked on duplicate keys: %v", r)
		}
	}()
	core.DynList(parent, items, dupeKey, render)

	got := childTexts(t, parent)
	want := []string{"", "a", "b", "c", ""} // start/end marker text nodes are empty
	if len(got) != len(want) {
		t.Fatalf("childTexts = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("child[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestDynListUnkeyedChurnDisposesOldItemEffects verifies that replacing an
// unkeyed DynList's entire item set disposes every effect and OnDispose
// cleanup the previous render's items registered, not just the DOM nodes.
func TestDynListUnkeyedChurnDisposesOldItemEffects(t *testing.T) {
	parent := core.El("ul")
	items := core.NewSignal([]int{1, 2, 3})

	var liveItems int // net OnDispose cleanups still pending for rendered items

	render := func(id int) core.Node {
		li := core.El("li")
		core.AppendText(li, strconv.Itoa(id))
		signal.Effect(func() {}) // stand-in for a real item's internal reactive binding
		liveItems++
		core.OnDispose(func() { liveItems-- })
		return li
	}

	core.DynList(parent, items.Get, nil, render)
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial DynList render")
	}
	if liveItems != 3 {
		t.Fatalf("liveItems = %d after initial render, want 3", liveItems)
	}
	// afterInitial counts DynList's own driving effect (which stays alive for
	// as long as the list is mounted) plus the 3 item effects just created.
	afterInitial := signal.LiveEffectCount()

	items.Set([]int{4, 5}) // entirely different set: the old 3 items must be disposed
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the DynList churn")
	}

	if liveItems != 2 {
		t.Fatalf("liveItems = %d after churn, want 2 (old items' OnDispose cleanups never ran)", liveItems)
	}
	want := afterInitial - 3 + 2 // dispose the 3 old item effects, create 2 new ones
	if got := signal.LiveEffectCount(); got != want {
		t.Fatalf("LiveEffectCount() = %d after churn, want %d (old items' effects were never disposed)", got, want)
	}
}

// TestDynListKeyedChurnDisposesReplacedItemEffects verifies that DynList's
// keyed path disposes the effects/cleanups a previous render registered when
// a key's node is rebuilt from scratch (the documented behavior: a matching
// key still gets whatever new node fn() returns on each render), not just
// when the key disappears from the list entirely.
func TestDynListKeyedChurnDisposesReplacedItemEffects(t *testing.T) {
	parent := core.El("ul")
	label := core.NewSignal("a")

	var liveItems int

	items := func() []core.Node {
		li := core.El("li")
		core.Unwrap(li).Set("key", "only-item")
		core.AppendText(li, label.Get())
		signal.Effect(func() {})
		liveItems++
		core.OnDispose(func() { liveItems-- })
		return []core.Node{li}
	}

	core.DynList(parent, items, core.NodePropertyKey, core.IdentityNode)
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial DynList render")
	}
	if liveItems != 1 {
		t.Fatalf("liveItems = %d after initial render, want 1", liveItems)
	}
	// afterInitial counts DynList's own driving effect plus the one item
	// effect just created; it shouldn't grow across same-key re-renders.
	afterInitial := signal.LiveEffectCount()

	for i := range 5 {
		label.Set(fmt.Sprintf("update-%d", i)) // same key, brand-new node/effect each time
		if !signal.WaitEffects(200 * time.Millisecond) {
			t.Fatalf("scheduler did not settle after update %d", i)
		}
	}

	if liveItems != 1 {
		t.Fatalf("liveItems = %d after 5 same-key re-renders, want 1 (each replaced render's cleanup never ran)", liveItems)
	}
	if got := signal.LiveEffectCount(); got != afterInitial {
		t.Fatalf("LiveEffectCount() = %d after 5 same-key re-renders, want %d (the previous render's effect leaked each time)", got, afterInitial)
	}
}

// TestDynListKeyedRemovalDisposesRemovedItemEffects verifies that shrinking
// a keyed DynList (removing a key entirely, as opposed to replacing a
// persisting key's content, covered by
// TestDynListKeyedChurnDisposesReplacedItemEffects) disposes that item's
// effects and cleanups too.
func TestDynListKeyedRemovalDisposesRemovedItemEffects(t *testing.T) {
	parent := core.El("ul")
	ids := core.NewSignal([]int{1, 2, 3})

	var liveItems int

	items := func() []core.Node {
		current := ids.Get()
		nodes := make([]core.Node, len(current))
		for i, id := range current {
			li := core.El("li")
			core.Unwrap(li).Set("key", strconv.Itoa(id))
			core.AppendText(li, strconv.Itoa(id))
			signal.Effect(func() {})
			liveItems++
			core.OnDispose(func() { liveItems-- })
			nodes[i] = li
		}
		return nodes
	}

	core.DynList(parent, items, core.NodePropertyKey, core.IdentityNode)
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}
	if liveItems != 3 {
		t.Fatalf("liveItems = %d after initial render, want 3", liveItems)
	}
	afterInitial := signal.LiveEffectCount()

	ids.Set([]int{1, 3}) // remove key "2", keep the other two
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after removing one item")
	}

	if liveItems != 2 {
		t.Fatalf("liveItems = %d after removing one item, want 2", liveItems)
	}
	if got := signal.LiveEffectCount(); got != afterInitial-1 {
		t.Fatalf("LiveEffectCount() = %d after removing one item, want %d (the removed item's effect was never disposed)", got, afterInitial-1)
	}
}

// TestDynListRemoveAllDisposesEverything is the literal "create hundreds of
// reactive items, remove all of them" stress scenario: it verifies that
// emptying a large unkeyed DynList disposes every item's effects, listeners,
// and OnDispose cleanups, not just their DOM nodes.
func TestDynListRemoveAllDisposesEverything(t *testing.T) {
	parent := core.El("ul")
	const n = 500
	ids := make([]int, n)
	for i := range ids {
		ids[i] = i
	}
	items := core.NewSignal(ids)

	var liveItems int

	buildItems := func() []core.Node {
		current := items.Get()
		nodes := make([]core.Node, len(current))
		for i, id := range current {
			li := core.El("li")
			core.AppendText(li, strconv.Itoa(id))
			signal.Effect(func() {})
			core.OnClick(li, func(core.MouseEvent) {})
			liveItems++
			core.OnDispose(func() { liveItems-- })
			nodes[i] = li
		}
		return nodes
	}

	core.DynList(parent, buildItems, nil, core.IdentityNode)
	if !signal.WaitEffects(500 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}
	if liveItems != n {
		t.Fatalf("liveItems = %d after initial render, want %d", liveItems, n)
	}
	afterInitial := signal.LiveEffectCount()
	callbacksAfterInitial := core.DebugLiveCallbacks()

	items.Set(nil)
	if !signal.WaitEffects(500 * time.Millisecond) {
		t.Fatal("scheduler did not settle after removing all items")
	}

	if liveItems != 0 {
		t.Fatalf("liveItems = %d after removing all %d items, want 0", liveItems, n)
	}
	if got := signal.LiveEffectCount(); got != afterInitial-n {
		t.Fatalf("LiveEffectCount() = %d after removing all items, want %d (%d item effects were never disposed)", got, afterInitial-n, n)
	}
	if got := core.DebugLiveCallbacks(); got != callbacksAfterInitial-n {
		t.Fatalf("DebugLiveCallbacks() = %d after removing all items, want %d (%d item listeners were never released)", got, callbacksAfterInitial-n, n)
	}
}

// TestDynListReRenderDoesNotAccumulateSubscribers verifies that re-rendering
// a DynList many times, with a per-item effect that reads a signal external
// to the list, doesn't grow LiveEffectCount unboundedly: each render's item
// effect must be disposed before the next one is created, or that external
// signal's subscriber set (not directly inspectable from this package) would
// otherwise grow forever.
func TestDynListReRenderDoesNotAccumulateSubscribers(t *testing.T) {
	parent := core.El("ul")
	theme := core.NewSignal("light")
	trigger := core.NewSignal(0)

	items := func() []core.Node {
		_ = trigger.Get()
		li := core.El("li")
		signal.Effect(func() {
			core.Unwrap(li).Set("data-theme", theme.Get())
		})
		return []core.Node{li}
	}

	core.DynList(parent, items, nil, core.IdentityNode)
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}
	afterInitial := signal.LiveEffectCount()

	const reRenders = 50
	for i := range reRenders {
		trigger.Set(i + 1)
		if !signal.WaitEffects(200 * time.Millisecond) {
			t.Fatalf("scheduler did not settle after re-render %d", i)
		}
	}

	if got := signal.LiveEffectCount(); got != afterInitial {
		t.Fatalf("LiveEffectCount() = %d after %d re-renders, want %d (each render's item effect was never disposed)", got, reRenders, afterInitial)
	}
}

// TestDynListMixedKeyedUnkeyedRendersAll verifies that when some nodes have
// key={...} and others don't, every node still renders. The old behavior
// silently dropped the unkeyed ones (key == "" was treated as "skip").
func TestDynListMixedKeyedUnkeyedRendersAll(t *testing.T) {
	parent := core.El("ul")

	keyed := core.El("li")
	core.Unwrap(keyed).Set("key", "k1")
	core.AppendText(keyed, "keyed")

	unkeyed := core.El("li")
	core.AppendText(unkeyed, "unkeyed")

	nodes := func() []core.Node { return []core.Node{keyed, unkeyed} }
	core.DynList(parent, nodes, core.NodePropertyKey, core.IdentityNode)

	raw := core.Unwrap(parent)
	liCount := raw.Call("querySelectorAll", "li").Get("length").Int()
	if liCount != 2 {
		t.Fatalf("rendered %d <li> elements, want 2 (unkeyed sibling must not be dropped)", liCount)
	}
}

// TestDynListSkipsRenderForUnchangedKey verifies the core claim of the
// generic DynList's keyed path: render is called once for a new key, and
// never again for that key once reflect.DeepEqual says its value hasn't
// changed across a later items() call - the one thing a naive/buggy
// implementation would most easily get wrong (re-invoking render on every
// update, the way the old DynList's own fn always did).
func TestDynListSkipsRenderForUnchangedKey(t *testing.T) {
	parent := core.El("ul")
	type row struct{ ID, Text string }
	rows := core.NewSignal([]row{{"1", "a"}, {"2", "b"}, {"3", "c"}})

	renderCounts := map[string]int{}
	render := func(r row) core.Node {
		renderCounts[r.ID]++
		return core.Text(r.Text)
	}

	core.DynList(parent, rows.Get, func(r row) string { return r.ID }, render)
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}
	for _, id := range []string{"1", "2", "3"} {
		if renderCounts[id] != 1 {
			t.Fatalf("initial renderCounts[%q] = %d, want 1", id, renderCounts[id])
		}
	}

	// Only row "2" actually changes; "1" and "3" are byte-for-byte identical
	// to their previous render.
	rows.Set([]row{{"1", "a"}, {"2", "B-changed"}, {"3", "c"}})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the update")
	}

	if renderCounts["1"] != 1 {
		t.Errorf(`renderCounts["1"] = %d after an unrelated update, want 1 (unchanged key must skip render)`, renderCounts["1"])
	}
	if renderCounts["3"] != 1 {
		t.Errorf(`renderCounts["3"] = %d after an unrelated update, want 1`, renderCounts["3"])
	}
	if renderCounts["2"] != 2 {
		t.Errorf(`renderCounts["2"] = %d after its own value changed, want 2`, renderCounts["2"])
	}
}

// TestDynListChangedKeyLeavesNeighborNodeUntouched verifies that rebuilding
// one changed key's own subtree never touches a different, unchanged key's
// DOM node - item-level, not list-level, rebuilding.
func TestDynListChangedKeyLeavesNeighborNodeUntouched(t *testing.T) {
	parent := core.El("ul")
	type row struct{ ID, Text string }
	rows := core.NewSignal([]row{{"1", "a"}, {"2", "b"}})

	core.DynList(parent, rows.Get, func(r row) string { return r.ID }, func(r row) core.Node {
		li := core.El("li")
		core.AppendText(li, r.Text)
		return li
	})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}

	raw := core.Unwrap(parent)
	neighborBefore := raw.Get("children").Index(0) // key "1"'s own <li>, never changes below

	rows.Set([]row{{"1", "a"}, {"2", "changed"}})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the update")
	}

	neighborAfter := raw.Get("children").Index(0)
	if !neighborBefore.Equal(neighborAfter) {
		t.Fatal("key \"1\"'s own DOM node was replaced when only key \"2\" changed - neighbor was touched")
	}
}

// TestDynListSwapProducesBoundedDOMOps is the concrete, falsifiable
// DOM-operation-count claim this whole fix exists to satisfy: a small,
// targeted change to a large keyed list touches only what actually moved,
// not the whole list. Uses core.DebugDynListDOMOps (export_test.go) since a
// JS-side Node.prototype monkey-patch isn't available inside
// wasm_test_exec.js's jsdom the way it is in a real browser.
func TestDynListSwapProducesBoundedDOMOps(t *testing.T) {
	parent := core.El("ul")
	const n = 200
	ids := make([]int, n)
	for i := range ids {
		ids[i] = i
	}
	items := core.NewSignal(ids)

	core.DynList(parent, items.Get, func(id int) string { return strconv.Itoa(id) }, func(id int) core.Node {
		return core.Text(strconv.Itoa(id))
	})
	if !signal.WaitEffects(500 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}
	before := core.DebugDynListDOMOps()

	swapped := append([]int(nil), ids...)
	swapped[0], swapped[n-1] = swapped[n-1], swapped[0]
	items.Set(swapped)
	if !signal.WaitEffects(500 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the swap")
	}

	delta := core.DebugDynListDOMOps() - before
	if delta == 0 {
		t.Fatal("swap produced 0 DOM ops - the swap apparently never reached the DOM")
	}
	// A 2-item swap should touch only what actually moved - a small
	// constant, nowhere near proportional to n. The bound is generous (not
	// tight to exactly 2) to avoid over-fitting to one specific LIS output
	// shape; the claim being tested is O(moved), not O(n).
	const maxExpectedOps = 10
	if delta > maxExpectedOps {
		t.Fatalf("swap of 2 out of %d items produced %d DOM ops, want <= %d (O(moved items), not O(n))",
			n, delta, maxExpectedOps)
	}
}

// TestDynListReorderHandlesHarderCasesThanASwap exercises the LIS-based
// reorder against sub-range reversal, moving an item to the opposite end,
// mid-list insertion, and a full shuffle - the class of case an off-by-one
// bug in the move logic would most easily get wrong, per real-world
// reconciler defects elsewhere. Checks final rendered order only; DOM-op
// bounding is TestDynListSwapProducesBoundedDOMOps's job.
func TestDynListReorderHandlesHarderCasesThanASwap(t *testing.T) {
	parent := core.El("ul")
	ids := core.NewSignal([]int{1, 2, 3, 4, 5, 6, 7, 8})

	core.DynList(parent, ids.Get, func(id int) string { return strconv.Itoa(id) }, func(id int) core.Node {
		return core.Text(strconv.Itoa(id))
	})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}

	cases := [][]int{
		{1, 6, 5, 4, 3, 2, 7, 8},    // reverse the sub-range [2..6]
		{8, 1, 2, 3, 4, 5, 6, 7},    // move the last item to the front
		{1, 2, 9, 3, 4, 5, 6, 7, 8}, // insert a brand new key in the middle
		{7, 3, 1, 9, 5, 2, 8, 4, 6}, // full shuffle, including the new key
	}
	for _, next := range cases {
		ids.Set(next)
		if !signal.WaitEffects(200 * time.Millisecond) {
			t.Fatalf("scheduler did not settle after Set(%v)", next)
		}
		got := childTexts(t, parent)
		got = got[1 : len(got)-1] // strip the start/end marker text nodes
		if len(got) != len(next) {
			t.Fatalf("after Set(%v): got %d children, want %d: %v", next, len(got), len(next), got)
		}
		for i, id := range next {
			want := strconv.Itoa(id)
			if got[i] != want {
				t.Errorf("after Set(%v): child[%d] = %q, want %q (full order: %v)", next, i, got[i], want, got)
			}
		}
	}
}

// TestDynListDeepEqualWithFuncFieldDoesNotPanic confirms the actual failure
// mode of reflect.DeepEqual on a T containing a function field (the
// documented uncomparable-in-the-usual-sense case): no panic, and per
// reflect.DeepEqual's own documented behavior ("func values are deeply
// equal if both are nil; otherwise they are not deeply equal"), a
// non-nil func field always reads as "changed" - render runs again every
// time rather than ever incorrectly skipping.
func TestDynListDeepEqualWithFuncFieldDoesNotPanic(t *testing.T) {
	parent := core.El("ul")
	type row struct {
		ID      string
		OnClick func()
	}
	rows := core.NewSignal([]row{{ID: "1", OnClick: func() {}}})

	var renderCount int
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("DynList panicked comparing a func field via reflect.DeepEqual: %v", r)
		}
	}()

	core.DynList(parent, rows.Get, func(r row) string { return r.ID }, func(r row) core.Node {
		renderCount++
		return core.Text(r.ID)
	})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}

	// Same key, a new (never-equal-to-the-old-one) func value each time.
	for i := 0; i < 3; i++ {
		rows.Set([]row{{ID: "1", OnClick: func() {}}})
		if !signal.WaitEffects(200 * time.Millisecond) {
			t.Fatalf("scheduler did not settle after update %d", i)
		}
	}

	if renderCount != 4 {
		t.Fatalf("renderCount = %d, want 4 (initial + 3 updates - a func field never compares equal, so every update re-renders)", renderCount)
	}
}

// TestPortalNilFnRendersEmptyInsteadOfPanicking verifies core.Portal no
// longer panics when called with a nil render function.
func TestPortalNilFnRendersEmptyInsteadOfPanicking(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("core.Portal panicked on nil fn: %v", r)
		}
	}()
	result := core.Portal("#does-not-matter", nil)
	if core.Unwrap(result).Truthy() && !core.Unwrap(result).IsUndefined() {
		// Empty() wraps js.Undefined(): Truthy() is false for it, so this
		// branch only fires if Portal returned something unexpected.
		t.Errorf("expected an empty Node, got a truthy value")
	}
}

// TestWarnDedupesPerDistinctMessage verifies core.Warn logs each distinct
// message once, but different messages both get through (not a single
// global "warned once ever" flag).
func TestWarnDedupesPerDistinctMessage(t *testing.T) {
	var calls []string
	raw := jsGlobalConsole(t)
	orig := raw.Get("warn")
	raw.Set("warn", jsFuncCapture(t, &calls))
	defer raw.Set("warn", orig)

	msgA := "wasmtest: dedup message A " + t.Name()
	msgB := "wasmtest: dedup message B " + t.Name()

	core.Warn(msgA)
	core.Warn(msgA) // duplicate, must not log again
	core.Warn(msgB) // distinct message, must log

	if len(calls) != 2 {
		t.Fatalf("console.warn called %d times, want 2 (one per distinct message): %v", len(calls), calls)
	}
}
