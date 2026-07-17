package core

import "github.com/filipejohansson/vane/core/signal"

// Type aliases, so users annotate types without importing core/signal directly.
type Signal[T any] = signal.Signal[T]
type ReadOnlySignal[T any] = signal.ReadOnlySignal[T]
type Computed[T any] = signal.Computed[T]
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

func ComputedOf[T any](fn func(prev T) T) *signal.Computed[T] { return signal.ComputedOf(fn) }
func Untrack(fn func())                                       { signal.Untrack(fn) }
