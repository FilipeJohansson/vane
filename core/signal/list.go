package signal

// List is a reactive, keyed collection, a sibling to Signal/Computed. Set
// reconciles by key: a key present in both the old and new items keeps its
// position tracked; a new key is added; a missing key is dropped - but
// unlike a DynList[T]'s own reconciliation, List never compares individual
// item values (there's nothing to render here, so there's nothing to skip
// re-rendering). Set always stores exactly what it's given, for every key
// present.
//
// Items is reactive only to structural changes - which keys exist, and
// their order - not to a value changing inside an existing item's own
// signal fields. T is expected to carry its own reactive state as real
// *Signal[F] struct fields, mutated directly (t.Done.Set(...)) rather than
// through List itself; List's own structural signal only fires when the key
// set or key order actually differs from before.
//
// Get is a plain, non-reactive snapshot lookup - it never subscribes the
// calling effect to anything.
type List[T any] struct {
	keyFn   func(T) string
	items   []T
	keys    []string
	byKey   map[string]int // key -> index into items/keys, kept in sync by Set
	version *Signal[int]
	gen     int
}

// NewList creates an empty List, keyed by keyFn.
func NewList[T any](keyFn func(T) string) *List[T] {
	return &List[T]{
		keyFn:   keyFn,
		version: New(0),
	}
}

// Set replaces every item in l. The structural signal fires (an Items()-
// reading effect re-runs) only if the resulting key set or key order
// actually differs from before - calling Set with the same keys in the
// same order, even with different field values inside those items, is a
// no-op signal-wise, since those values live in their own signals, tracked
// independently.
func (l *List[T]) Set(items []T) {
	newKeys := make([]string, len(items))
	for i, t := range items {
		newKeys[i] = l.keyFn(t)
	}

	changed := len(newKeys) != len(l.keys)
	if !changed {
		for i, k := range newKeys {
			if l.keys[i] != k {
				changed = true
				break
			}
		}
	}

	l.items = items
	l.keys = newKeys
	l.byKey = make(map[string]int, len(newKeys))
	for i, k := range newKeys {
		l.byKey[k] = i
	}

	if changed {
		l.gen++
		l.version.Set(l.gen)
	}
}

// Items returns every item, in order. Reactive: an effect calling Items
// re-runs when the key set or key order changes, never when a value inside
// an existing item's own signal fields changes.
func (l *List[T]) Items() []T {
	l.version.Get() // subscribe to structural changes only
	return l.items
}

// Get returns the item currently stored under key, and whether it was
// found - a plain, non-reactive snapshot lookup (see List's own doc
// comment: structural reactivity is Items()'s job, value reactivity is each
// item's own field signals' job).
func (l *List[T]) Get(key string) (T, bool) {
	i, ok := l.byKey[key]
	if !ok {
		var zero T
		return zero, false
	}
	return l.items[i], true
}
