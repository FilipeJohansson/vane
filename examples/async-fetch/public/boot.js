const base = new URL(".", document.currentScript.src).pathname

const go = new Go()
WebAssembly.instantiateStreaming(fetch(base + "app.wasm"), go.importObject)
  .then(r => go.run(r.instance))
