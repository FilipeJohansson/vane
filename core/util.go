//go:build js && wasm

package core

import (
	"fmt"
	"strconv"
	"syscall/js"
)

// isNilRaw checks a raw js.Value directly, for internal use where a value
// has already been unwrapped (e.g. anchor/marker nodes never exposed as Node).
func isNilRaw(v js.Value) bool {
	return v.IsNull() || v.IsUndefined()
}

// warned tracks which distinct messages have already been logged via Warn, so
// a bug that fires every render (e.g. inside a reactive Effect) doesn't flood
// the console, each distinct message logs once per page load. WASM is
// single-threaded (cooperative goroutine scheduling), so no lock is needed.
var warned = make(map[string]bool)

// releaseFlag is set via `-ldflags "-X github.com/filipejohansson/vane/core.releaseFlag=1"`,
// which `vane build --release` passes automatically. When set, Warn is a no-op,
// stripping developer-facing warnings from production builds.
var releaseFlag string

// Warn logs a developer-facing misuse warning to the browser console. Each
// distinct msg is logged at most once per page load, safe to call from a
// reactive Effect that reruns on every signal change without spamming the console.
// No-op in release builds (see releaseFlag).
func Warn(msg string) {
	if releaseFlag == "1" {
		return
	}
	if warned[msg] {
		return
	}
	warned[msg] = true
	js.Global().Get("console").Call("warn", "[vane] "+msg)
}

// IsRelease reports whether the running binary was built with `vane build
// --release` (or `vane build --tinygo --release`), i.e. whether releaseFlag
// was set via -ldflags -X at link time. App code can use this to gate
// dev-only checks, debug overlays, or verbose logging the same way Warn
// already gates its own output.
func IsRelease() bool {
	return releaseFlag == "1"
}

// isNilNode checks a Node. A nil interface, or one wrapping null/undefined, both count as nil.
func isNilNode(n Node) bool {
	if n == nil {
		return true
	}
	return isNilRaw(Unwrap(n))
}

// NextTick schedules fn to run on the next microtask, after the current
// synchronous call stack returns, before the browser does anything else
// (paints, timers, etc). Backed by queueMicrotask.
//
// vane builds each subtree fully in memory, bottom-up, then attaches it to
// the document in one shot (Mount, Portal, and the {expr}/{slice...}
// reactive-child helpers all follow this shape, building first and inserting
// after. A ref callback (ref={func(el core.Node){...}}) fires during that
// build step, before the element is connected to the document, so anything
// needing a live connection (moving focus chief among them) has to wait
// a tick. NextTick is a plain deferral with no lifecycle awareness. It
// doesn't know about components, doesn't cancel if the surrounding scope is
// disposed before it fires, and fires every time it's called, not once per
// mount. It isn't a substitute for core.OnDispose-based cleanup.
func NextTick(fn func()) {
	var cb js.Func
	cb = js.FuncOf(func(this js.Value, args []js.Value) any {
		fn()
		cb.Release()
		return nil
	})
	js.Global().Call("queueMicrotask", cb)
}

func stringify(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return strconv.Itoa(val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case js.Value:
		if val.Type() == js.TypeString {
			return val.String()
		}
		return fmt.Sprint(val)
	default:
		return fmt.Sprint(val)
	}
}
