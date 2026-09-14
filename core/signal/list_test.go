package signal_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/filipejohansson/vane/core/signal"
)

type listRow struct {
	ID   string
	Text string
}

func rowKey(r listRow) string { return r.ID }

func TestListSetOnEmptyList(t *testing.T) {
	l := signal.NewList(rowKey)
	if got := l.Items(); len(got) != 0 {
		t.Fatalf("Items() on a new List = %v, want empty", got)
	}

	l.Set([]listRow{{"1", "a"}, {"2", "b"}})
	got := l.Items()
	if len(got) != 2 || got[0].ID != "1" || got[1].ID != "2" {
		t.Fatalf("Items() after Set = %v, want [{1 a} {2 b}]", got)
	}
}

func TestListSetReplacesAllKeys(t *testing.T) {
	l := signal.NewList(rowKey)
	l.Set([]listRow{{"1", "a"}, {"2", "b"}})
	l.Set([]listRow{{"3", "c"}})

	got := l.Items()
	if len(got) != 1 || got[0].ID != "3" {
		t.Fatalf("Items() after second Set = %v, want [{3 c}]", got)
	}
	if _, ok := l.Get("1"); ok {
		t.Error(`Get("1") found after it was replaced away, want not found`)
	}
}

func TestListGetFoundAndNotFound(t *testing.T) {
	l := signal.NewList(rowKey)
	l.Set([]listRow{{"1", "a"}})

	got, ok := l.Get("1")
	if !ok || got.Text != "a" {
		t.Fatalf(`Get("1") = %v, %v, want {1 a}, true`, got, ok)
	}

	if _, ok := l.Get("missing"); ok {
		t.Error(`Get("missing") = found, want not found`)
	}
}

// TestListSetDuplicateKeyWarns is a regression test for a real
// inconsistency: List had no duplicate-key detection at all, unlike
// DynList's explicit warn contract for the identical bug class. Two items
// sharing a key must not silently diverge between Items() (still returns
// both, the raw data) and Get() (can only ever return one) with no warning.
func TestListSetDuplicateKeyWarns(t *testing.T) {
	var warnings []string
	orig := signal.WarnHandler
	defer func() { signal.WarnHandler = orig }()
	signal.WarnHandler = func(msg string) { warnings = append(warnings, msg) }

	l := signal.NewList(rowKey)
	l.Set([]listRow{{"1", "a"}, {"1", "b"}})

	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly 1", warnings)
	}
	if !strings.Contains(warnings[0], `"1"`) {
		t.Errorf("warning doesn't mention the duplicate key: %q", warnings[0])
	}

	if items := l.Items(); len(items) != 2 {
		t.Fatalf("Items() = %v, want both duplicates present (2 items)", items)
	}
	if got, ok := l.Get("1"); !ok || got.Text != "b" {
		t.Errorf(`Get("1") = %+v, %v, want the last one written ({1 b}), true`, got, ok)
	}
}

// TestListSetThreeWayDuplicateWarnsPerExtraOccurrence confirms 3+ items
// sharing a key still warn (not a special-cased "exactly 2" check).
func TestListSetThreeWayDuplicateWarnsPerExtraOccurrence(t *testing.T) {
	var warnings []string
	orig := signal.WarnHandler
	defer func() { signal.WarnHandler = orig }()
	signal.WarnHandler = func(msg string) { warnings = append(warnings, msg) }

	l := signal.NewList(rowKey)
	l.Set([]listRow{{"1", "a"}, {"1", "b"}, {"1", "c"}})

	if len(warnings) != 2 {
		t.Fatalf("warnings = %v, want 2 (one per extra occurrence past the first)", warnings)
	}
}

// TestListSetDuplicateKeyOnlyOnUpdateWarns confirms detection isn't a
// construction-time-only check - the first Set has no duplicates, the
// second does.
func TestListSetDuplicateKeyOnlyOnUpdateWarns(t *testing.T) {
	var warnings []string
	orig := signal.WarnHandler
	defer func() { signal.WarnHandler = orig }()
	signal.WarnHandler = func(msg string) { warnings = append(warnings, msg) }

	l := signal.NewList(rowKey)
	l.Set([]listRow{{"1", "a"}, {"2", "b"}})
	if len(warnings) != 0 {
		t.Fatalf("warnings after a clean Set = %v, want none", warnings)
	}

	l.Set([]listRow{{"1", "a"}, {"1", "b"}})
	if len(warnings) != 1 {
		t.Fatalf("warnings after the duplicate-key Set = %v, want exactly 1", warnings)
	}
}

