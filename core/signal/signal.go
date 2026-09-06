package signal

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

// EffectPanicHandler, if set, is called when an Effect panics instead of
// crashing the app. Set by the core package to log to the browser console.
var EffectPanicHandler func(recovered any)

// LoopWatchdogHandler, if set, is called when flushEffects detects a likely
// infinite update loop (see maxEffectsPerFlush) instead of silently hanging.
// Set by the core package to log to the browser console. Falls back to
// EffectPanicHandler, then to stdout, so callers only wiring up the older
// hook still get a diagnostic instead of a silent freeze.
var LoopWatchdogHandler func(message string)

//* Scope

// Scope collects the disposers registered (via RegisterDispose) while fn runs
// inside RunScoped, so Dispose can undo everything that setup created —
// effects, DOM listeners, JS callbacks — in one call. This is Vane's cleanup
// mechanism end to end: there is no virtual DOM diff or mount tracer deciding
// what to tear down, only whatever scope-aware code registered itself.
type Scope struct {
	mu       sync.Mutex
	disposes []func()
}

var (
	scopeStack []*Scope
	scopeLock  sync.Mutex
)

func pushScope(s *Scope) {
	scopeLock.Lock()
	scopeStack = append(scopeStack, s)
	scopeLock.Unlock()
}

func popScope() {
	scopeLock.Lock()
	if len(scopeStack) > 0 {
		scopeStack = scopeStack[:len(scopeStack)-1]
	}
	scopeLock.Unlock()
}

func activeScope() *Scope {
	scopeLock.Lock()
	defer scopeLock.Unlock()
	if len(scopeStack) == 0 {
		return nil
	}
	return scopeStack[len(scopeStack)-1]
}

// RunScoped runs fn inside a new Scope and returns it. Call Dispose() later to
// clean up all effects created during fn.
//
// If fn panics, RunScoped never reaches its own return, so the caller gets
// no reference to Dispose whatever the Scope collected up to the panic
// point; RunScoped disposes it itself before letting the panic continue
// propagating, so a partially-built subtree doesn't leak the effects and
// listeners it managed to create.
func RunScoped(fn func()) *Scope {
	s := &Scope{}
	pushScope(s)
	ok := false
	defer func() {
		popScope()
		if !ok {
			s.Dispose()
		}
	}()
	fn()
	ok = true
	return s
}

// Dispose runs every disposer registered in s, most-recently-registered
// first, then clears them. Safe to call more than once: the second call has
// nothing left to run.
func (s *Scope) Dispose() {
	s.mu.Lock()
	ds := s.disposes
	s.disposes = nil
	s.mu.Unlock()
	for i := len(ds) - 1; i >= 0; i-- {
		ds[i]()
	}
}

// RegisterDispose registers fn to run when the current Scope (the one
// RunScoped is running) is disposed. No-op if there is no active scope, e.g.
// called outside RunScoped or after it has already returned.
func RegisterDispose(fn func()) {
	if s := activeScope(); s != nil {
		s.mu.Lock()
		s.disposes = append(s.disposes, fn)
		s.mu.Unlock()
	}
}

//* Signal base

type computation interface {
	dependOn(*baseSignal)
}

type baseSignal struct {
	subscribers map[computation]struct{}
	mutex       sync.Mutex
}

func (s *baseSignal) track(c computation) {
	if c == nil || isNilInterface(c) {
		return
	}
	s.mutex.Lock()
	if s.subscribers == nil {
		s.subscribers = make(map[computation]struct{})
	}
	s.subscribers[c] = struct{}{}
	s.mutex.Unlock()
}

func (s *baseSignal) notify() {
	s.mutex.Lock()
	subscribers := make([]computation, 0, len(s.subscribers))
	for c := range s.subscribers {
		if c == nil || isNilInterface(c) {
			continue
		}
		subscribers = append(subscribers, c)
	}
	s.mutex.Unlock()
	for _, c := range subscribers {
		c.dependOn(s)
	}
}

//* Signal[T]

// Signal is a reactive container for a value of type T. Reading it via Get,
// while an Effect or Computed is running, subscribes that computation to
// future changes; Set stores a new value and re-runs every subscriber. There
// is no equality check between the old and new value: Set always notifies,
// even when v deep-equals the current value.
type Signal[T any] struct {
	baseSignal
	value T
}

// New creates a Signal holding initial. Prefer core.NewSignal from component
// code; this constructor is for code in core/signal itself and other code
// that can't import core.
func New[T any](initial T) *Signal[T] {
	return &Signal[T]{
		baseSignal: baseSignal{subscribers: make(map[computation]struct{})},
		value:      initial,
	}
}

// Get returns s's current value, subscribing the running Effect/Computed (if
// any) to future changes.
func (s *Signal[T]) Get() T {
	current := currentComputation()
	s.track(current)
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.value
}

// Set stores v and synchronously re-runs every subscriber that read s via
// Get. Unconditional: there is no check against the previous value, so
// setting the same value again still re-runs subscribers.
func (s *Signal[T]) Set(v T) {
	s.mutex.Lock()
	s.value = v
	s.mutex.Unlock()
	s.notify()
}

