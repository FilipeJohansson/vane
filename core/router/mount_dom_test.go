//go:build js && wasm

package router_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.
//
// Covers Router/Route/Layout/mountEntries, the route-matching and mounting
// engine. matchPrefix/matchRoute/splitPath/joinPath are unexported pure
// string functions with no direct test seam from this external test
// package; their behavior is exercised indirectly through Router+Route
// below, same as any other caller of the package.
//
// See router_dom_test.go's navigateAndRestore/waitForPath for why navigation
// assertions poll instead of a single signal.WaitEffects call.

import (
	"testing"

	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/core/router"
	"github.com/filipejohansson/vane/core/signal"
)

func TestRouterMountsMatchingRouteAtInitialPath(t *testing.T) {
	// Package-level path starts at "/" (see router_dom_test.go's file comment).
	el := router.Router(
		router.Route("/", func() core.Node { return core.Text("home page") }),
		router.Route("/about", func() core.Node { return core.Text("about page") }),
	)

	if got := core.Unwrap(el).Get("textContent").String(); got != "home page" {
		t.Errorf("textContent = %q, want %q", got, "home page")
	}
}

func TestRouterSwitchesContentOnNavigation(t *testing.T) {
	el := router.Router(
		router.Route("/", func() core.Node { return core.Text("home page") }),
		router.Route("/about", func() core.Node { return core.Text("about page") }),
	)

	navigateAndRestore(t, "/about")

	if got := core.Unwrap(el).Get("textContent").String(); got != "about page" {
		t.Errorf("textContent after navigating to /about = %q, want %q", got, "about page")
	}
}

func TestRouterFallsBackTo404WhenNoRouteMatches(t *testing.T) {
	el := router.Router(
		router.Route("/", func() core.Node { return core.Text("home page") }),
	)

	navigateAndRestore(t, "/does-not-exist")

	if got := core.Unwrap(el).Get("textContent").String(); got == "home page" || got == "" {
		t.Errorf("textContent = %q, want the 404 fallback content, not home page or empty", got)
	}
}

func TestRouterWildcardCatchAllMatchesAnyPath(t *testing.T) {
	el := router.Router(
		router.Route("/", func() core.Node { return core.Text("home page") }),
		router.Route("*", func() core.Node { return core.Text("caught by wildcard") }),
	)

	navigateAndRestore(t, "/anything/goes/here")

	if got := core.Unwrap(el).Get("textContent").String(); got != "caught by wildcard" {
		t.Errorf("textContent = %q, want %q", got, "caught by wildcard")
	}
}

func TestRouterExtractsURLParams(t *testing.T) {
	var gotID string
	el := router.Router(
		router.Route("/", func() core.Node { return core.Text("home page") }),
		router.Route("/users/:id", func() core.Node {
			gotID = router.Params().Get()["id"]
			return core.Text("user page")
		}),
	)
	_ = el

	navigateAndRestore(t, "/users/42")

	if gotID != "42" {
		t.Errorf("extracted id param = %q, want %q", gotID, "42")
	}
}

func TestRouterSamePatternUpdatesParamsWithoutRemounting(t *testing.T) {
	mountCount := 0
	el := router.Router(
		router.Route("/", func() core.Node { return core.Text("home page") }),
		router.Route("/users/:id", func() core.Node {
			mountCount++
			return core.El("div") // stable node; params read reactively via router.Params() inside, not shown here
		}),
	)

	navigateAndRestore(t, "/users/1")
	if mountCount != 1 {
		t.Fatalf("mountCount after first navigation = %d, want 1", mountCount)
	}

	// Navigating to a different :id under the SAME pattern must update the
	// params signal in place, not tear down and remount the component.
	router.Navigate("/users/2")
	waitForPath(t, "/users/2")

	if mountCount != 1 {
		t.Errorf("mountCount after navigating /users/1 -> /users/2 = %d, want 1 (same pattern should not remount)", mountCount)
	}
	if got := core.Unwrap(el).Get("children").Get("length").Int(); got != 1 {
		t.Errorf("container has %d children, want 1 (no duplicate mount)", got)
	}
}

func TestLayoutRendersShellWithOutletPopulated(t *testing.T) {
	el := router.Router(
		router.Layout("/dashboard", func() core.Node {
			outlet := router.Outlet()
			shell := core.El("div")
			core.Unwrap(shell).Set("className", "shell")
			core.AppendChild(shell, outlet)
			return shell
		},
			router.Route("", func() core.Node { return core.Text("dashboard home") }),
			router.Route("users", func() core.Node { return core.Text("dashboard users") }),
		),
	)

	navigateAndRestore(t, "/dashboard")

	raw := core.Unwrap(el)
	if got := raw.Call("querySelector", ".shell").Get("textContent").String(); got != "dashboard home" {
		t.Errorf("shell textContent = %q, want %q", got, "dashboard home")
	}
}

