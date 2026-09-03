const base = new URL(".", document.currentScript.src).pathname
if (!location.hash && location.pathname !== base) {
  history.replaceState(null, "", base + "#" + location.pathname.slice(base.length) + location.search)
}

const go = new Go()
WebAssembly.instantiateStreaming(fetch("app.wasm"), go.importObject)
  .then(r => go.run(r.instance))
