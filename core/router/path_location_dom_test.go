//go:build js && wasm

package router_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.
//
// Unlike router_dom_test.go, these exercise PathLocation directly as a
// struct instead of through the package-level router singleton
// (router.Navigate, router.Path, ...): SetLocation can only be called once
// per process, before the router's first use, and router_dom_test.go's
// tests have already locked that singleton onto HashLocation by the time
// any test in this file runs. Location is designed to be usable standalone
// (see core/router/location.go), so testing PathLocation's methods
// directly on their own instance sidesteps that constraint entirely.

import (
	"syscall/js"
	"testing"

	"github.com/filipejohansson/vane/core/router"
)

// resetPathname restores location.pathname to "/" (jsdom's baseline URL is
// http://localhost/) once a test finishes, so later tests aren't affected
// by leftover navigation state.
func resetPathname(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		js.Global().Get("history").Call("replaceState", js.Null(), "", "/")
	})
}

// --- Path() / Href(): pure reads/builds, safe on a fresh instance per test ---

func TestPathLocationPathReadsPathname(t *testing.T) {
	resetPathname(t)
	js.Global().Get("history").Call("pushState", js.Null(), "", "/docs/concepts")

	loc := &router.PathLocation{}
	if got := loc.Path(); got != "/docs/concepts" {
		t.Errorf("Path() = %q, want %q", got, "/docs/concepts")
	}
}

func TestPathLocationPathZeroValueBasePathDoesNotEatLeadingSlash(t *testing.T) {
	resetPathname(t)
	js.Global().Get("history").Call("pushState", js.Null(), "", "/docs")

	loc := &router.PathLocation{}
	if got := loc.Path(); got != "/docs" {
		t.Errorf("Path() = %q, want %q (BasePath zero value must not eat the leading slash)", got, "/docs")
	}
}

func TestPathLocationPathStripsBasePath(t *testing.T) {
	resetPathname(t)
	js.Global().Get("history").Call("pushState", js.Null(), "", "/vane/docs/concepts")

	loc := &router.PathLocation{BasePath: "/vane"}
	if got := loc.Path(); got != "/docs/concepts" {
		t.Errorf("Path() = %q, want %q", got, "/docs/concepts")
	}
}

func TestPathLocationPathOutsideBasePathFallsBackToRoot(t *testing.T) {
	resetPathname(t)
	js.Global().Get("history").Call("pushState", js.Null(), "", "/vanessa/page")

	loc := &router.PathLocation{BasePath: "/vane"}
	if got := loc.Path(); got != "/" {
		t.Errorf("Path() = %q, want %q (pathname outside BasePath must fall back to root, and /vane must not prefix-match /vanessa)", got, "/")
	}
}

func TestPathLocationPathIgnoresQueryAndHash(t *testing.T) {
	resetPathname(t)
	js.Global().Get("history").Call("pushState", js.Null(), "", "/docs?foo=bar#examples")

	loc := &router.PathLocation{}
	if got := loc.Path(); got != "/docs" {
		t.Errorf("Path() = %q, want %q (query/hash must not leak into the app path)", got, "/docs")
	}
}

func TestPathLocationHref(t *testing.T) {
	loc := &router.PathLocation{}
	if got := loc.Href("/docs"); got != "/docs" {
		t.Errorf("Href(%q) = %q, want %q", "/docs", got, "/docs")
	}

	based := &router.PathLocation{BasePath: "/vane"}
	if got := based.Href("/docs"); got != "/vane/docs" {
		t.Errorf("Href(%q) with BasePath /vane = %q, want %q", "/docs", got, "/vane/docs")
	}
}

// --- Navigate() / Replace(): pushState/replaceState mechanics. Safe on a
// fresh instance with no OnChange registered - both tolerate a nil
// callback, so these don't need the shared, OnChange-registered instance
// below. ---

