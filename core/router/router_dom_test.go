//go:build js && wasm

package router_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.
//
// pathSignal is a package-level singleton seeded once from the jsdom URL at
// process start (no hash → "/"). Tests that navigate must restore "/" in a
// t.Cleanup so later tests still see the expected initial state.

import (
	"net/url"
	"syscall/js"
	"testing"
	"time"

	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/core/router"
	"github.com/filipejohansson/vane/core/signal"
)

// This file's tests exercise the package-level router singleton under
// HashLocation, so they lock it explicitly here before any test can force
// ensureInit onto the router's actual default (PathLocation) instead.
// SetLocation can only succeed once per process; PathLocation's own
// singleton-level coverage lives in the separate core/router/pathlocation_test
// package for exactly that reason.
func init() {
	router.SetLocation(&router.HashLocation{})
}

func waitEffects(t *testing.T) {
	t.Helper()
	if !signal.WaitEffects(time.Second) {
		t.Fatal("effects did not settle within timeout")
	}
}

// waitForPath polls until router.Path() reflects want. router.Navigate sets
// location.hash, but jsdom dispatches the resulting "hashchange" event
// asynchronously (its own task queue), independent of Go's effect
// scheduler. signal.WaitEffects alone is not enough: it can return "idle"
// before the hashchange listener has even run, since no Go effect has been
// triggered yet. Polling with a sleep between checks lets Go yield back to
// the JS event loop long enough for the pending jsdom task to run. Also
// waits for the scroll decision every navigation schedules (see
// waitNextTick) so it never survives past the test that triggered it - left
// pending, it would run during a LATER test instead, acting on whatever
// DOM/stubs (e.g. a stubbed window.scrollTo) that other test happens to have
// set up at the time.
func waitForPath(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if router.Path().Get() == want {
			waitEffects(t) // let dependent effects (ActiveLink, IsActive) finish reacting
			waitNextTick(t)
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("router.Path() did not become %q within timeout (stuck at %q)", want, router.Path().Get())
}

// waitForQuery polls until router.Query() has key set to want. Needed for a
// query-only navigation, where router.Path() never changes value, so
// waitForPath has nothing new to poll for.
func waitForQuery(t *testing.T, key, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if router.Query().Get().Get(key) == want {
			waitEffects(t)
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("router.Query().Get(%q) did not become %q within timeout (stuck at %q)", key, want, router.Query().Get().Get(key))
}

// waitNextTick blocks until a core.NextTick callback fires. router.go's
// scroll decision (ensureInit's OnChange) is itself deferred via
// core.NextTick, so a test asserting on it - or a later test sharing this
// package's singleton router - must wait for one too: queued strictly after
// the router's own (FIFO microtask ordering), this guarantees the router's
// callback has already run by the time this returns.
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

// navigateAndRestore navigates to `to` and restores "/" (the test baseline)
// after the test completes.
func navigateAndRestore(t *testing.T, to string) {
	t.Helper()
	t.Cleanup(func() {
		router.Navigate("/")
		waitForPath(t, "/")
	})
	router.Navigate(to)
	waitForPath(t, to)
}

func TestLinkSetsHrefWithHashPrefix(t *testing.T) {
	el := router.Link(router.LinkProps{To: "/about"})
	if got := core.Unwrap(el).Get("href").String(); got != "http://localhost/#/about" {
		t.Errorf("href = %q, want it to end in %q", got, "#/about")
	}
}

func TestLinkAppliesTargetRelAndHTMLProps(t *testing.T) {
	el := router.Link(router.LinkProps{
		To:     "/docs",
		Target: "_blank",
		Rel:    "noopener",
		HTMLProps: core.HTMLProps{
			Class: "nav-link",
			ID:    "docs-link",
		},
	})
	raw := core.Unwrap(el)
	if got := raw.Get("target").String(); got != "_blank" {
		t.Errorf("target = %q, want _blank", got)
	}
	if got := raw.Get("rel").String(); got != "noopener" {
		t.Errorf("rel = %q, want noopener", got)
	}
	if got := raw.Get("className").String(); got != "nav-link" {
		t.Errorf("className = %q, want nav-link", got)
	}
	if got := raw.Get("id").String(); got != "docs-link" {
		t.Errorf("id = %q, want docs-link", got)
	}
}

func TestLinkAppendsChildren(t *testing.T) {
	el := router.Link(router.LinkProps{To: "/"}, "Home")
	if got := core.Unwrap(el).Get("textContent").String(); got != "Home" {
		t.Errorf("textContent = %q, want %q", got, "Home")
	}
}

// TestLinkAcceptsNodeChild guards the routing.md claim that Link/ActiveLink
// children accept a core.Node (e.g. an icon component), not just a string.
func TestLinkAcceptsNodeChild(t *testing.T) {
	icon := core.El("i")
	core.SetProp(icon, "className", "icon-gear")

	el := router.Link(router.LinkProps{To: "/settings"}, icon, "Settings")

	raw := core.Unwrap(el)
	if got := raw.Get("textContent").String(); got != "Settings" {
		t.Errorf("textContent = %q, want %q", got, "Settings")
	}
	if !raw.Call("querySelector", "i.icon-gear").Truthy() {
		t.Error("Node child (icon) was not appended into the link")
	}
}

func TestActiveLinkAppliesActiveClassWhenPathMatchesInitially(t *testing.T) {
	// Package-level pathSignal starts at "/" (jsdom URL has no hash).
	el := router.ActiveLink(router.ActiveLinkProps{
		LinkProps:   router.LinkProps{To: "/", HTMLProps: core.HTMLProps{Class: "nav-link"}},
		ActiveClass: "nav-link--active",
	}, "Home")

	got := core.Unwrap(el).Get("className").String()
	want := "nav-link nav-link--active"
	if got != want {
		t.Errorf("className = %q, want %q", got, want)
	}
}

func TestActiveLinkOmitsActiveClassWhenPathDoesNotMatch(t *testing.T) {
	el := router.ActiveLink(router.ActiveLinkProps{
		LinkProps:   router.LinkProps{To: "/settings", HTMLProps: core.HTMLProps{Class: "nav-link"}},
		ActiveClass: "nav-link--active",
	}, "Settings")

	got := core.Unwrap(el).Get("className").String()
	if got != "nav-link" {
		t.Errorf("className = %q, want %q (no active class, path is \"/\", not \"/settings\")", got, "nav-link")
	}
}

func TestActiveLinkReactsToNavigation(t *testing.T) {
	el := router.ActiveLink(router.ActiveLinkProps{
		LinkProps:   router.LinkProps{To: "/dashboard", HTMLProps: core.HTMLProps{Class: "nav-link"}},
		ActiveClass: "nav-link--active",
	}, "Dashboard")

	if got := core.Unwrap(el).Get("className").String(); got != "nav-link" {
		t.Fatalf("className before navigation = %q, want %q", got, "nav-link")
	}

	navigateAndRestore(t, "/dashboard")

	got := core.Unwrap(el).Get("className").String()
	want := "nav-link nav-link--active"
	if got != want {
		t.Errorf("className after navigating to /dashboard = %q, want %q", got, want)
	}
}

func TestActiveLinkPrefixMatchesSubRoutes(t *testing.T) {
	el := router.ActiveLink(router.ActiveLinkProps{
		LinkProps:   router.LinkProps{To: "/users", HTMLProps: core.HTMLProps{Class: "nav-link"}},
		ActiveClass: "nav-link--active",
	}, "Users")

	navigateAndRestore(t, "/users/42")

	got := core.Unwrap(el).Get("className").String()
	want := "nav-link nav-link--active"
	if got != want {
		t.Errorf("className = %q, want %q (/users should prefix-match /users/42)", got, want)
	}
}

func TestActiveLinkExactExcludesSubRoutes(t *testing.T) {
	el := router.ActiveLink(router.ActiveLinkProps{
		LinkProps:   router.LinkProps{To: "/dashboard", HTMLProps: core.HTMLProps{Class: "nav-link"}},
		ActiveClass: "nav-link--active",
		Exact:       true,
	}, "Overview")

	navigateAndRestore(t, "/dashboard/users")

	got := core.Unwrap(el).Get("className").String()
	if got != "nav-link" {
		t.Errorf("className = %q, want %q (Exact must not match /dashboard/users)", got, "nav-link")
	}
}

func TestIsActiveInitialValue(t *testing.T) {
	active := router.IsActive("/")
	waitEffects(t)
	if !active.Get() {
		t.Error("IsActive(\"/\") = false at initial path \"/\", want true")
	}
}

func TestIsActiveReactsToNavigation(t *testing.T) {
	active := router.IsActive("/profile")
	waitEffects(t)
	if active.Get() {
		t.Fatal("IsActive(\"/profile\") = true before navigating there")
	}

	navigateAndRestore(t, "/profile")

	if !active.Get() {
		t.Error("IsActive(\"/profile\") = false after navigating to /profile, want true")
	}
}

func TestPathReflectsNavigation(t *testing.T) {
	navigateAndRestore(t, "/reports")

	if got := router.Path().Get(); got != "/reports" {
		t.Errorf("Path() = %q, want %q", got, "/reports")
	}
}

func TestQueryReflectsCurrentURLUnderHashLocation(t *testing.T) {
	t.Cleanup(func() {
		router.Navigate("/")
		waitForPath(t, "/")
	})

	router.Navigate("/dashboard?tab=2&sort=asc")
	waitForPath(t, "/dashboard")

	got := router.Query().Get()
	if got.Get("tab") != "2" {
		t.Errorf(`Query().Get("tab") = %q, want %q`, got.Get("tab"), "2")
	}
	if got.Get("sort") != "asc" {
		t.Errorf(`Query().Get("sort") = %q, want %q`, got.Get("sort"), "asc")
	}
}

func TestQueryEmptyWhenURLHasNoQueryString(t *testing.T) {
	navigateAndRestore(t, "/reports")

	if got := router.Query().Get(); len(got) != 0 {
		t.Errorf("Query() = %v, want empty", got)
	}
}

func TestSetQueryReplacesQueryPreservingPath(t *testing.T) {
	navigateAndRestore(t, "/dashboard")

	router.SetQuery(url.Values{"tab": {"3"}})
	waitForQuery(t, "tab", "3")

	if got := router.Path().Get(); got != "/dashboard" {
		t.Errorf("Path() after SetQuery = %q, want unchanged %q", got, "/dashboard")
	}
}

// TestSetQueryPushesHistoryEntry guards that SetQuery follows Navigate's
// history semantics (push, not replace) — see TestNavigatePushesReplaceDoesNot.
func TestSetQueryPushesHistoryEntry(t *testing.T) {
	navigateAndRestore(t, "/dashboard")
	before := js.Global().Get("history").Get("length").Int()

	router.SetQuery(url.Values{"tab": {"3"}})
	waitForQuery(t, "tab", "3")

	if got := js.Global().Get("history").Get("length").Int(); got != before+1 {
		t.Errorf("history.length after SetQuery = %d, want %d (SetQuery should push a new entry, like Navigate)", got, before+1)
	}
}

func TestSetQueryWithNilClearsQueryString(t *testing.T) {
	navigateAndRestore(t, "/dashboard")
	router.SetQuery(url.Values{"tab": {"3"}})
	waitForQuery(t, "tab", "3")

	router.SetQuery(nil)
	waitForQuery(t, "tab", "")

	if got := js.Global().Get("location").Get("hash").String(); got != "#/dashboard" {
		t.Errorf("location.hash = %q, want %q (query string fully cleared)", got, "#/dashboard")
	}
}

// TestSetLocationPanicsAfterFirstUse guards the ensureInit/SetLocation
// ordering rule: SetLocation must run before the router is first used
// (Router, Navigate, Path, Link, ...), and panics otherwise. router.Path()
// below just guarantees ensureInit has already run by the time SetLocation
// is called, regardless of what other tests in this package ran first.
func TestSetLocationPanicsAfterFirstUse(t *testing.T) {
	router.Path()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("router.SetLocation did not panic when called after the router was already in use")
		}
	}()
	router.SetLocation(&router.HashLocation{})
}

