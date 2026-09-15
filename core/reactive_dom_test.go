//go:build js && wasm

package core_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. Run with
// GOOS=js GOARCH=wasm go test -exec="node tools/wasmtest/wasm_test_exec.js" ./core/...

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
	render := func(s string) []core.Node { return []core.Node{core.Text(s)} }

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

// TestDynListPropertyKeyedDuplicateWarnsAndFallsBack verifies that the
// property-keyed compatibility instantiation (core.NodePropertyKey,
// core.IdentityNode - what {items()...}'s single-return spread compiles to)
// gets the same duplicate-key detection as any other DynList instantiation:
// the duplicate check lives inside the generic DynList itself, applying
// uniformly regardless of T, not something the compatibility path opts out
// of. Two nodes sharing the same key={} value must both still render (via
// the same warn-and-fall-back-to-unkeyed path), not have the second
// silently overwrite the first.
func TestDynListPropertyKeyedDuplicateWarnsAndFallsBack(t *testing.T) {
	parent := core.El("ul")

	rowA := core.El("li")
	core.Unwrap(rowA).Set("key", "a")
	core.AppendText(rowA, "First")

	rowB := core.El("li")
	core.Unwrap(rowB).Set("key", "a") // same key as rowA
	core.AppendText(rowB, "Second")

	items := func() []core.Node { return []core.Node{rowA, rowB} }
	core.DynList(parent, items, core.NodePropertyKey, core.IdentityNode)

	raw := core.Unwrap(parent)
	liCount := raw.Call("querySelectorAll", "li").Get("length").Int()
	if liCount != 2 {
		t.Fatalf("rendered %d <li> elements, want 2 (duplicate key must not drop the second one)", liCount)
	}
}