func TestPathLocationNavigatePushesHistoryEntry(t *testing.T) {
	resetPathname(t)
	before := js.Global().Get("history").Get("length").Int()

	loc := &router.PathLocation{}
	loc.Navigate("/docs/signals")

	if got := js.Global().Get("location").Get("pathname").String(); got != "/docs/signals" {
		t.Errorf("pathname after Navigate = %q, want %q", got, "/docs/signals")
	}
	if got := js.Global().Get("history").Get("length").Int(); got != before+1 {
		t.Errorf("history.length = %d, want %d (Navigate should push a new entry)", got, before+1)
	}
}

func TestPathLocationReplaceDoesNotPushHistoryEntry(t *testing.T) {
	resetPathname(t)
	js.Global().Get("history").Call("pushState", js.Null(), "", "/docs")
	before := js.Global().Get("history").Get("length").Int()

	loc := &router.PathLocation{}
	loc.Replace("/docs/signals")

	if got := js.Global().Get("location").Get("pathname").String(); got != "/docs/signals" {
		t.Errorf("pathname after Replace = %q, want %q", got, "/docs/signals")
	}
	if got := js.Global().Get("history").Get("length").Int(); got != before {
		t.Errorf("history.length = %d, want %d (Replace must not push a new entry)", got, before)
	}
}

// --- OnChange (Navigate/Replace/popstate/click all notify through it): one
// instance shared across the rest of this file, registered exactly once.
// OnChange wires a real, permanent document/window listener with no way to
// unregister it, so a fresh instance per test would leak an extra listener
// each time - by the click tests, a single click would fire once per
// accumulated instance (each independently calling preventDefault +
// pushState) instead of once. ---

var (
	notifyLoc = &router.PathLocation{}
	notify    func()
)

func init() {
	notifyLoc.OnChange(func() {
		if notify != nil {
			notify()
		}
	})
}

// useNotify wires notify to count calls for the duration of t, returning
// a pointer to the running count. Clears notify on cleanup so it doesn't
// leak into unrelated tests.
func useNotify(t *testing.T) *int {
	t.Helper()
	count := 0
	notify = func() { count++ }
	t.Cleanup(func() { notify = nil })
	return &count
}

func TestPathLocationNavigateInvokesOnChange(t *testing.T) {
	resetPathname(t)
	count := useNotify(t)

	notifyLoc.Navigate("/docs")

	if *count != 1 {
		t.Errorf("OnChange called %d times after Navigate, want 1", *count)
	}
}

func TestPathLocationReplaceInvokesOnChange(t *testing.T) {
	resetPathname(t)
	count := useNotify(t)

	notifyLoc.Replace("/docs")

	if *count != 1 {
		t.Errorf("OnChange called %d times after Replace, want 1", *count)
	}
}

func TestPathLocationOnChangeFiresOnPopState(t *testing.T) {
	resetPathname(t)
	count := useNotify(t)

	event := js.Global().Get("Event").New("popstate", map[string]any{"bubbles": true})
	js.Global().Get("window").Call("dispatchEvent", event)

	if *count != 1 {
		t.Errorf("OnChange called %d times on popstate, want 1", *count)
	}
}

