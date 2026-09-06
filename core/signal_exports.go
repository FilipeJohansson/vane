package core

import "github.com/filipejohansson/vane/core/signal"

// Type aliases, so users annotate types without importing core/signal directly.

// Signal is a reactive container for a value: Get() reads it (and subscribes
// the enclosing Effect/Computed to future changes), Set() writes it and
// re-runs every subscriber. Create one with core.NewSignal.
type Signal[T any] = signal.Signal[T]

// ReadOnlySignal is a read-only view of a Signal, with Get() but no Set().
// Returned by (*Signal).ReadOnly, for exposing internal state that callers
// should read but never write.
type ReadOnlySignal[T any] = signal.ReadOnlySignal[T]

// Computed is a read-only signal whose value is derived from other signals
// and recomputed whenever they change. Create one with core.ComputedOf.
type Computed[T any] = signal.Computed[T]

// Scope collects the cleanup registered (via core.OnDispose) while a subtree
// mounts, so it can all be undone in one call when that subtree unmounts.
// core.Mount and the router create one per mounted component tree/route.
type Scope = signal.Scope

// NewSignal creates a new reactive signal.
func NewSignal[T any](initial T) *signal.Signal[T] { return signal.New(initial) }

// Effect runs fn reactively; re-runs when signal deps change.
// The cleanup returned by fn runs before each re-run and on unmount.
func Effect(fn func() func()) {
	var cleanup func()
	signal.Effect(func() {
		if cleanup != nil {
			cleanup()
			cleanup = nil
		}
		cleanup = fn()
	})
	signal.RegisterDispose(func() {
		if cleanup != nil {
			cleanup()
			cleanup = nil
		}
	})
}

// OnDispose registers fn to be called when the current component scope is disposed (unmount).
// Use for non-reactive cleanup: timers, goroutines, subscriptions created during mount.
// No-op if called outside a tracked scope (e.g. root component mounted via core.Mount).
func OnDispose(fn func()) { signal.RegisterDispose(fn) }

// ComputedOf creates a derived signal from fn, recomputed whenever a signal
// fn reads changes. fn receives the previous value, so incremental updates
// (e.g. appending to a running total instead of recomputing it from scratch)
// don't need the full source data. Subscribes like any signal when its Get()
// is called inside an Effect.
func ComputedOf[T any](fn func(prev T) T) *signal.Computed[T] { return signal.ComputedOf(fn) }

// Untrack runs fn without subscribing the enclosing Effect to any signal it
// reads. Use during component setup to read a signal's current value without
// creating a reactive dependency on it — e.g. reading a prop once to seed
// local state, rather than re-running the surrounding Effect on every change
// to that prop.
func Untrack(fn func()) { signal.Untrack(fn) }