// TestDynListUnkeyedChurnDisposesOldItemEffects verifies that replacing an
// unkeyed DynList's entire item set disposes every effect and OnDispose
// cleanup the previous render's items registered, not just the DOM nodes.
func TestDynListUnkeyedChurnDisposesOldItemEffects(t *testing.T) {
	parent := core.El("ul")
	items := core.NewSignal([]int{1, 2, 3})

	var liveItems int // net OnDispose cleanups still pending for rendered items

	render := func(id int) []core.Node {
		li := core.El("li")
		core.AppendText(li, strconv.Itoa(id))
		signal.Effect(func() {}) // stand-in for a real item's internal reactive binding
		liveItems++
		core.OnDispose(func() { liveItems-- })
		return []core.Node{li}
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

// TestDynListMixedKeylessKeyedAcrossThreeRenders confirms the per-item
// degradation holds across multiple renders, not just the first: a
// persistently keyless item mixed into an otherwise fully-keyed
// list must never make the properly-keyed rows lose their identity/
// skip-render behavior, across 3 consecutive updates that only ever touch
// the keyless item's own value.
func TestDynListMixedKeylessKeyedAcrossThreeRenders(t *testing.T) {
	parent := core.El("ul")
	type row struct{ ID, Text string }
	rows := core.NewSignal([]row{{"1", "a"}, {"", "x1"}, {"2", "b"}})

	keyedRenderCounts := map[string]int{}
	keylessRenderCount := 0
	render := func(r row) []core.Node {
		if r.ID == "" {
			keylessRenderCount++
			return []core.Node{core.Text(r.Text)}
		}
		keyedRenderCounts[r.ID]++
		return []core.Node{core.Text(r.ID + ":" + r.Text)}
	}
	core.DynList(parent, rows.Get, func(r row) string { return r.ID }, render)
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}
	if keyedRenderCounts["1"] != 1 || keyedRenderCounts["2"] != 1 {
		t.Fatalf("initial keyedRenderCounts = %v, want 1 each", keyedRenderCounts)
	}
	if keylessRenderCount != 1 {
		t.Fatalf("initial keylessRenderCount = %d, want 1", keylessRenderCount)
	}

	for i, text := range []string{"x2", "x3", "x4"} {
		rows.Set([]row{{"1", "a"}, {"", text}, {"2", "b"}})
		if !signal.WaitEffects(200 * time.Millisecond) {
			t.Fatalf("scheduler did not settle after update %d", i+1)
		}
		if keyedRenderCounts["1"] != 1 || keyedRenderCounts["2"] != 1 {
			t.Fatalf("after update %d: keyedRenderCounts = %v, want still 1 each (unchanged keyed rows must never re-render)", i+1, keyedRenderCounts)
		}
		if want := i + 2; keylessRenderCount != want {
			t.Fatalf("after update %d: keylessRenderCount = %d, want %d (keyless never persists, always re-renders)", i+1, keylessRenderCount, want)
		}
	}

	got := childTexts(t, parent)
	got = got[1 : len(got)-1]
	want := []string{"1:a", "x4", "2:b"}
	if len(got) != len(want) {
		t.Fatalf("final childTexts = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("final childTexts = %v, want %v", got, want)
		}
	}
}

// TestDynListKeylessItemAtStartMiddleEnd confirms a keyless item renders
// correctly regardless of where it sits in the list.
func TestDynListKeylessItemAtStartMiddleEnd(t *testing.T) {
	type row struct{ ID, Text string }
	cases := []struct {
		name string
		rows []row
		want []string
	}{
		{"start", []row{{"", "k"}, {"1", "a"}, {"2", "b"}}, []string{"k", "1:a", "2:b"}},
		{"middle", []row{{"1", "a"}, {"", "k"}, {"2", "b"}}, []string{"1:a", "k", "2:b"}},
		{"end", []row{{"1", "a"}, {"2", "b"}, {"", "k"}}, []string{"1:a", "2:b", "k"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parent := core.El("ul")
			rowsSig := core.NewSignal(tc.rows)
			core.DynList(parent, rowsSig.Get, func(r row) string { return r.ID }, func(r row) []core.Node {
				if r.ID == "" {
					return []core.Node{core.Text(r.Text)}
				}
				return []core.Node{core.Text(r.ID + ":" + r.Text)}
			})
			if !signal.WaitEffects(200 * time.Millisecond) {
				t.Fatal("scheduler did not settle after the initial render")
			}
			got := childTexts(t, parent)
			got = got[1 : len(got)-1]
			if len(got) != len(tc.want) {
				t.Fatalf("childTexts = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("childTexts = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// TestDynListTwoKeylessItemsSimultaneously confirms two different keyless
// items in the same list both render correctly and don't get confused for
// each other (both have the same "identity" - empty key - so nothing must
// rely on key uniqueness among them), while real keyed rows around them keep
// their own skip-render behavior.
func TestDynListTwoKeylessItemsSimultaneously(t *testing.T) {
	parent := core.El("ul")
	type row struct{ ID, Text string }
	rows := core.NewSignal([]row{{"1", "a"}, {"", "k1"}, {"", "k2"}, {"2", "b"}})

	keyedRenderCounts := map[string]int{}
	render := func(r row) []core.Node {
		if r.ID == "" {
			return []core.Node{core.Text(r.Text)}
		}
		keyedRenderCounts[r.ID]++
		return []core.Node{core.Text(r.ID + ":" + r.Text)}
	}
	core.DynList(parent, rows.Get, func(r row) string { return r.ID }, render)
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}

	assertOrder := func(want []string) {
		t.Helper()
		got := childTexts(t, parent)
		got = got[1 : len(got)-1]
		if len(got) != len(want) {
			t.Fatalf("childTexts = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("childTexts = %v, want %v", got, want)
			}
		}
	}
	assertOrder([]string{"1:a", "k1", "k2", "2:b"})

	rows.Set([]row{{"1", "a"}, {"", "k1-changed"}, {"", "k2-changed"}, {"2", "b"}})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the update")
	}
	if keyedRenderCounts["1"] != 1 || keyedRenderCounts["2"] != 1 {
		t.Fatalf("keyedRenderCounts = %v, want still 1 each - unaffected by the keyless items' own updates", keyedRenderCounts)
	}
	assertOrder([]string{"1:a", "k1-changed", "k2-changed", "2:b"})
}

// TestDynListUnkeyedNilRenderHasZeroDOMFootprint confirms a nil-rendering
// item in an unkeyed list leaves no trace at all - no placeholder node of
// any kind, not even a comment - at every position: start, middle, and end
// of the list, not just "somewhere in there."
func TestDynListUnkeyedNilRenderHasZeroDOMFootprint(t *testing.T) {
	parent := core.El("ul")
	type row struct {
		ID   string
		Skip bool
	}
	rows := core.NewSignal([]row{
		{"1", true},  // nil at the start
		{"2", false},
		{"3", true},  // nil in the middle
		{"4", false},
		{"5", true},  // nil at the end
	})

	core.DynList(parent, rows.Get, func(row) string { return "" }, func(r row) []core.Node {
		if r.Skip {
			return nil
		}
		li := core.El("li")
		core.AppendText(li, r.ID)
		return []core.Node{li}
	})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}

	raw := core.Unwrap(parent)
	if n := raw.Get("children").Get("length").Int(); n != 2 {
		t.Fatalf("got %d elements, want 2 (only the non-nil items)", n)
	}
	if got := childTexts(t, parent); len(got) != 2+2 { // 2 real items + 2 start/end markers
		t.Fatalf("childNodes = %v, want exactly 2 real items plus the 2 list markers, no nil placeholders", got)
	}
}

// TestDynListKeyedNilRenderUsesCommentPlaceholder confirms a keyed item
// whose render returns no real nodes gets a comment-node placeholder - a
// real, stable anchor for the reorder pass - but is invisible to the DOM
// APIs the original finding named: :empty/children.length/querySelectorAll.
func TestDynListKeyedNilRenderUsesCommentPlaceholder(t *testing.T) {
	parent := core.El("ul")
	type row struct {
		ID   string
		Skip bool
	}
	rows := core.NewSignal([]row{{"1", true}})

	core.DynList(parent, rows.Get, func(r row) string { return r.ID }, func(r row) []core.Node {
		if r.Skip {
			return nil
		}
		li := core.El("li")
		core.AppendText(li, r.ID)
		return []core.Node{li}
	})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}

	raw := core.Unwrap(parent)
	if n := raw.Get("children").Get("length").Int(); n != 0 {
		t.Fatalf("children.length = %d, want 0 (a comment node is not an Element)", n)
	}
	if n := raw.Call("querySelectorAll", "li").Get("length").Int(); n != 0 {
		t.Fatalf("querySelectorAll(\"li\") found %d, want 0", n)
	}

	// The parent itself is genuinely non-empty (start/end markers plus the
	// placeholder are all real childNodes), but nothing an :empty-style
	// element check or children.length would ever see.
	childNodeCount := raw.Get("childNodes").Get("length").Int()
	if childNodeCount == 0 {
		t.Fatal("childNodes is empty - the placeholder comment node itself is missing")
	}
}

// TestDynListKeyedNilThenNonNilRenderTransitions confirms both directions of
// a keyed item's render toggling between nil and real nodes - non-nil
// becoming nil, and nil becoming non-nil - end with the DOM correctly
// reflecting the new state, not stuck on stale content or a stray element.
func TestDynListKeyedNilThenNonNilRenderTransitions(t *testing.T) {
	parent := core.El("ul")
	type row struct {
		ID   string
		Skip bool
	}
	rows := core.NewSignal([]row{{"1", false}})

	render := func(r row) []core.Node {
		if r.Skip {
			return nil
		}
		li := core.El("li")
		core.AppendText(li, r.ID)
		return []core.Node{li}
	}
	core.DynList(parent, rows.Get, func(r row) string { return r.ID }, render)
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}
	raw := core.Unwrap(parent)
	if n := raw.Call("querySelectorAll", "li").Get("length").Int(); n != 1 {
		t.Fatalf("initial: querySelectorAll(\"li\") = %d, want 1", n)
	}

	// Non-nil -> nil.
	rows.Set([]row{{"1", true}})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after becoming nil")
	}
	if n := raw.Call("querySelectorAll", "li").Get("length").Int(); n != 0 {
		t.Fatalf("after becoming nil: querySelectorAll(\"li\") = %d, want 0", n)
	}

	// nil -> non-nil.
	rows.Set([]row{{"1", false}})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after becoming non-nil again")
	}
	if n := raw.Call("querySelectorAll", "li").Get("length").Int(); n != 1 {
		t.Fatalf("after becoming non-nil again: querySelectorAll(\"li\") = %d, want 1", n)
	}
	if got := raw.Call("querySelectorAll", "li").Index(0).Get("textContent").String(); got != "1" {
		t.Errorf("revived item's text = %q, want \"1\"", got)
	}
}