// TestPathLocationHandleClick covers the click-interception checklist from
// WORK.md's Part D, plus scenario 21 (a same-page fragment link must not be
// intercepted, so the browser's native anchor scroll still works). All
// subtests share notifyLoc/notify (see above), so there's exactly one
// click listener attached for the whole test.
//
// Every subtest below that expects the click to NOT be intercepted leaves
// event.preventDefault() uncalled by design (that's the whole point), which
// means jsdom then tries to actually follow the link and logs an async
// "Not implemented: navigation" error, harmless console noise from jsdom
// itself, not a test failure (jsdom has no real navigation to perform
// here), and not something the test can suppress: it fires from a
// setTimeout queued during dispatch, after this function's assertions have
// already run.
func TestPathLocationHandleClick(t *testing.T) {
	resetPathname(t)
	js.Global().Get("history").Call("replaceState", js.Null(), "", "/")
	count := useNotify(t)

	body := js.Global().Get("document").Get("body")
	newAnchor := func(t *testing.T, href string) js.Value {
		t.Helper()
		a := js.Global().Get("document").Call("createElement", "a")
		a.Set("href", href)
		body.Call("appendChild", a)
		t.Cleanup(func() { body.Call("removeChild", a) })
		return a
	}
	click := func(a js.Value, opts map[string]any) {
		merged := map[string]any{"bubbles": true, "cancelable": true, "button": 0}
		for k, v := range opts {
			merged[k] = v
		}
		event := js.Global().Get("MouseEvent").New("click", merged)
		a.Call("dispatchEvent", event)
	}
	assertNotIntercepted := func(t *testing.T, before string) {
		t.Helper()
		if *count != 0 {
			t.Error("click should not have been intercepted, but OnChange fired")
		}
		if got := js.Global().Get("location").Get("pathname").String(); got != before {
			t.Errorf("pathname changed to %q, want unchanged (%q)", got, before)
		}
	}

	t.Run("plain left click navigates via pushState", func(t *testing.T) {
		*count = 0
		before := js.Global().Get("history").Get("length").Int()
		a := newAnchor(t, "/docs")

		click(a, nil)

		if *count != 1 {
			t.Errorf("OnChange called %d times, want 1", *count)
		}
		if got := js.Global().Get("location").Get("pathname").String(); got != "/docs" {
			t.Errorf("pathname = %q, want %q", got, "/docs")
		}
		if got := js.Global().Get("history").Get("length").Int(); got != before+1 {
			t.Errorf("history.length = %d, want %d", got, before+1)
		}
	})

	t.Run("ctrl+click is not intercepted", func(t *testing.T) {
		*count = 0
		before := js.Global().Get("location").Get("pathname").String()
		a := newAnchor(t, "/other-ctrl")

		click(a, map[string]any{"ctrlKey": true})

		assertNotIntercepted(t, before)
	})

	t.Run("middle click is not intercepted", func(t *testing.T) {
		*count = 0
		before := js.Global().Get("location").Get("pathname").String()
		a := newAnchor(t, "/other-middle")

		click(a, map[string]any{"button": 1})

		assertNotIntercepted(t, before)
	})

	t.Run("target=_blank is not intercepted", func(t *testing.T) {
		*count = 0
		before := js.Global().Get("location").Get("pathname").String()
		a := newAnchor(t, "/other-blank")
		a.Set("target", "_blank")

		click(a, nil)

		assertNotIntercepted(t, before)
	})

	t.Run("download link is not intercepted", func(t *testing.T) {
		*count = 0
		before := js.Global().Get("location").Get("pathname").String()
		a := newAnchor(t, "/file.zip")
		a.Call("setAttribute", "download", "")

		click(a, nil)

		assertNotIntercepted(t, before)
	})

	t.Run("cross-origin link is not intercepted", func(t *testing.T) {
		*count = 0
		before := js.Global().Get("location").Get("pathname").String()
		a := newAnchor(t, "https://example.com/elsewhere")

		click(a, nil)

		assertNotIntercepted(t, before)
	})

	t.Run("a click already defaultPrevented by app code is not also handled by the router", func(t *testing.T) {
		*count = 0
		before := js.Global().Get("location").Get("pathname").String()
		a := newAnchor(t, "/other-prevented")
		a.Call("addEventListener", "click", js.FuncOf(func(_ js.Value, args []js.Value) interface{} {
			args[0].Call("preventDefault")
			return nil
		}))

		click(a, nil)

		assertNotIntercepted(t, before)
	})

	// Scenario 21: a same-page fragment link (only the hash differs from
	// the current URL) must be left to the browser, so the native
	// scroll-to-anchor behavior still works - the real-world regression
	// this guards was href="#section" links on vane-page's navbar going
	// dead once PathLocation started intercepting every qualifying click.
	t.Run("same-page fragment click is not intercepted", func(t *testing.T) {
		js.Global().Get("history").Call("replaceState", js.Null(), "", "/docs/concepts")
		*count = 0
		a := newAnchor(t, "#section")

		click(a, nil)

		assertNotIntercepted(t, "/docs/concepts")
	})
}
