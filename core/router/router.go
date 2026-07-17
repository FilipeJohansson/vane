//go:build js && wasm

package router

import (
	"strings"
	"syscall/js"

	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/core/signal"
	"github.com/filipejohansson/vane/internal/dom"
)

// Entry is a sealed interface for router config items (Route or Layout).
type Entry interface{ routerEntry() }

type routeEntry struct {
	pattern string
	fn      func() core.Node
	title   string
}

func (routeEntry) routerEntry() {}

type layoutEntry struct {
	prefix   string
	shell    func() core.Node
	children []Entry
	title    string
}

func (layoutEntry) routerEntry() {}

// routerCtx is the internal context pushed onto ctxStack before calling a component.
// WASM is single-threaded and component setup is always sync, so a package-level stack is safe.
type routerCtx struct {
	params *signal.Signal[map[string]string]
	outlet core.Node
}

var (
	ctxStack     []routerCtx
	pathSignal   *signal.Signal[string]
	pathReadOnly *signal.ReadOnlySignal[string]
)

func init() {
	pathSignal = signal.New(currentLocation())
	pathReadOnly = pathSignal.ReadOnly()
	listenLocation(func(p string) { pathSignal.Set(p) })
}

func currentLocation() string {
	hash := js.Global().Get("location").Get("hash").String()
	// Only hashes starting with "#/" are router paths. Plain anchor hashes like
	// "#section-id" are native browser scroll targets, so those are treated as "/".
	if hash == "" || hash == "#" || !strings.HasPrefix(hash, "#/") {
		return "/"
	}
	return hash[1:]
}

func listenLocation(fn func(string)) {
	js.Global().Get("window").Call(dom.AddEventListener, dom.EventHashChange, js.FuncOf(
		func(_ js.Value, _ []js.Value) interface{} {
			hash := js.Global().Get("location").Get("hash").String()
			fn(currentLocation())
			if strings.HasPrefix(hash, "#/") {
				js.Global().Get("window").Call("scrollTo", 0, 0)
			}
			return nil
		},
	))
}

// Path returns the current path as a reactive read-only signal.
// Use as an escape hatch for manual routing logic.
func Path() *signal.ReadOnlySignal[string] { return pathReadOnly }

// Params returns the route params for the current component as a reactive signal.
// Must be called during component setup (sync), not inside an Effect.
// Returns an empty signal if called outside a router context.
func Params() *signal.ReadOnlySignal[map[string]string] {
	if len(ctxStack) == 0 {
		core.Warn("router.Params() called outside a router context, returning an empty signal")
		return signal.New(map[string]string{}).ReadOnly()
	}
	return ctxStack[len(ctxStack)-1].params.ReadOnly()
}

// Outlet returns the stable DOM node that the sub-router populates.
// Must be called during layout shell setup (sync), not inside an Effect.
// Returns core.Empty() if called outside a layout context.
func Outlet() core.Node {
	if len(ctxStack) == 0 {
		core.Warn("router.Outlet() called outside a Layout shell")
		return core.Empty()
	}
	return ctxStack[len(ctxStack)-1].outlet
}

// Route creates a leaf route entry.
//
//	router.Route("/users/:id", UserPage)
//	router.Route("/users/:id", UserPage, "User Detail")
func Route(pattern string, fn func() core.Node, title ...string) Entry {
	t := ""
	if len(title) > 0 {
		t = title[0]
	}
	return routeEntry{pattern: pattern, fn: fn, title: t}
}

// Layout creates a persistent layout entry with nested child routes.
// Shell runs once per prefix match; only the Outlet content changes on sub-navigation.
// Children patterns are relative to prefix (leading slash optional).
//
//	router.Layout("/dashboard", DashboardShell,
//	    router.Route("stats", Stats),
//	    router.Route("users/:id", UserPage),
//	)
func Layout(prefix string, shell func() core.Node, children ...Entry) Entry {
	return layoutEntry{prefix: prefix, shell: shell, children: children}
}

// Router renders the first matching entry for the current path.
// Returns a stable container element; contents replace reactively on navigation.
func Router(entries ...Entry) core.Node {
	container := core.El("div")
	state := &routerState{}
	signal.Effect(func() {
		path := pathSignal.Get()
		mountEntries(container, entries, path, "", state)
	})
	return container
}

// Navigate programmatically changes the current route.
func Navigate(to string) {
	js.Global().Get("location").Set("hash", to)
}