// runCounter wraps an Effect around fn, counting how many times it runs
// (including the synchronous initial run) under a mutex - the same
// counting-effect shape TestEffectDispose already uses in this package, not
// a channel, since these tests only ever need "how many times," never
// "which value."
type runCounter struct {
	mu sync.Mutex
	n  int
}

func (c *runCounter) inc() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
}

func (c *runCounter) get() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// settle gives the scheduler a short window to run any effect it just
// queued, then returns the current count - mirrors TestEffectDispose's own
// time.Sleep-then-check convention for "confirm no further re-run
// happened."
func (c *runCounter) settle() int {
	time.Sleep(20 * time.Millisecond)
	return c.get()
}

func TestListItemsIsReactiveToStructuralChanges(t *testing.T) {
	l := signal.NewList(rowKey)
	l.Set([]listRow{{"1", "a"}})

	var c runCounter
	dispose := signal.Effect(func() { _ = l.Items(); c.inc() })
	defer dispose()

	if n := c.settle(); n != 1 {
		t.Fatalf("run count after initial effect = %d, want 1", n)
	}

	// Adding a key is a structural change: must re-run.
	l.Set([]listRow{{"1", "a"}, {"2", "b"}})
	if n := c.settle(); n != 2 {
		t.Fatalf("run count after adding a key = %d, want 2", n)
	}

	// Removing a key is a structural change: must re-run.
	l.Set([]listRow{{"1", "a"}})
	if n := c.settle(); n != 3 {
		t.Fatalf("run count after removing a key = %d, want 3", n)
	}
}

func TestListItemsIsReactiveToReorderNotJustKeySet(t *testing.T) {
	l := signal.NewList(rowKey)
	l.Set([]listRow{{"1", "a"}, {"2", "b"}})

	var c runCounter
	dispose := signal.Effect(func() { _ = l.Items(); c.inc() })
	defer dispose()

	if n := c.settle(); n != 1 {
		t.Fatalf("run count after initial effect = %d, want 1", n)
	}

	// Same key set, different order: still a structural change.
	l.Set([]listRow{{"2", "b"}, {"1", "a"}})
	if n := c.settle(); n != 2 {
		t.Fatalf("run count after reordering the same keys = %d, want 2", n)
	}
}

func TestListSetWithSameKeysAndOrderDoesNotFireStructuralSignal(t *testing.T) {
	l := signal.NewList(rowKey)
	l.Set([]listRow{{"1", "a"}, {"2", "b"}})

	var c runCounter
	dispose := signal.Effect(func() { _ = l.Items(); c.inc() })
	defer dispose()

	if n := c.settle(); n != 1 {
		t.Fatalf("run count after initial effect = %d, want 1", n)
	}

	// Same keys, same order, different field values - not a structural
	// change. Items()-reading effects must not re-run; field-level updates
	// are each item's own signal's job, not List's.
	l.Set([]listRow{{"1", "a-changed"}, {"2", "b-changed"}})
	if n := c.settle(); n != 1 {
		t.Fatalf("run count after a same-keys-same-order Set = %d, want 1 (no structural change)", n)
	}

	// The new field values ARE stored (Set always replaces outright, see
	// List's own doc comment) - just without firing the structural signal.
	if got, _ := l.Get("1"); got.Text != "a-changed" {
		t.Fatalf(`Get("1").Text = %q, want "a-changed" (Set still stores new values)`, got.Text)
	}
}

func TestListGetIsNotReactive(t *testing.T) {
	l := signal.NewList(rowKey)
	l.Set([]listRow{{"1", "a"}})

	var c runCounter
	dispose := signal.Effect(func() { _, _ = l.Get("1"); c.inc() })
	defer dispose()

	if n := c.settle(); n != 1 {
		t.Fatalf("run count after initial effect = %d, want 1", n)
	}

	// A structural change happens, but the effect only ever called Get,
	// never Items - Get must not have subscribed it to anything.
	l.Set([]listRow{{"1", "a"}, {"2", "b"}})
	if n := c.settle(); n != 1 {
		t.Fatalf("run count after a structural change = %d, want 1 (Get is non-reactive)", n)
	}
}
