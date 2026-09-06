package signal

// This file exposes package-private state for white-box testing only (the
// standard library uses the same export_test.go pattern, e.g. net/http,
// time). It never ships in a production build: the _test.go suffix excludes
// it from `go build`.

// DebugSubscriberCount reports how many computations are currently
// subscribed to s. Tests use it to prove a signal's subscriber set doesn't
// grow unbounded across re-renders (see e.g. DynList churn tests).
func (s *Signal[T]) DebugSubscriberCount() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return len(s.subscribers)
}

// DebugPendingCount reports how many cleanup functions are currently
// registered on s, i.e. how much would run on the next Dispose.
func (s *Scope) DebugPendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.disposes)
}

// DebugScopeStackDepth reports the current depth of the package-level Scope
// stack. It should return to the depth it had before a RunScoped call once
// that call returns, panic or not: a mismatch means RunScoped leaked a
// Scope onto the stack.
func DebugScopeStackDepth() int {
	scopeLock.Lock()
	defer scopeLock.Unlock()
	return len(scopeStack)
}

// DebugEffectStackDepth reports the current depth of the package-level
// effect stack. It should return to the depth it had before an effect ran
// once that run returns, panic or not: a mismatch means a panicking effect
// body leaked a stale, disposed effect onto the stack, corrupting
// currentComputation() for every read that follows.
func DebugEffectStackDepth() int {
	effectLock.Lock()
	defer effectLock.Unlock()
	return len(effectStack)
}