// TestNavigatePushesReplaceDoesNot exercises ensureInit's wiring end to end
// through the two ways it can be notified of a path change: Navigate (via
// HashLocation setting location.hash, browser fires "hashchange") and
// Replace (via updateHistory's manual notify, since history.replaceState
// fires no event on its own). Both must land in router.Path(); only Navigate
// should grow the browser's history.
func TestNavigatePushesReplaceDoesNot(t *testing.T) {
	before := js.Global().Get("history").Get("length").Int()

	t.Cleanup(func() {
		router.Navigate("/")
		waitForPath(t, "/")
	})

	router.Navigate("/reports")
	waitForPath(t, "/reports")
	afterNavigate := js.Global().Get("history").Get("length").Int()
	if afterNavigate != before+1 {
		t.Errorf("history.length after Navigate = %d, want %d (Navigate should push a new entry)", afterNavigate, before+1)
	}

	router.Replace("/reports/summary")
	if got := router.Path().Get(); got != "/reports/summary" {
		t.Fatalf("Path() after Replace = %q, want %q", got, "/reports/summary")
	}
	afterReplace := js.Global().Get("history").Get("length").Int()
	if afterReplace != afterNavigate {
		t.Errorf("history.length after Replace = %d, want %d (Replace should not push a new entry)", afterReplace, afterNavigate)
	}
}