// LinkProps configures a Link element.
// Embed core.HTMLProps for common attributes (Class, ID, Style, OnClick, Extra).
type LinkProps struct {
	core.HTMLProps
	To     string
	Target string
	Rel    string
}

// Link renders an anchor element that navigates to the given path.
func Link(props LinkProps, children ...any) core.Node {
	if props.To == "" {
		core.Warn("router.Link called with an empty To, which creates a link to \"#\" that navigates to \"/\"")
	}
	return core.ElWithProps("a", linkHTMLProps(props), core.Nodes(children...)...)
}

// linkHTMLProps folds To/Target/Rel into a copy of props.HTMLProps.Extra, the
// (not just literal) attrs ElWithProps can't take as typed HTMLProps fields.
// Copies rather than mutates Extra so the caller's own map isn't touched.
func linkHTMLProps(props LinkProps) core.HTMLProps {
	p := props.HTMLProps
	extra := make(map[string]string, len(p.Extra)+3)
	for k, v := range p.Extra {
		extra[k] = v
	}
	extra["href"] = "#" + props.To
	if props.Target != "" {
		extra["target"] = props.Target
	}
	if props.Rel != "" {
		extra["rel"] = props.Rel
	}
	p.Extra = extra
	return p
}

// ActiveLinkProps configures an ActiveLink element.
// Class is always applied; ActiveClass is added when the path matches To.
type ActiveLinkProps struct {
	LinkProps
	ActiveClass string
	// Exact restricts matching to To itself, rather than prefix-matching its
	// sub-paths. "/" is always exact regardless of this field. Set Exact when
	// To is a section index (e.g. "/dashboard") that has its own sub-routes
	// (e.g. "/dashboard/users") which should not also mark this link active.
	Exact bool
}

// ActiveLink renders an anchor that reactively adds ActiveClass when the
// current path matches To (prefix match; "/" uses exact match; set
// ActiveLinkProps.Exact to force exact match for any other To).
func ActiveLink(props ActiveLinkProps, children ...any) core.Node {
	if props.To == "" {
		core.Warn("router.ActiveLink called with an empty To, so the link will always be marked active (matches every path)")
	}
	el := core.ElWithProps("a", linkHTMLProps(props.LinkProps), core.Nodes(children...)...)
	raw := core.Unwrap(el)
	signal.Effect(func() {
		path := pathSignal.Get()
		var active bool
		if props.To == "/" || props.Exact {
			active = path == props.To
		} else {
			_, active = matchPrefix(props.To, path)
		}
		cls := props.Class
		if active && props.ActiveClass != "" {
			if cls != "" {
				cls += " " + props.ActiveClass
			} else {
				cls = props.ActiveClass
			}
		}
		raw.Set(dom.ClassName, cls)
	})
	return el
}

// IsActive returns a read-only signal that is true when the current path
// matches to (prefix match; "/" uses exact match).
// Call during component setup so the underlying Effect is scoped to the component.
func IsActive(to string) *signal.ReadOnlySignal[bool] {
	if to == "" {
		core.Warn("router.IsActive called with an empty to, and it will always report active (matches every path)")
	}
	s := signal.New(false)
	signal.Effect(func() {
		path := pathSignal.Get()
		var active bool
		if to == "/" {
			active = path == "/"
		} else {
			_, active = matchPrefix(to, path)
		}
		s.Set(active)
	})
	return s.ReadOnly()
}

// --- internals ---

type routerState struct {
	activeKey    string // matched pattern or layout prefix
	activeScope  *signal.Scope
	paramsSignal *signal.Signal[map[string]string]
}

func clearState(container core.Node, s *routerState) {
	if s.activeScope != nil {
		s.activeScope.Dispose()
		s.activeScope = nil
	}
	core.Unwrap(container).Set("innerHTML", "")
	s.activeKey = ""
	s.paramsSignal = nil
}