// ReadOnly returns a ReadOnlySignal backed by s, for exposing s to callers
// that should read but never write it (e.g. a package-level store returning
// its internal signal to the rest of the app).
func (s *Signal[T]) ReadOnly() *ReadOnlySignal[T] {
	return &ReadOnlySignal[T]{
		source: s,
	}
}

//* ReadOnlySignal[T]

// ReadOnlySignal is a read-only view of a Signal: same subscription behavior
// as Get, no Set. Returned by Signal.ReadOnly and NewReadOnly.
type ReadOnlySignal[T any] struct {
	source *Signal[T]
}

// NewReadOnly creates a standalone ReadOnlySignal holding initial, backed by
// a Signal only this call has a reference to (so nothing else can ever call
// Set on it). Prefer Signal.ReadOnly when exposing an existing, mutable
// Signal read-only to callers instead.
func NewReadOnly[T any](initial T) *ReadOnlySignal[T] {
	return &ReadOnlySignal[T]{
		source: New(initial),
	}
}

// Get returns s's current value, subscribing the running Effect/Computed (if
// any) to future changes — same behavior as Signal.Get.
func (s *ReadOnlySignal[T]) Get() T {
	return s.source.Get()
}

//* Effect

type effect struct {
	fn       func()
	deps     map[*baseSignal]struct{}
	runMu    sync.Mutex
	pending  atomic.Bool
	disposed atomic.Bool
}

var (
	effectStack []*effect
	effectLock  sync.Mutex
)

// liveEffects counts effects created but not yet disposed, across the whole
// process. Incremented by newEffect, decremented at the single point (in
// dispose, or flushEffects' panic recovery) where an effect's disposed flag
// actually transitions false->true, so each effect is counted exactly once
// regardless of which path disposes it.
var liveEffects atomic.Int64

// LiveEffectCount reports how many effects have been created but not yet
// disposed. A coarse memory-leak signal for tests (a number that should
// return to baseline after a mount/unmount cycle) and, like
// runtime.NumGoroutine, a metric apps can surface in a debug panel.
func LiveEffectCount() int {
	return int(liveEffects.Load())
}

// newEffect allocates an effect and accounts for it in liveEffects. Both
// Effect and ComputedOf (which wraps its recomputation in an effect too)
// construct effects through this so the counter covers every one of them.
func newEffect(fn func()) *effect {
	liveEffects.Add(1)
	return &effect{fn: fn, deps: make(map[*baseSignal]struct{})}
}

// Effect runs fn immediately, tracking all signal reads, and re-runs fn when
// any dependency changes. The returned function disposes the effect.
// The effect is also registered in the active Scope (if any) for auto-disposal.
func Effect(fn func()) func() {
	e := newEffect(fn)
	dispose := func() { e.dispose() }
	RegisterDispose(dispose)
	e.run()
	return dispose
}

func (e *effect) dispose() {
	if !e.disposed.CompareAndSwap(false, true) {
		return
	}
	liveEffects.Add(-1)
	// run() (via flushEffects, on the scheduler goroutine) also reads/writes
	// e.deps under runMu. Without this lock, a dispose() racing a scheduled
	// re-run would read/write e.deps concurrently with run().
	e.runMu.Lock()
	e.clearDeps()
	e.runMu.Unlock()
}

// clearDeps unsubscribes e from all its current dependencies. Caller must
// already hold e.runMu, either by locking it directly (dispose, called from
// outside the scheduler) or because it's already held by the caller
// (flushEffects' panic recovery, which runs inside its own e.runMu.Lock()
// further up the call stack and would deadlock re-locking it here).
func (e *effect) clearDeps() {
	for dep := range e.deps {
		dep.mutex.Lock()
		delete(dep.subscribers, e)
		dep.mutex.Unlock()
	}
	e.deps = make(map[*baseSignal]struct{})
}

func (e *effect) run() {
	if e.disposed.Load() {
		return
	}
	effectLock.Lock()
	effectStack = append(effectStack, e)
	effectLock.Unlock()
	defer func() {
		effectLock.Lock()
		effectStack = effectStack[:len(effectStack)-1]
		effectLock.Unlock()
	}()

	e.clearDeps()

	e.fn()
}

// Global effect scheduler, one drain goroutine instead of one per effect.
// In WASM (cooperative goroutines) this eliminates mutex contention from
// N goroutines racing to run after a single Set().
var (
	schedulerMu  sync.Mutex
	schedulerQ   []*effect
	schedulerRun bool
)

// maxEffectsPerFlush caps how many effect runs a single flushEffects call
// processes before it assumes the queue is a self-rescheduling infinite loop
// (e.g. a signal Set inside its own dependent effect) rather than legitimate
// fan-out, and aborts instead of hanging the goroutine forever. Ordinary
// updates, even large fan-out ones, finish in the tens to low hundreds of
// effect runs, so this leaves generous headroom above real usage.
const maxEffectsPerFlush = 10000

