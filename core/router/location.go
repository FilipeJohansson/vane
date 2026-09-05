//go:build js && wasm

package router

import (
	"fmt"
	"strings"
	"syscall/js"

	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/internal/dom"
)

// Location abstracts how the router represents its current route in the
// browser's URL, and how it's notified when that URL changes from outside
// the router (back/forward, a typed URL, ...). The router only ever deals
// with application paths (e.g. "/docs"); a Location decides how that path
// is read from and written to the browser's actual URL.
type Location interface {
	// Path returns the current application path, derived from the browser's URL.
	Path() string
	// Href returns the URL this Location would use to represent path, suitable
	// for an anchor's href attribute.
	Href(path string) string
	// Navigate changes the current route, pushing a new browser history entry.
	Navigate(path string)
	// Replace changes the current route without pushing a new history entry.
	Replace(path string)
	// OnChange registers fn to run whenever the browser's URL changes to a new
	// route. Called once by the router at startup.
	OnChange(fn func())
	// AnchorID returns the id of an element the current URL wants scrolled
	// into view (the text after a "#" that isn't itself the routing
	// mechanism), or "" if there is none. The router calls this after every
	// navigation to decide between scrolling to that element and the default
	// scroll-to-top.
	AnchorID() string
}

var (
	_ Location = (*PathLocation)(nil)
	_ Location = (*HashLocation)(nil)
)

// PathLocation represents the router's state in the pathname of the
// browser's URL (e.g. "/docs"), using the History API (pushState/
// replaceState) to navigate without a full page reload. Requires the host
// server to fall back to index.html for unknown paths, since a direct
// load or refresh of e.g. "/docs" is a real request for that path.
//
// A plain <a href="/docs"> would otherwise make the browser reload the page;
// PathLocation intercepts same-origin, unmodified left-clicks on in-app
// links to navigate via the router instead, letting everything else
// (modified clicks, new-tab, external/cross-origin links, target="_blank",
// download links, or a click already handled by the app) through untouched.
//
//	router.SetLocation(&router.PathLocation{BasePath: "/vane"})
type PathLocation struct {
	// BasePath is the path prefix the app is served under (e.g. "/vane" when
	// deployed at https://example.com/vane/). Optional; the zero value means
	// the app is served at the root ("/").
	BasePath string

	onChange func()
}

// base returns BasePath normalized, defaulting to "/" when unset.
func (l *PathLocation) base() string {
	return normalizeBasePath(l.BasePath)
}

// stripBase removes the configured BasePath from pathname, returning ok=false
// if pathname isn't actually under it. Matches on a path-segment boundary
// (pathname == base, or pathname starting with base+"/"), so a BasePath of
// "/vane" doesn't accidentally match "/vanessa".
func (l *PathLocation) stripBase(pathname string) (string, bool) {
	base := l.base()
	if base == "/" {
		return pathname, true
	}
	if pathname == base {
		return "/", true
	}
	if strings.HasPrefix(pathname, base+"/") {
		return pathname[len(base):], true
	}
	return "", false
}

// Path reads location.pathname and strips the configured BasePath.
func (l *PathLocation) Path() string {
	pathname := js.Global().Get("location").Get("pathname").String()
	rest, ok := l.stripBase(pathname)
	if !ok {
		core.Warn(fmt.Sprintf("router.PathLocation: pathname %q is outside configured BasePath %q, falling back to \"/\"", pathname, l.base()))
		return "/"
	}
	return normalizePath(rest)
}

// Href joins the configured BasePath with path, then resolves the result
// against the current document location and keeps only its pathname, query
// and fragment. That last step means the returned href can never point
// outside the current origin, no matter what path contains (e.g. a
// protocol-relative "//evil.com" would otherwise survive untouched, since it
// already "starts with /"): pushState/replaceState throw a SecurityError for
// any URL outside the current origin, and Navigate/Replace both build their
// href through this method, so this is what keeps them safe to call with an
// arbitrary string.
//
// A path that's only a fragment ("#section") or only a query ("?tab=2") is
// resolved relative to the current URL instead of being joined with
// BasePath, so it preserves the current path (already part of that URL) -
// same as a plain <a href="#section">, whose target inherits the current
// page's path under WHATWG URL resolution rules, rather than jumping to the
// app's root.
func (l *PathLocation) Href(path string) string {
	if strings.HasPrefix(path, "#") || strings.HasPrefix(path, "?") {
		return resolveSameOrigin(path)
	}
	base := l.base()
	joined := normalizePath(path)
	if base != "/" {
		joined = base + joined
	}
	return resolveSameOrigin(joined)
}

// AnchorID returns the current URL's fragment, without the leading "#", if
// any. PathLocation never uses the fragment for routing (see Path()), so any
// fragment present is always a genuine in-page anchor target.
func (l *PathLocation) AnchorID() string {
	return strings.TrimPrefix(js.Global().Get("location").Get("hash").String(), "#")
}

// resolveSameOrigin resolves href against the current document location and
// returns only its pathname+search+hash, discarding any scheme/host it may
// have picked up in the process.
func resolveSameOrigin(href string) string {
	u := js.Global().Get("URL").New(href, js.Global().Get("location").Get("href"))
	return u.Get("pathname").String() + u.Get("search").String() + u.Get("hash").String()
}

// Navigate pushes a new history entry via history.pushState.
func (l *PathLocation) Navigate(path string) {
	updateHistory(l.Href(path), false, l.onChange)
}

