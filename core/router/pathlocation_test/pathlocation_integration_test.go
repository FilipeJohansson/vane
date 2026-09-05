//go:build js && wasm

// Package pathlocation_test exercises the router's Router/ActiveLink/Outlet/
// Layout machinery through the real router.SetLocation singleton, configured
// with PathLocation - covering WORK.md scenarios 25 (SetLocation succeeds
// before first use), 29 (the routing engine works identically under both
// Locations) and 30 (ActiveLink/IsActive correct under PathLocation with a
// BasePath).
//
// This needs its own package (hence its own test binary/process): SetLocation
// can only succeed once per process, and core/router's own *_dom_test.go
// files already lock the singleton onto HashLocation, since some of them
// call a router function (forcing ensureInit) before any SetLocation call
// could land. Being in a separate directory/package is what guarantees this
// file's init() below is the first thing to ever touch the router package.
package pathlocation_test

import (
	"syscall/js"
	"testing"
	"time"

	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/core/router"
	"github.com/filipejohansson/vane/core/signal"
)

// basePath is deliberately non-root, so this also doubles as BasePath
// coverage (scenario 30) rather than just the zero-value case already
// covered by path_location_dom_test.go.
const basePath = "/app"

func init() {
	js.Global().Get("history").Call("replaceState", js.Null(), "", basePath)
	router.SetLocation(&router.PathLocation{BasePath: basePath})
}

func waitEffects(t *testing.T) {
	t.Helper()
	if !signal.WaitEffects(time.Second) {
		t.Fatal("effects did not settle within timeout")
	}
}

// waitForPath polls until router.Path() reflects want - PathLocation's
// Navigate/Replace notify synchronously, but poll anyway for the same reason
// core/router's own tests do: it's a resilient way to wait for dependent
// Effects (ActiveLink, IsActive) to finish reacting too.
func waitForPath(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if router.Path().Get() == want {
			waitEffects(t)
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("router.Path() did not become %q within timeout (stuck at %q)", want, router.Path().Get())
}

// waitNextTick blocks until a core.NextTick callback fires. router.go's
// scroll-to-anchor decision (ensureInit's OnChange) is itself deferred via
// core.NextTick, so a test asserting on it must wait for one too - queued
// strictly after the router's own (FIFO microtask ordering), this
// guarantees the router's callback has already run by the time this
// returns, rather than relying on some other incidental yield (e.g.
// waitForPath's time.Sleep) to have given it a chance in the meantime.
func waitNextTick(t *testing.T) {
	t.Helper()
	done := make(chan struct{})
	core.NextTick(func() { close(done) })
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("NextTick never ran fn")
	}
}

func navigateAndRestore(t *testing.T, to string) {
	t.Helper()
	t.Cleanup(func() {
		router.Navigate("/")
		waitForPath(t, "/")
	})
	router.Navigate(to)
	waitForPath(t, to)
}

// TestSetLocationWithPathLocationBeforeFirstUse is scenario 25's explicit
// check: every other test in this file already proves SetLocation "worked"
// (they all run under PathLocation), but this asserts it directly - Path()
// reads through PathLocation, and the app's own BasePath segment in the URL
// was left untouched (SetLocation configuring, not consuming, the location).
func TestSetLocationWithPathLocationBeforeFirstUse(t *testing.T) {
	if got := router.Path().Get(); got != "/" {
		t.Fatalf("Path() = %q, want %q (SetLocation(&PathLocation{BasePath: %q}) should be active)", got, "/", basePath)
	}
	if got := js.Global().Get("location").Get("pathname").String(); got != basePath {
		t.Fatalf("pathname = %q, want %q (BasePath should not have been touched)", got, basePath)
	}
}

// TestRouterAndLayoutWorkUnderPathLocation is scenario 29: matchRoute,
// matchPrefix, Outlet, Params and Layout all operate on app-relative path
// strings already stripped of BasePath by PathLocation.Path(), so the whole
// mounting engine should behave identically to how mount_dom_test.go proves
// it does under HashLocation - same assertions, different Location.
func TestRouterAndLayoutWorkUnderPathLocation(t *testing.T) {
	var gotID string
	shellMounts := 0
	el := router.Router(
		router.Route("/", func() core.Node { return core.Text("home page") }),
		router.Layout("/dashboard", func() core.Node {
			shellMounts++
			outlet := router.Outlet()
			shell := core.El("div")
			core.Unwrap(shell).Set("className", "shell")
			core.AppendChild(shell, outlet)
			return shell
		},
			router.Route("", func() core.Node { return core.Text("dashboard home") }),
			router.Route("users/:id", func() core.Node {
				gotID = router.Params().Get()["id"]
				return core.Text("dashboard user")
			}),
		),
	)
	t.Cleanup(func() {
		router.Navigate("/")
		waitForPath(t, "/")
	})

	if got := core.Unwrap(el).Get("textContent").String(); got != "home page" {
		t.Fatalf("textContent = %q, want %q", got, "home page")
	}

	router.Navigate("/dashboard")
	waitForPath(t, "/dashboard")
	if got := js.Global().Get("location").Get("pathname").String(); got != basePath+"/dashboard" {
		t.Errorf("pathname = %q, want %q (BasePath must prefix the real URL)", got, basePath+"/dashboard")
	}
	raw := core.Unwrap(el)
	if got := raw.Call("querySelector", ".shell").Get("textContent").String(); got != "dashboard home" {
		t.Errorf("shell textContent = %q, want %q", got, "dashboard home")
	}
	if shellMounts != 1 {
		t.Fatalf("shellMounts = %d, want 1", shellMounts)
	}

	router.Navigate("/dashboard/users/7")
	waitForPath(t, "/dashboard/users/7")
	if gotID != "7" {
		t.Errorf("extracted id param = %q, want %q", gotID, "7")
	}
	if shellMounts != 1 {
		t.Errorf("shellMounts after sub-navigation = %d, want 1 (shell must persist across sub-routes)", shellMounts)
	}
}