// TestHashLocationIgnoresNonRouteHashChange guards the Part G fix for a
// scroll-to-top regression: a same-page anchor hash change (e.g. "#section")
// also fires "hashchange", but isn't a route change at all - HashLocation's
// documented quirk maps any hash not starting with "#/" to "/", and before
// this fix the router treated that fallback as a real navigation, scrolling
// to top and fighting the browser's own scroll-to-anchor behavior (the same
// class of problem PathLocation.handleClick already avoids for its
// equivalent case, see TestPathLocationHandleClick/same-page_fragment).
// window.scrollTo is stubbed to prove it's never invoked; router.Path()
// must also stay put, since none of this should be seen as a navigation.
func TestHashLocationIgnoresNonRouteHashChange(t *testing.T) {
	navigateAndRestore(t, "/reports")

	window := js.Global().Get("window")
	original := window.Get("scrollTo")
	scrollCalls := 0
	window.Set("scrollTo", js.FuncOf(func(_ js.Value, _ []js.Value) interface{} {
		scrollCalls++
		return nil
	}))
	t.Cleanup(func() { window.Set("scrollTo", original) })

	js.Global().Get("location").Set("hash", "#section")

	// Nothing should happen here, so there's no target state to poll for
	// (unlike waitForPath) - just give the async hashchange task (see
	// waitForPath's comment) a generous chance to run before asserting.
	time.Sleep(100 * time.Millisecond)
	waitEffects(t)

	if got := router.Path().Get(); got != "/reports" {
		t.Errorf("Path() = %q after a same-page \"#section\" hash change, want it unchanged (%q)", got, "/reports")
	}
	if scrollCalls != 0 {
		t.Errorf("window.scrollTo called %d times, want 0", scrollCalls)
	}
}