func mountEntries(container core.Node, entries []Entry, path, prefix string, state *routerState) {
	raw := core.Unwrap(container)
	for _, entry := range entries {
		switch e := entry.(type) {
		case routeEntry:
			full := joinPath(prefix, e.pattern)
			var params map[string]string
			var ok bool
			if e.pattern == "*" {
				params, ok = map[string]string{}, true
			} else {
				params, ok = matchRoute(full, path)
			}
			if !ok {
				continue
			}
			if full == state.activeKey {
				// same pattern: update params signal, component reacts via its own Effects
				state.paramsSignal.Set(params)
				return
			}
			clearState(container, state)
			state.activeKey = full
			state.paramsSignal = signal.New(params)
			ps := state.paramsSignal
			state.activeScope = signal.RunScoped(func() {
				if e.title != "" {
					t := e.title
					core.Head(func() core.HeadConfig { return core.HeadConfig{Title: t} })
				}
				ctxStack = append(ctxStack, routerCtx{params: ps})
				signal.Untrack(func() {
					raw.Call(dom.AppendChild, core.Unwrap(e.fn()))
				})
				ctxStack = ctxStack[:len(ctxStack)-1]
			})
			return

		case layoutEntry:
			fullPrefix := joinPath(prefix, e.prefix)
			params, ok := matchPrefix(fullPrefix, path)
			if !ok {
				continue
			}
			if fullPrefix == state.activeKey {
				// layout still active: update outer params, inner Effect handles sub-path
				state.paramsSignal.Set(params)
				return
			}
			clearState(container, state)
			state.activeKey = fullPrefix
			state.paramsSignal = signal.New(params)
			subOutlet := core.El("div")
			ps := state.paramsSignal
			innerState := &routerState{}
			state.activeScope = signal.RunScoped(func() {
				if e.title != "" {
					t := e.title
					core.Head(func() core.HeadConfig { return core.HeadConfig{Title: t} })
				}
				ctxStack = append(ctxStack, routerCtx{params: ps, outlet: subOutlet})
				signal.Untrack(func() {
					raw.Call(dom.AppendChild, core.Unwrap(e.shell()))
				})
				ctxStack = ctxStack[:len(ctxStack)-1]

				// Inner router: tracks path independently, disposed when layout unmounts.
				signal.Effect(func() {
					innerPath := pathSignal.Get()
					mountEntries(subOutlet, e.children, innerPath, fullPrefix, innerState)
				})
				// Ensure the last active child scope is disposed on layout unmount,
				// since it lives outside the inner Effect's own scope on subsequent runs.
				signal.RegisterDispose(func() {
					if innerState.activeScope != nil {
						innerState.activeScope.Dispose()
					}
				})
			})
			return
		}
	}

	// no match
	clearState(container, state)
	raw.Call(dom.AppendChild, core.Unwrap(buildPageNotFound()))
}

func joinPath(prefix, child string) string {
	child = strings.TrimPrefix(child, "/")
	if child == "" {
		if prefix == "" {
			return "/"
		}
		return prefix
	}
	if prefix == "" {
		return "/" + child
	}
	return strings.TrimRight(prefix, "/") + "/" + child
}

// matchPrefix checks if path starts with all segments of prefix (supports :params).
func matchPrefix(prefix, path string) (map[string]string, bool) {
	if prefix == "" {
		return map[string]string{}, true
	}
	pParts := splitPath(prefix)
	qParts := splitPath(path)
	if len(qParts) < len(pParts) {
		return nil, false
	}
	params := make(map[string]string)
	for i, part := range pParts {
		if strings.HasPrefix(part, ":") {
			params[part[1:]] = qParts[i]
		} else if part != qParts[i] {
			return nil, false
		}
	}
	return params, true
}

// matchRoute checks for exact segment match (supports :params and * wildcard).
func matchRoute(pattern, path string) (map[string]string, bool) {
	if pattern == "*" {
		return map[string]string{}, true
	}
	pParts := splitPath(pattern)
	qParts := splitPath(path)
	if len(pParts) != len(qParts) {
		return nil, false
	}
	params := make(map[string]string)
	for i, part := range pParts {
		if strings.HasPrefix(part, ":") {
			params[part[1:]] = qParts[i]
		} else if part != qParts[i] {
			return nil, false
		}
	}
	return params, true
}

func splitPath(path string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		return []string{}
	}
	return parts
}

func buildPageNotFound() core.Node {
	div := core.El("div")
	core.Unwrap(div).Get(dom.Style).Set("cssText", "display:flex;flex-direction:column;align-items:center;justify-content:center;min-height:100vh;font-family:system-ui,sans-serif;gap:8px")
	h1 := core.El("h1")
	core.Unwrap(h1).Get(dom.Style).Set("cssText", "font-size:4rem;font-weight:700;margin:0;color:#111")
	core.AppendText(h1, "404")
	p := core.El("p")
	core.Unwrap(p).Get(dom.Style).Set("cssText", "font-size:1rem;color:#666;margin:0")
	core.AppendText(p, "Page not found")
	core.AppendChild(div, h1)
	core.AppendChild(div, p)
	return div
}
