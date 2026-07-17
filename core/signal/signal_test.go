package signal_test

import (
	"sync"
	"testing"
	"time"

	"github.com/filipejohansson/vane/core/signal"
)

// waitEffect waits up to 100ms for a value to appear on ch, failing the test on timeout.
func waitEffect[T any](t *testing.T, ch <-chan T, label string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("%s: effect did not re-run within 100ms", label)
		var zero T
		return zero
	}
}

//* Signal

func TestSignalGetSet(t *testing.T) {
	s := signal.New(42)
	if got := s.Get(); got != 42 {
		t.Fatalf("want 42, got %d", got)
	}
	s.Set(100)
	if got := s.Get(); got != 100 {
		t.Fatalf("want 100 after Set, got %d", got)
	}
}

func TestSignalStringZeroValue(t *testing.T) {
	s := signal.New("")
	if got := s.Get(); got != "" {
		t.Fatalf("want empty string, got %q", got)
	}
}

func TestSignalBoolToggle(t *testing.T) {
	s := signal.New(false)
	s.Set(true)
	if !s.Get() {
		t.Fatal("want true after Set(true)")
	}
}

func TestSignalStruct(t *testing.T) {
	type point struct{ x, y int }
	s := signal.New(point{1, 2})
	s.Set(point{3, 4})
	got := s.Get()
	if got.x != 3 || got.y != 4 {
		t.Fatalf("want {3,4}, got %v", got)
	}
}

func TestSignalSlice(t *testing.T) {
	s := signal.New([]int{1, 2})
	s.Set([]int{3, 4, 5})
	if got := s.Get(); len(got) != 3 {
		t.Fatalf("want len 3, got %d", len(got))
	}
}

//* ReadOnly

func TestReadOnlyInitialValue(t *testing.T) {
	s := signal.New("hello")
	ro := s.ReadOnly()
	if got := ro.Get(); got != "hello" {
		t.Fatalf("want hello, got %s", got)
	}
}

func TestReadOnlyReflectsSourceUpdates(t *testing.T) {
	s := signal.New("a")
	ro := s.ReadOnly()

	s.Set("b")
	if got := ro.Get(); got != "b" {
		t.Fatalf("want b after source Set, got %s", got)
	}

	s.Set("c")
	if got := ro.Get(); got != "c" {
		t.Fatalf("want c after second Set, got %s", got)
	}
}

func TestReadOnlyMultipleFromSameSource(t *testing.T) {
	s := signal.New(1)
	ro1 := s.ReadOnly()
	ro2 := s.ReadOnly()

	s.Set(99)
	if ro1.Get() != 99 || ro2.Get() != 99 {
		t.Fatalf("want 99 on both read-only views, got %d, %d", ro1.Get(), ro2.Get())
	}
}

//* Effect

func TestEffectRunsImmediately(t *testing.T) {
	s := signal.New(7)
	var got int
	dispose := signal.Effect(func() {
		got = s.Get()
	})
	defer dispose()
	if got != 7 {
		t.Fatalf("want 7 on immediate run, got %d", got)
	}
}

func TestEffectRerunsOnSignalChange(t *testing.T) {
	s := signal.New("a")
	ch := make(chan string, 10)

	dispose := signal.Effect(func() { ch <- s.Get() })
	defer dispose()

	<-ch // consume initial synchronous run

	s.Set("b")
	got := waitEffect(t, ch, "after Set(b)")
	if got != "b" {
		t.Fatalf("want b, got %s", got)
	}
}

func TestEffectTracksMultipleSignals(t *testing.T) {
	a := signal.New(1)
	b := signal.New(2)
	ch := make(chan int, 10)

	dispose := signal.Effect(func() { ch <- a.Get() + b.Get() })
	defer dispose()

	<-ch // initial: 3

	a.Set(10)
	if got := waitEffect(t, ch, "after a.Set(10)"); got != 12 {
		t.Fatalf("want 12, got %d", got)
	}

	b.Set(20)
	if got := waitEffect(t, ch, "after b.Set(20)"); got != 30 {
		t.Fatalf("want 30, got %d", got)
	}
}