func TestLayoutSubNavigationOnlyChangesOutlet(t *testing.T) {
	shellMounts := 0
	el := router.Router(
		router.Layout("/dashboard", func() core.Node {
			shellMounts++
			outlet := router.Outlet()
			shell := core.El("div")
			core.Unwrap(shell).Set("className", "shell")
			core.AppendChild(shell, outlet)
			return shell
		},
			router.Route("", func() core.Node { return core.Text("dashboard home") }),
			router.Route("users", func() core.Node { return core.Text("dashboard users") }),
		),
	)

	navigateAndRestore(t, "/dashboard")
	if shellMounts != 1 {
		t.Fatalf("shellMounts after entering layout = %d, want 1", shellMounts)
	}

	router.Navigate("/dashboard/users")
	waitForPath(t, "/dashboard/users")

	if shellMounts != 1 {
		t.Errorf("shellMounts after sub-navigation = %d, want 1 (shell must persist across sub-routes)", shellMounts)
	}
	raw := core.Unwrap(el)
	if got := raw.Call("querySelector", ".shell").Get("textContent").String(); got != "dashboard users" {
		t.Errorf("shell textContent after sub-navigation = %q, want %q", got, "dashboard users")
	}
}

//* Lifecycle stress

// TestRouterRepeatedNavigationDisposesRouteScope repeatedly navigates
// between two routes, each creating its own effect, and verifies
// LiveEffectCount doesn't grow with the number of round trips - each route's
// scope must be disposed before the next one mounts, not just at the very
// end.
func TestRouterRepeatedNavigationDisposesRouteScope(t *testing.T) {
	t.Cleanup(func() {
		router.Navigate("/")
		waitForPath(t, "/")
	})

	router.Router(
		router.Route("/lifecycle-a", func() core.Node {
			signal.Effect(func() {})
			return core.Text("A")
		}),
		router.Route("/lifecycle-b", func() core.Node {
			signal.Effect(func() {})
			return core.Text("B")
		}),
	)

	router.Navigate("/lifecycle-a")
	waitForPath(t, "/lifecycle-a")
	baseline := signal.LiveEffectCount()

	const iterations = 30
	for range iterations {
		router.Navigate("/lifecycle-b")
		waitForPath(t, "/lifecycle-b")
		router.Navigate("/lifecycle-a")
		waitForPath(t, "/lifecycle-a")
	}

	if got := signal.LiveEffectCount(); got != baseline {
		t.Fatalf("LiveEffectCount() = %d after %d navigation round trips, want %d (route effects were not disposed on navigation)", got, iterations, baseline)
	}
}

// TestRouterSameRouteParamChangeDoesNotRecreateScope verifies that
// navigating between different params of the SAME route pattern (the
// mountEntries fast path that just updates the params signal) never
// disposes and rebuilds the route's scope - LiveEffectCount must stay flat
// across every param change.
func TestRouterSameRouteParamChangeDoesNotRecreateScope(t *testing.T) {
	t.Cleanup(func() {
		router.Navigate("/")
		waitForPath(t, "/")
	})

	router.Router(
		router.Route("/users/:id", func() core.Node {
			signal.Effect(func() {})
			return core.El("div")
		}),
	)

	router.Navigate("/users/1")
	waitForPath(t, "/users/1")
	baseline := signal.LiveEffectCount()

	for _, id := range []string{"2", "3", "4", "5"} {
		router.Navigate("/users/" + id)
		waitForPath(t, "/users/"+id)
		if got := signal.LiveEffectCount(); got != baseline {
			t.Fatalf("LiveEffectCount() = %d after navigating to /users/%s (same pattern, different param), want %d (the component was remounted instead of just updating params)", got, id, baseline)
		}
	}
}

// TestRouterLayoutInnerScopeDisposedOnLayoutSwap verifies that swapping
// between two different layouts fully disposes the outgoing layout's scope,
// including its inner router effect and whichever inner route was active -
// not just the currently active leaf route. Both layouts' single active
// route creates exactly one effect and their shells create none, so
// LiveEffectCount right after entering either layout's default route must
// be identical; if the outgoing layout leaked, entering the other would show
// growth instead.
func TestRouterLayoutInnerScopeDisposedOnLayoutSwap(t *testing.T) {
	t.Cleanup(func() {
		router.Navigate("/")
		waitForPath(t, "/")
	})

	shell := func() core.Node {
		outlet := router.Outlet()
		div := core.El("div")
		core.AppendChild(div, outlet)
		return div
	}

	router.Router(
		router.Layout("/layout-a", shell,
			router.Route("", func() core.Node {
				signal.Effect(func() {})
				return core.Text("a-home")
			}),
			router.Route("sub", func() core.Node {
				signal.Effect(func() {})
				return core.Text("a-sub")
			}),
		),
		router.Layout("/layout-b", shell,
			router.Route("", func() core.Node {
				signal.Effect(func() {})
				return core.Text("b-home")
			}),
		),
	)

	baseline := signal.LiveEffectCount()

	router.Navigate("/layout-a")
	waitForPath(t, "/layout-a")
	afterLayoutA := signal.LiveEffectCount()
	if afterLayoutA != baseline+2 {
		t.Fatalf("LiveEffectCount() = %d after entering layout-a, want %d (1 inner-router effect + 1 route effect)", afterLayoutA, baseline+2)
	}

	router.Navigate("/layout-a/sub")
	waitForPath(t, "/layout-a/sub")
	if got := signal.LiveEffectCount(); got != afterLayoutA {
		t.Fatalf("LiveEffectCount() = %d after sub-navigating within layout-a, want %d (sub-navigation must not accumulate effects)", got, afterLayoutA)
	}

	router.Navigate("/layout-b")
	waitForPath(t, "/layout-b")
	if got := signal.LiveEffectCount(); got != afterLayoutA {
		t.Fatalf("LiveEffectCount() = %d after swapping to layout-b, want %d (layout-a's shell/route effects were not disposed)", got, afterLayoutA)
	}
}
