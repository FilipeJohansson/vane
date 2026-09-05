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

// Href joins the configured BasePath with path.
func (l *PathLocation) Href(path string) string {
	base := l.base()
	if base == "/" {
		return normalizePath(path)
	}
	return base + normalizePath(path)
}

// Navigate pushes a new history entry via history.pushState.
func (l *PathLocation) Navigate(path string) {
	updateHistory(l.Href(path), false, l.onChange)
}

// Replace swaps the current history entry via history.replaceState.
func (l *PathLocation) Replace(path string) {
	updateHistory(l.Href(path), true, l.onChange)
}

// OnChange registers fn to run on "popstate" (browser back/forward).
func (l *PathLocation) OnChange(fn func()) {
	l.onChange = fn
	dom.Window.Call(dom.AddEventListener, dom.EventPopState, js.FuncOf(
		func(_ js.Value, _ []js.Value) interface{} {
			fn()
			return nil
		},
	))
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
// back/forward).
func (l *HashLocation) OnChange(fn func()) {
	l.onChange = fn
	dom.Window.Call(dom.AddEventListener, dom.EventHashChange, js.FuncOf(
		func(_ js.Value, _ []js.Value) interface{} {
			fn()
			return nil
		},
	))
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