func TestEffectDispose(t *testing.T) {
	s := signal.New(0)
	var mu sync.Mutex
	var count int

	dispose := signal.Effect(func() {
		_ = s.Get()
		mu.Lock()
		count++
		mu.Unlock()
	})

	mu.Lock()
	c := count
	mu.Unlock()
	if c != 1 {
		t.Fatalf("want 1 initial run, got %d", c)
	}

	dispose()

	s.Set(1)
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	c = count
	mu.Unlock()
	if c != 1 {
		t.Fatalf("effect ran after dispose; want count 1, got %d", c)
	}
}

func TestEffectDoesNotRerunWithoutRead(t *testing.T) {
	s := signal.New(0)
	var count int

	dispose := signal.Effect(func() {
		// Does NOT read s, should not subscribe
		count++
	})
	defer dispose()

	if count != 1 {
		t.Fatalf("want 1 initial run, got %d", count)
	}

	s.Set(99)
	time.Sleep(20 * time.Millisecond)

	if count != 1 {
		t.Fatalf("effect should not re-run when it doesn't read the signal, got %d", count)
	}
}

func TestEffectResubscribesAfterRerun(t *testing.T) {
	s := signal.New(0)
	ch := make(chan int, 10)

	dispose := signal.Effect(func() { ch <- s.Get() })
	defer dispose()

	<-ch // initial

	s.Set(1)
	waitEffect(t, ch, "first update")

	s.Set(2)
	if got := waitEffect(t, ch, "second update"); got != 2 {
		t.Fatalf("want 2 on second update, got %d", got)
	}
}

//* Computed

func TestComputedInitialValue(t *testing.T) {
	s := signal.New(5)
	c := signal.ComputedOf(func(_ int) int { return s.Get() * 2 })
	if got := c.Get(); got != 10 {
		t.Fatalf("want 10, got %d", got)
	}
}

func TestComputedUpdatesWhenSourceChanges(t *testing.T) {
	s := signal.New(5)
	c := signal.ComputedOf(func(_ int) int { return s.Get() * 2 })

	s.Set(7)
	time.Sleep(20 * time.Millisecond)
	if got := c.Get(); got != 14 {
		t.Fatalf("want 14 after s.Set(7), got %d", got)
	}
}

func TestComputedChain(t *testing.T) {
	s := signal.New(2)
	doubled := signal.ComputedOf(func(_ int) int { return s.Get() * 2 })
	quadrupled := signal.ComputedOf(func(_ int) int { return doubled.Get() * 2 })

	if got := quadrupled.Get(); got != 8 {
		t.Fatalf("want 8 initially, got %d", got)
	}

	s.Set(3)
	time.Sleep(30 * time.Millisecond) // two scheduler hops
	if got := quadrupled.Get(); got != 12 {
		t.Fatalf("want 12 after s.Set(3), got %d", got)
	}
}

func TestComputedPreviousValue(t *testing.T) {
	s := signal.New(1)
	// fn receives the previous computed value
	c := signal.ComputedOf(func(prev int) int {
		return prev + s.Get()
	})

	// initial: prev=0, s=1 → 0+1=1
	if got := c.Get(); got != 1 {
		t.Fatalf("want 1 initially, got %d", got)
	}

	s.Set(5)
	time.Sleep(20 * time.Millisecond)
	// prev=1, s=5 → 1+5=6
	if got := c.Get(); got != 6 {
		t.Fatalf("want 6 after s.Set(5), got %d", got)
	}
}

func TestComputedSubscribedByEffect(t *testing.T) {
	s := signal.New(3)
	doubled := signal.ComputedOf(func(_ int) int { return s.Get() * 2 })
	ch := make(chan int, 10)

	dispose := signal.Effect(func() { ch <- doubled.Get() })
	defer dispose()

	<-ch // initial: 6

	s.Set(10)
	time.Sleep(30 * time.Millisecond)
	if got := waitEffect(t, ch, "after s.Set(10)"); got != 20 {
		t.Fatalf("want 20, got %d", got)
	}
}

//* Scope

func TestScopeDisposesEffects(t *testing.T) {
	s := signal.New(0)
	var mu sync.Mutex
	var count int

	scope := signal.RunScoped(func() {
		signal.Effect(func() {
			_ = s.Get()
			mu.Lock()
			count++
			mu.Unlock()
		})
	})

	// initial run happened synchronously
	mu.Lock()
	before := count
	mu.Unlock()
	if before < 1 {
		t.Fatalf("want at least 1 run before dispose, got %d", before)
	}

	scope.Dispose()

	s.Set(1)
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	after := count
	mu.Unlock()
	if after != before {
		t.Fatalf("effect ran after scope.Dispose(); count was %d, now %d", before, after)
	}
}

