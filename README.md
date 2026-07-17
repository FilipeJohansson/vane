# Vane

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Go WebAssembly frontend framework with JSX-like syntax and fine-grained reactivity.

Write components in `.vane` files. The compiler turns JSX returns into direct DOM calls: no virtual DOM, no diffing. Each `{expr}` binding gets its own reactive Effect that updates only the affected node when a signal changes.

## Install

```bash
go install github.com/filipejohansson/vane@latest
```

## Quick start

```bash
mkdir my-app && cd my-app
vane init github.com/you/my-app
vane run .
# open http://localhost:8080
```

`vane init <module>` scaffolds a complete project in the current directory, using `<module>` as the `go.mod` module path: `public/index.html`, `public/wasm_exec.js`, `main.go`, `App.vane`, and starter components.

```
my-app/
├── go.mod
├── .gitignore
├── public/             # static assets, copied to dist/ on every build
│   ├── index.html
│   ├── boot.js
│   └── wasm_exec.js
├── src/
│   ├── components/     # .vane components + co-located .css
│   └── store/          # global signals (optional)
├── dist/               # build output (gitignored)
│   └── app.wasm
├── main.go             # bootstrap
└── App.vane            # root component
```

```go
// main.go
//go:build js && wasm

package main

import (
    "github.com/filipejohansson/vane/core"
)

func main() {
    core.Mount("root", App)
    select {}
}
```

## A counter

```go
// App.vane
package main

import (
    "github.com/filipejohansson/vane/core"
)

func App() core.Node {
    count := core.NewSignal(0)

    return (
        <div className="card">
            <h1>Count: {count.Get()}</h1>
            <button onClick={func() { count.Set(count.Get() + 1) }}>+1</button>
        </div>
    )
}
```

`{count.Get()}` is its own reactive binding: clicking the button updates only that text node, the `<button>` itself is never touched.

## Why Vane

- **No virtual DOM.** JSX compiles straight to `core.El`/`AppendChild`/etc. calls, there's nothing to diff.
- **Fine-grained reactivity.** Every `{expr}` is its own signal-driven Effect, not a component-level re-render.
- **Own compiler.** `.vane` → `.go` with source maps and `//line` directives, so Go compiler errors and stack traces point back at your `.vane` source.
- **Plain Go underneath.** Components are Go functions returning `core.Node`. No new runtime to learn.
- **Batteries included.** Router (layouts, params, nested routes), accessibility helpers (`FocusTrap`, `Announce`, first-class `aria-*`), portals, error boundaries, reactive `<head>` management, hot-reload dev server.

## Docs

The full docs site is at [filipejohansson.github.io/vane](https://filipejohansson.github.io/vane/#/docs): concepts, components, signals & reactivity, JSX syntax, refs & DOM, style, accessibility, routing, head management, portal, error handling, raw HTML, global store, do's and don'ts, and develop & build.

## Known limitations

Vane is pre-`0.1.0`, the API can still change.

- **No public VS Code extension yet.** One is in development, syntax highlighting, autocomplete, go-to-definition, and it's what was used to build Vane's own docs site, but it's still rough with several open issues, not ready to publish.
- **WASM binary size.** 2.5–10MB per app with the standard Go compiler, normal for Go WASM but larger than a typical JS bundle. `vane build --tinygo` cuts that to roughly a third (needs a separate TinyGo + binaryen install), at the cost of `//line`-accurate breakpoints in `vane run --debug --tinygo` (TinyGo's WASM DWARF output isn't compatible with Chrome's breakpoint engine).
- **Test coverage has a ceiling.** The DOM test suite runs against jsdom (no real browser), which has no layout engine, so anything depending on real layout (`offsetWidth`, `getBoundingClientRect`) isn't covered yet.
- **No SSR.** SPA-only.

## Examples

```bash
go build -o vane .

vane run examples/async-fetch         # goroutine + net/http + signals
vane run examples/component-api       # Modal + Toaster via controller pattern
vane run examples/forms-and-lists     # router, forms, validation, accordion
vane run examples/fullstack-app       # wasm frontend + separate Go API backend, auth, dashboard, users CRUD
vane run examples/lifecycle           # Effect patterns: post-mount, reactive, cleanup
vane run examples/reactive-showcase   # fine-grained reactivity, computed, effects
vane run examples/routing             # hash router, named params, 404
vane run examples/shared-store        # global store, auth pattern
```
