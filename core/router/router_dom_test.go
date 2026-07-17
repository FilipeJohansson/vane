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
	"testing"
	"time"

	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/core/router"
	"github.com/filipejohansson/vane/core/signal"
)

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
// the JS event loop long enough for the pending jsdom task to run.
func waitForPath(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if router.Path().Get() == want {
			waitEffects(t) // let dependent effects (ActiveLink, IsActive) finish reacting
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("router.Path() did not become %q within timeout (stuck at %q)", want, router.Path().Get())
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