// TestActiveLinkAndIsActiveUnderPathLocationWithBasePath is scenario 30:
// ActiveLink/IsActive match against the app-relative path (unaffected by
// BasePath), while the rendered href still carries BasePath - same
// separation of concerns Href()/Path() already guarantee individually, now
// proven through the components that build on top of them.
func TestActiveLinkAndIsActiveUnderPathLocationWithBasePath(t *testing.T) {
	el := router.ActiveLink(router.ActiveLinkProps{
		LinkProps:   router.LinkProps{To: "/settings", HTMLProps: core.HTMLProps{Class: "nav-link"}},
		ActiveClass: "nav-link--active",
	}, "Settings")

	raw := core.Unwrap(el)
	wantHref := "http://localhost" + basePath + "/settings"
	if got := raw.Get("href").String(); got != wantHref {
		t.Errorf("href = %q, want %q", got, wantHref)
	}
	if got := raw.Get("className").String(); got != "nav-link" {
		t.Fatalf("className before navigation = %q, want %q", got, "nav-link")
	}

	active := router.IsActive("/settings")
	waitEffects(t)
	if active.Get() {
		t.Fatal("IsActive(\"/settings\") = true before navigating there")
	}

	navigateAndRestore(t, "/settings")

	if got := raw.Get("className").String(); got != "nav-link nav-link--active" {
		t.Errorf("className after navigating to /settings = %q, want %q", got, "nav-link nav-link--active")
	}
	if !active.Get() {
		t.Error("IsActive(\"/settings\") = false after navigating to /settings, want true")
	}
}

// TestNavigateToAnchorScrollsToElementInsteadOfTop is scenario 38: a
// navigation whose target URL carries a fragment (Href's fix in
// PathLocation preserves it as a real in-page anchor, not folded away) must
// scroll to that element instead of the page top - even though the path
// itself did change (a case that would otherwise hit the scroll-to-top
// branch first). scrollIntoView is stubbed on the specific element instance
// (not a global), so it can't leak into unrelated tests.
func TestNavigateToAnchorScrollsToElementInsteadOfTop(t *testing.T) {
	target := core.El("div")
	core.SetProp(target, "id", "target-section")
	js.Global().Get("document").Get("body").Call("appendChild", core.Unwrap(target))
	t.Cleanup(func() { js.Global().Get("document").Get("body").Call("removeChild", core.Unwrap(target)) })

	scrollIntoViewCalls := 0
	core.Unwrap(target).Set("scrollIntoView", js.FuncOf(func(_ js.Value, _ []js.Value) interface{} {
		scrollIntoViewCalls++
		return nil
	}))

	window := js.Global().Get("window")
	original := window.Get("scrollTo")
	scrollToCalls := 0
	window.Set("scrollTo", js.FuncOf(func(_ js.Value, _ []js.Value) interface{} {
		scrollToCalls++
		return nil
	}))
	t.Cleanup(func() { window.Set("scrollTo", original) })

	router.Navigate("/settings#target-section")
	t.Cleanup(func() {
		router.Navigate("/")
		waitForPath(t, "/")
	})
	waitForPath(t, "/settings")
	waitNextTick(t)

	if scrollIntoViewCalls != 1 {
		t.Errorf("target element's scrollIntoView called %d times, want 1", scrollIntoViewCalls)
	}
	if scrollToCalls != 0 {
		t.Errorf("window.scrollTo called %d times, want 0 (the anchor should take priority)", scrollToCalls)
	}
}

// TestNavigateWithoutAnchorStillScrollsToTop is scenario 39: the ordinary
// case (a plain navigation, no fragment) must keep scrolling to top - the
// scenario 38 fix must not have swallowed this by, say, always short-
// circuiting on AnchorID().
func TestNavigateWithoutAnchorStillScrollsToTop(t *testing.T) {
	window := js.Global().Get("window")
	original := window.Get("scrollTo")
	scrollToCalls := 0
	window.Set("scrollTo", js.FuncOf(func(_ js.Value, _ []js.Value) interface{} {
		scrollToCalls++
		return nil
	}))
	t.Cleanup(func() { window.Set("scrollTo", original) })

	navigateAndRestore(t, "/reports")
	waitNextTick(t)

	if scrollToCalls != 1 {
		t.Errorf("window.scrollTo called %d times, want 1 (plain navigation with no anchor should scroll to top)", scrollToCalls)
	}
}