func TestHashLocationSearchReadsQueryFromHash(t *testing.T) {
	loc := &router.HashLocation{}
	t.Cleanup(func() { js.Global().Get("location").Set("hash", "") })

	js.Global().Get("location").Set("hash", "#/dashboard?tab=2")
	if got := loc.Search(); got != "tab=2" {
		t.Errorf("Search() = %q, want %q", got, "tab=2")
	}
}

func TestHashLocationSearchEmptyWhenNoQuery(t *testing.T) {
	loc := &router.HashLocation{}
	t.Cleanup(func() { js.Global().Get("location").Set("hash", "") })

	js.Global().Get("location").Set("hash", "#/dashboard")
	if got := loc.Search(); got != "" {
		t.Errorf("Search() = %q, want %q", got, "")
	}
}

func TestHashLocationSearchEmptyForNonRouteHash(t *testing.T) {
	loc := &router.HashLocation{}
	t.Cleanup(func() { js.Global().Get("location").Set("hash", "") })

	js.Global().Get("location").Set("hash", "#section")
	if got := loc.Search(); got != "" {
		t.Errorf("Search() = %q, want %q (not a route hash)", got, "")
	}
}

// TestHashLocationNavigateNotifiesOnceNotTwice guards the auth-guard flake
// fix: Navigate notifies synchronously right after setting location.hash,
// but the browser still fires a real "hashchange" for that same change a
// moment later - without the dedupe in HashLocation.notify, that would
// double-invoke every effect reacting to the navigation (the scroll
// decision in particular). Exercises a standalone *HashLocation, like
// TestHashLocationIgnoresNonRouteHashChange's sibling tests do, so it
// doesn't depend on - or interfere with - the package singleton.
func TestHashLocationNavigateNotifiesOnceNotTwice(t *testing.T) {
	loc := &router.HashLocation{}
	calls := 0
	loc.OnChange(func() { calls++ })
	t.Cleanup(func() { js.Global().Get("location").Set("hash", "") })

	loc.Navigate("/guard-test-a")
	if calls != 1 {
		t.Fatalf("calls right after Navigate = %d, want 1 (synchronous notify)", calls)
	}

	// Give the native "hashchange" event that Navigate's own location.hash
	// assignment triggers a real chance to arrive and confirm it's deduped.
	time.Sleep(100 * time.Millisecond)
	if calls != 1 {
		t.Errorf("calls after the native hashchange settles = %d, want still 1 (deduped)", calls)
	}

	// A genuinely distinct subsequent change (browser back/forward, a typed
	// URL, or any location.hash write Navigate didn't make) must still
	// notify - the dedupe must not swallow real changes.
	js.Global().Get("location").Set("hash", "#/guard-test-b")
	time.Sleep(100 * time.Millisecond)
	if calls != 2 {
		t.Errorf("calls after a distinct external hash change = %d, want 2", calls)
	}
}
