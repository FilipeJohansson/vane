const base = new URL(".", document.currentScript.src).pathname

// Exposed so App.vane can configure router.PathLocation{BasePath: ...} with
// the same value this script already derives from its own (always-correct)
// script src, instead of hardcoding the deploy sub-path (e.g. "/vane") in Go
// source, which would break local dev (served at "/", not "/vane").
window.__vaneBasePath = base

const go = new Go()
WebAssembly.instantiateStreaming(fetch(base + "app.wasm"), go.importObject)
  .then(r => go.run(r.instance))