// Replace swaps the current history entry via history.replaceState.
func (l *PathLocation) Replace(path string) {
	updateHistory(l.Href(path), true, l.onChange)
}

// OnChange registers fn to run on "popstate" (browser back/forward) and on
// an intercepted in-app link click (see handleClick).
func (l *PathLocation) OnChange(fn func()) {
	l.onChange = fn
	dom.Window.Call(dom.AddEventListener, dom.EventPopState, js.FuncOf(
		func(_ js.Value, _ []js.Value) interface{} {
			fn()
			return nil
		},
	))
	dom.Document.Call(dom.AddEventListener, dom.EventClick, js.FuncOf(l.handleClick))
}

// handleClick intercepts a document-level click so a plain <a href="/x">
// navigates via the router (pushState) instead of the browser reloading the
// page. Left as a no-op for anything that isn't a plain, unmodified,
// same-origin, in-app left-click on a link (see PathLocation's doc comment
// for the exact list of what's left untouched).
func (l *PathLocation) handleClick(_ js.Value, args []js.Value) interface{} {
	event := args[0]

	if event.Get("defaultPrevented").Bool() {
		return nil
	}
	if event.Get("button").Int() != 0 {
		return nil
	}
	if event.Get("ctrlKey").Bool() || event.Get("metaKey").Bool() ||
		event.Get("shiftKey").Bool() || event.Get("altKey").Bool() {
		return nil
	}

	anchor := event.Get("target").Call("closest", "a")
	if !anchor.Truthy() {
		return nil
	}
	if target := anchor.Get("target").String(); target != "" && target != "_self" {
		return nil
	}
	if anchor.Call("hasAttribute", "download").Bool() {
		return nil
	}

	loc := js.Global().Get("location")
	if anchor.Get("origin").String() != loc.Get("origin").String() {
		return nil
	}
	// A same-page fragment link (e.g. href="#section") only ever changes the
	// hash: nothing for the router to do, and intercepting it would replace
	// the browser's native scroll-to-anchor behavior with a silent pushState.
	if anchor.Get("hash").String() != "" &&
		anchor.Get("pathname").String() == loc.Get("pathname").String() &&
		anchor.Get("search").String() == loc.Get("search").String() {
		return nil
	}
	if _, ok := l.stripBase(anchor.Get("pathname").String()); !ok {
		return nil
	}

	event.Call(dom.PreventDefault)
	updateHistory(anchor.Get("href").String(), false, l.onChange)
	return nil
}

// HashLocation represents the router's state in the hash fragment of the
// browser's URL (e.g. "#/docs"). This is the router's default: the hash is
// never sent to the server, so it works on any static host without needing
// a server-side rewrite rule for unknown paths.
type HashLocation struct {
	onChange func()
}

// Path reads location.hash. Only hashes starting with "#/" are router
// paths — plain anchor hashes like "#section-id" are native browser scroll
// targets, so those are treated as "/".
func (l *HashLocation) Path() string {
	hash := js.Global().Get("location").Get("hash").String()
	if !strings.HasPrefix(hash, "#/") {
		return "/"
	}
	return normalizePath(hash[1:])
}

// Href returns path as a hash fragment (e.g. "#/docs").
func (l *HashLocation) Href(path string) string {
	return "#" + normalizePath(path)
}

// Navigate sets location.hash, which the browser turns into a new history
// entry and fires "hashchange" on its own — that's what invokes the
// OnChange callback below.
func (l *HashLocation) Navigate(path string) {
	js.Global().Get("location").Set("hash", l.Href(path))
}

// Replace swaps the current history entry via history.replaceState, since
// setting location.hash directly always pushes a new one.
func (l *HashLocation) Replace(path string) {
	updateHistory(l.Href(path), true, l.onChange)
}

// OnChange registers fn to run on "hashchange" (Navigate, or browser
// back/forward), but only when the new hash is actually a route ("#/...").
// A same-page anchor hash (e.g. "#section") also fires "hashchange", but
// isn't a route change at all (see Path()'s doc comment on the quirk); the
// router would otherwise treat it as a navigation to "/" and scroll to top,
// fighting the browser's own scroll-to-anchor, the same class of problem
// PathLocation.handleClick already avoids for its equivalent case.
func (l *HashLocation) OnChange(fn func()) {
	l.onChange = fn
	dom.Window.Call(dom.AddEventListener, dom.EventHashChange, js.FuncOf(
		func(_ js.Value, _ []js.Value) interface{} {
			hash := js.Global().Get("location").Get("hash").String()
			if !strings.HasPrefix(hash, "#/") {
				return nil
			}
			fn()
			return nil
		},
	))
}

// AnchorID always returns "" - HashLocation uses the fragment as the routing
// mechanism itself (see Path()'s doc comment on the accepted quirk), so
// there's no separate in-page anchor for the router to scroll to.
func (l *HashLocation) AnchorID() string {
	return ""
}

// updateHistory pushes or replaces a browser history entry with href. Unlike
// setting location.hash, history.pushState/replaceState never fire a browser
// event on their own, so notify is invoked manually afterwards.
func updateHistory(href string, replace bool, notify func()) {
	method := "pushState"
	if replace {
		method = "replaceState"
	}
	dom.History.Call(method, js.Null(), "", href)
	if notify != nil {
		notify()
	}
}

func normalizeBasePath(path string) string {
	if path == "" {
		return "/"
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	path = strings.TrimRight(path, "/")

	if path == "" {
		return "/"
	}

	return path
}

func normalizePath(path string) string {
	if path == "" {
		return "/"
	}

	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}

	return path
}