// TestDynListReorderMovesCommentBackedKeyAsStableAnchor confirms a
// comment-node-backed (nil-rendering) key survives a reorder as a real,
// stable anchor, and the surrounding real items still end up in the right
// order - the reorder pass doesn't need every key to have a real element.
func TestDynListReorderMovesCommentBackedKeyAsStableAnchor(t *testing.T) {
	parent := core.El("ul")
	type row struct {
		ID   string
		Skip bool
	}
	rows := core.NewSignal([]row{{"1", false}, {"2", true}, {"3", false}})

	render := func(r row) []core.Node {
		if r.Skip {
			return nil
		}
		return []core.Node{core.Text(r.ID)}
	}
	core.DynList(parent, rows.Get, func(r row) string { return r.ID }, render)
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}

	rows.Set([]row{{"3", false}, {"2", true}, {"1", false}})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the reorder")
	}

	got := childTexts(t, parent)
	got = got[1 : len(got)-1] // strip start/end markers
	// Key "2" (the comment placeholder) contributes an empty textContent but
	// still occupies its own childNodes slot, in the middle, between the two
	// real items in their new order - the comment moved as a real anchor,
	// not just "somewhere, doesn't matter where."
	want := []string{"3", "", "1"}
	if len(got) != len(want) {
		t.Fatalf("childTexts after reorder = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("childTexts after reorder = %v, want %v", got, want)
		}
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
	render := func(r row) []core.Node {
		renderCounts[r.ID]++
		return []core.Node{core.Text(r.Text)}
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

	core.DynList(parent, rows.Get, func(r row) string { return r.ID }, func(r row) []core.Node {
		li := core.El("li")
		core.AppendText(li, r.Text)
		return []core.Node{li}
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

	core.DynList(parent, items.Get, func(id int) string { return strconv.Itoa(id) }, func(id int) []core.Node {
		return []core.Node{core.Text(strconv.Itoa(id))}
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

	core.DynList(parent, ids.Get, func(id int) string { return strconv.Itoa(id) }, func(id int) []core.Node {
		return []core.Node{core.Text(strconv.Itoa(id))}
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

// TestDynListTwoNodesPerKey_InitialMountOrder confirms a keyed list whose
// render produces 2 top-level nodes per item mounts both, in the right
// relative order, for every key - the one-key-to-many-nodes shape this
// whole fix exists to support.
func TestDynListTwoNodesPerKey_InitialMountOrder(t *testing.T) {
	parent := core.El("ul")
	type row struct{ ID, Text string }
	rows := core.NewSignal([]row{{"1", "a"}, {"2", "b"}})

	core.DynList(parent, rows.Get, func(r row) string { return r.ID }, func(r row) []core.Node {
		return []core.Node{core.Text(r.ID + "-a"), core.Text(r.ID + "-b")}
	})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}

	got := childTexts(t, parent)
	want := []string{"", "1-a", "1-b", "2-a", "2-b", ""} // start/end markers are blank
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestDynListTwoNodesPerKey_UpdateReplacesOnlyThatKeysNodes confirms that
// updating one key's value in a 2-node-per-key list rebuilds only that key's
// 2 nodes - a sibling key's own 2 DOM nodes must keep their exact identity,
// not just their text content, mirroring
// TestDynListChangedKeyLeavesNeighborNodeUntouched's single-node version.
func TestDynListTwoNodesPerKey_UpdateReplacesOnlyThatKeysNodes(t *testing.T) {
	parent := core.El("ul")
	type row struct{ ID, Text string }
	rows := core.NewSignal([]row{{"1", "a"}, {"2", "b"}})

	core.DynList(parent, rows.Get, func(r row) string { return r.ID }, func(r row) []core.Node {
		liA := core.El("li")
		core.AppendText(liA, r.ID+"-a-"+r.Text)
		liB := core.El("li")
		core.AppendText(liB, r.ID+"-b-"+r.Text)
		return []core.Node{liA, liB}
	})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}

	raw := core.Unwrap(parent)
	children := raw.Get("children")
	if n := children.Get("length").Int(); n != 4 {
		t.Fatalf("got %d <li> elements, want 4 (2 keys x 2 nodes each)", n)
	}
	neighbor0 := children.Index(0) // key "1"'s first node
	neighbor1 := children.Index(1) // key "1"'s second node

	rows.Set([]row{{"1", "a"}, {"2", "changed"}})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the update")
	}

	children = raw.Get("children")
	if n := children.Get("length").Int(); n != 4 {
		t.Fatalf("got %d <li> elements after update, want 4", n)
	}
	if !neighbor0.Equal(children.Index(0)) || !neighbor1.Equal(children.Index(1)) {
		t.Fatal("key \"1\"'s own 2 DOM nodes were replaced when only key \"2\" changed")
	}
	if got := children.Index(2).Get("textContent").String(); got != "2-a-changed" {
		t.Errorf("key \"2\"'s first node text = %q, want \"2-a-changed\"", got)
	}
	if got := children.Index(3).Get("textContent").String(); got != "2-b-changed" {
		t.Errorf("key \"2\"'s second node text = %q, want \"2-b-changed\"", got)
	}
}

// TestDynListTwoNodesPerKey_ReorderMovesNodePairsTogether confirms a reorder
// moves each key's node-pair as one unit, never splitting the pair apart
// or interleaving it with another key's nodes.
func TestDynListTwoNodesPerKey_ReorderMovesNodePairsTogether(t *testing.T) {
	parent := core.El("ul")
	ids := core.NewSignal([]int{1, 2, 3})

	render := func(id int) []core.Node {
		return []core.Node{core.Text(strconv.Itoa(id) + "a"), core.Text(strconv.Itoa(id) + "b")}
	}
	core.DynList(parent, ids.Get, func(id int) string { return strconv.Itoa(id) }, render)
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}

	ids.Set([]int{3, 1, 2})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the reorder")
	}

	got := childTexts(t, parent)
	got = got[1 : len(got)-1] // strip start/end markers
	want := []string{"3a", "3b", "1a", "1b", "2a", "2b"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v (a key's node pair was split apart or interleaved)", got, want)
		}
	}
}

// TestDynListTwoNodesPerKey_RemovalRemovesBothNodes confirms removing an
// item from a 2-node-per-key list removes both of its DOM nodes, not just
// one, leaving the surviving keys' nodes intact and in order.
func TestDynListTwoNodesPerKey_RemovalRemovesBothNodes(t *testing.T) {
	parent := core.El("ul")
	ids := core.NewSignal([]int{1, 2, 3})

	render := func(id int) []core.Node {
		return []core.Node{core.Text(strconv.Itoa(id) + "a"), core.Text(strconv.Itoa(id) + "b")}
	}
	core.DynList(parent, ids.Get, func(id int) string { return strconv.Itoa(id) }, render)
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}

	ids.Set([]int{1, 3})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after removing key \"2\"")
	}

	got := childTexts(t, parent)
	got = got[1 : len(got)-1]
	want := []string{"1a", "1b", "3a", "3b"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v (removed key's nodes must both be gone)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestDynListKeyNodeCountChangesBetweenRenders covers the edge case a
// one-key-to-many-nodes render introduces: a single key whose render
// produces a different number of nodes across renders (a
// conditionally-rendered second element), in both directions.
func TestDynListKeyNodeCountChangesBetweenRenders(t *testing.T) {
	parent := core.El("ul")
	type row struct {
		ID     string
		Expand bool
	}
	rows := core.NewSignal([]row{{"1", false}})

	core.DynList(parent, rows.Get, func(r row) string { return r.ID }, func(r row) []core.Node {
		nodes := []core.Node{core.Text(r.ID + "-a")}
		if r.Expand {
			nodes = append(nodes, core.Text(r.ID+"-b"))
		}
		return nodes
	})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the initial render")
	}
	got := childTexts(t, parent)
	if want := []string{"", "1-a", ""}; !equalStrings(got, want) {
		t.Fatalf("1 node case: got %v, want %v", got, want)
	}

	rows.Set([]row{{"1", true}})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after growing to 2 nodes")
	}
	got = childTexts(t, parent)
	if want := []string{"", "1-a", "1-b", ""}; !equalStrings(got, want) {
		t.Fatalf("grown to 2 nodes: got %v, want %v", got, want)
	}

	rows.Set([]row{{"1", false}})
	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after shrinking back to 1 node")
	}
	got = childTexts(t, parent)
	if want := []string{"", "1-a", ""}; !equalStrings(got, want) {
		t.Fatalf("shrunk back to 1 node: got %v, want %v (no orphaned second node)", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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

	core.DynList(parent, rows.Get, func(r row) string { return r.ID }, func(r row) []core.Node {
		renderCount++
		return []core.Node{core.Text(r.ID)}
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