func TestScopeDisposeIsIdempotent(t *testing.T) {
	s := signal.New(0)

	scope := signal.RunScoped(func() {
		signal.Effect(func() { _ = s.Get() })
	})

	scope.Dispose()
	scope.Dispose() // must not panic
}

func TestNestedScopes(t *testing.T) {
	s := signal.New(0)
	var mu sync.Mutex
	var outer, inner int

	outerScope := signal.RunScoped(func() {
		signal.Effect(func() {
			_ = s.Get()
			mu.Lock()
			outer++
			mu.Unlock()
		})
		innerScope := signal.RunScoped(func() {
			signal.Effect(func() {
				_ = s.Get()
				mu.Lock()
				inner++
				mu.Unlock()
			})
		})
		innerScope.Dispose() // dispose inner immediately
	})
	defer outerScope.Dispose()

	time.Sleep(5 * time.Millisecond)
	mu.Lock()
	outerBefore := outer
	mu.Unlock()

	s.Set(1)
	time.Sleep(20 * time.Millisecond)

	// outer effect should have re-run, inner should not (disposed)
	mu.Lock()
	outerAfter, innerAfter := outer, inner
	mu.Unlock()
	if outerAfter <= outerBefore {
		t.Fatalf("outer effect should have re-run, outer=%d outerBefore=%d", outerAfter, outerBefore)
	}
	if innerAfter != 1 {
		t.Fatalf("inner effect should have run exactly once (then disposed), got %d", innerAfter)
	}
}

//* Untrack

func TestUntrackPreventsSubscription(t *testing.T) {
	s := signal.New("x")
	var count int

	dispose := signal.Effect(func() {
		signal.Untrack(func() {
			_ = s.Get() // read without tracking
		})
		count++
	})
	defer dispose()

	if count != 1 {
		t.Fatalf("want 1 initial run, got %d", count)
	}

	s.Set("y")
	time.Sleep(20 * time.Millisecond)

	if count != 1 {
		t.Fatalf("effect should not re-run after Untrack read, got count=%d", count)
	}
}

func TestUntrackedReadInsideEffect(t *testing.T) {
	tracked := signal.New(1)
	untracked := signal.New(100)
	ch := make(chan int, 10)

	dispose := signal.Effect(func() {
		var u int
		signal.Untrack(func() { u = untracked.Get() })
		ch <- tracked.Get() + u
	})
	defer dispose()

	<-ch // initial: 1 + 100 = 101

	// Changing untracked should NOT re-run the effect
	untracked.Set(999)
	time.Sleep(20 * time.Millisecond)
	if len(ch) > 0 {
		t.Fatalf("changing untracked signal should not trigger effect")
	}

	// Changing tracked SHOULD re-run, using current untracked value (999)
	tracked.Set(2)
	got := waitEffect(t, ch, "after tracked.Set(2)")
	if got != 2+999 {
		t.Fatalf("want %d, got %d", 2+999, got)
	}
}

// TestLoopWatchdogAbortsRunawayEffect proves that an effect which
// synchronously Sets its own dependency (self-rescheduling forever) gets its
// flush aborted by the watchdog instead of hanging flushEffects' goroutine in
// an unbounded loop.
func TestLoopWatchdogAbortsRunawayEffect(t *testing.T) {
	orig := signal.LoopWatchdogHandler
	defer func() { signal.LoopWatchdogHandler = orig }()

	fired := make(chan string, 1)
	signal.LoopWatchdogHandler = func(msg string) {
		select {
		case fired <- msg:
		default:
		}
	}

	s := signal.New(0)
	dispose := signal.Effect(func() {
		s.Set(s.Get() + 1) // reschedules itself on every run
	})
	defer dispose()

	select {
	case msg := <-fired:
		if msg == "" {
			t.Fatal("want a non-empty diagnostic message")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not fire within 2s; flushEffects is likely stuck in an unbounded loop")
	}

	if !signal.WaitEffects(200 * time.Millisecond) {
		t.Fatal("scheduler did not settle after the watchdog aborted the runaway flush")
	}
}