func enqueue(e *effect) {
	schedulerMu.Lock()
	schedulerQ = append(schedulerQ, e)
	start := !schedulerRun
	if start {
		schedulerRun = true
	}
	schedulerMu.Unlock()
	if start {
		go flushEffects()
	}
}

func flushEffects() {
	processed := 0
	for {
		schedulerMu.Lock()
		if len(schedulerQ) == 0 {
			schedulerRun = false
			schedulerMu.Unlock()
			return
		}
		processed++
		if processed > maxEffectsPerFlush {
			// Drop the rest of the queue rather than keep looping: the effects
			// still queued are almost certainly more of the same runaway cycle,
			// and this goroutine has no yield point, so continuing would hang
			// the whole (single-threaded, in WASM) runtime instead of just
			// logging and giving control back.
			for i := range schedulerQ {
				schedulerQ[i] = nil
			}
			schedulerQ = nil
			schedulerRun = false
			schedulerMu.Unlock()
			msg := fmt.Sprintf("vane: aborted after %d effect runs in a single flush, likely an infinite update loop (e.g. a signal Set called synchronously from its own dependent Effect, or from a ref callback before the element is attached to the DOM, see NextTick)", processed-1)
			switch {
			case LoopWatchdogHandler != nil:
				LoopWatchdogHandler(msg)
			case EffectPanicHandler != nil:
				EffectPanicHandler(msg)
			default:
				fmt.Println(msg)
			}
			return
		}
		e := schedulerQ[0]
		schedulerQ[0] = nil // allow GC
		schedulerQ = schedulerQ[1:]
		schedulerMu.Unlock()

		e.runMu.Lock()
		e.pending.Store(false)
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Not e.dispose() here, since that would re-lock e.runMu,
					// which this goroutine already holds (locked above and only
					// released after this func() returns), causing a deadlock.
					if e.disposed.CompareAndSwap(false, true) {
						liveEffects.Add(-1)
						e.clearDeps()
					}
					if EffectPanicHandler != nil {
						EffectPanicHandler(r)
					} else {
						fmt.Printf("vane: effect panic: %v\n", r)
					}
				}
			}()
			e.run()
		}()
		e.runMu.Unlock()
	}
}

func (e *effect) schedule() {
	if e.disposed.Load() {
		return
	}
	if e.pending.CompareAndSwap(false, true) {
		enqueue(e)
	}
}

func (e *effect) dependOn(s *baseSignal) {
	if e.disposed.Load() {
		return
	}
	e.deps[s] = struct{}{}
	s.mutex.Lock()
	s.subscribers[e] = struct{}{}
	s.mutex.Unlock()
	e.schedule()
}

func currentComputation() *effect {
	effectLock.Lock()
	defer effectLock.Unlock()
	if len(effectStack) == 0 {
		return nil
	}
	return effectStack[len(effectStack)-1]
}

// Untrack runs fn without tracking any signal reads.
// Use when component setup code should not subscribe a parent Effect.
func Untrack(fn func()) {
	effectLock.Lock()
	n := len(effectStack)
	var saved *effect
	if n > 0 {
		saved = effectStack[n-1]
		effectStack = effectStack[:n-1]
	}
	effectLock.Unlock()

	fn()

	if saved != nil {
		effectLock.Lock()
		effectStack = append(effectStack, saved)
		effectLock.Unlock()
	}
}

//* Computed[T]

// Computed is a read-only derived signal. Its value is recomputed whenever
// any of its signal deps change. Think of it as a memoized signal.
type Computed[T any] struct {
	baseSignal
	value  T
	effect *effect
	mutex  sync.Mutex
}

// ComputedOf creates a derived signal from fn. fn receives the previous value
// so you can do incremental updates. Subscribes like any signal.
func ComputedOf[T any](fn func(prev T) T) *Computed[T] {
	c := &Computed[T]{}
	c.subscribers = make(map[computation]struct{})
	c.effect = newEffect(func() {
		c.mutex.Lock()
		c.value = fn(c.value)
		c.mutex.Unlock()
		c.notify()
	})
	RegisterDispose(func() { c.effect.dispose() })
	c.effect.run()
	return c
}

// Get returns c's current (memoized) value, subscribing the running
// Effect/Computed (if any) to future changes. Never re-runs fn itself: fn
// only runs from the internal effect ComputedOf sets up, which recomputes
// and calls notify whenever one of fn's own deps changes.
func (c *Computed[T]) Get() T {
	c.track(currentComputation())
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.value
}

// WaitEffects blocks until the effect queue is drained or timeout elapses.
// Returns true if the queue drained before the timeout. For use in tests only.
func WaitEffects(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		schedulerMu.Lock()
		idle := !schedulerRun && len(schedulerQ) == 0
		schedulerMu.Unlock()
		if idle {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

func isNilInterface(i interface{}) bool {
	if i == nil {
		return true
	}
	v := reflect.ValueOf(i)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	}
	return false
}
