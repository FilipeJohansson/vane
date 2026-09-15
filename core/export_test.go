//go:build js && wasm

package core

// This file exposes package-private state for white-box testing only, the
// same export_test.go pattern used in core/signal. The _test.go suffix
// excludes it from `go build`.

// DebugLiveCallbacks reports how many js.Func callbacks created via
// bindHandler/bindWindowListener (core/events.go) are currently bound to the
// DOM. Tests use it to prove listeners don't accumulate across mount/unmount
// or re-render cycles.
func DebugLiveCallbacks() int {
	return int(liveCallbacks.Load())
}

// DebugDynListDOMOps reports how many replaceChild/insertBefore/removeChild
// calls DynList's keyed reconciliation path has made in total (process
// lifetime, not per-instance - see dynListDOMOps's own doc comment). Tests
// read it before and after a single update to assert the delta stays
// bounded by what actually changed, not the list's total size.
func DebugDynListDOMOps() int64 {
	return dynListDOMOps.Load()
}
