// wasm_test_exec.js - drop-in replacement for Go's official go_js_wasm_exec,
// with a jsdom DOM injected before the WASM binary runs.
//
// Lets `GOOS=js GOARCH=wasm go test ./core/...` execute vane's real
// syscall/js DOM code (core.El, core.DynList, core.Portal, etc.) against a
// working `document`/`window` - no Chrome/Playwright, no real browser.
//
// Known limitation: jsdom does not compute layout. Anything depending on
// real layout (offsetWidth, getBoundingClientRect, etc. - e.g. the reflow
// trick in examples/reactive-showcase's flashOn) is not meaningfully testable
// here and still needs a real browser.
//
// Usage: GOOS=js GOARCH=wasm go test -exec="node tools/wasmtest/wasm_test_exec.js" ./core/...

"use strict";

const { execSync } = require("child_process");
const path = require("path");

if (process.argv.length < 3) {
	console.error("usage: wasm_test_exec.js [wasm binary] [arguments]");
	process.exit(1);
}

const { JSDOM } = require("jsdom");
const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
	url: "http://localhost/",
});

// jsdom doesn't implement scrollTo/scrollIntoView (no layout engine); scrollTo
// otherwise logs a noisy "Not implemented" error on every call, and
// scrollIntoView is simply undefined. vane's router calls scrollTo on every
// hash navigation, and core.ScrollIntoView/ScrollTo wrap both directly -
// stub both to no-ops so test output stays clean and the wrappers are
// callable under this harness.
dom.window.scrollTo = () => {};
dom.window.Element.prototype.scrollIntoView = () => {};

// Copy jsdom's window properties onto globalThis so `js.Global()` inside the
// WASM binary sees document/window/etc. jsdom's versions win over any Node
// native globals (Node 15+ ships its own Event/EventTarget/navigator/etc.),
// elements and events created by jsdom belong to jsdom's realm, so e.g. an
// `Event` constructed from Node's native global fails el.dispatchEvent's
// internal instanceof check ("parameter 1 is not of type 'Event'").
// defineProperty is used because some of these (navigator, performance,
// crypto) are getter-only on Node's globalThis, plain assignment throws.
//
// Exception: a handful of jsdom's window methods are bound internally to the
// window instance and recurse infinitely (stack overflow) when invoked via a
// plain globalThis reference instead of through the actual window object,
// `performance.now()`, `setTimeout`, etc. Go's wasm runtime calls these
// directly for timing/scheduling, so we leave Node's native versions in place.
const skipGlobal = new Set([
	"performance",
	"setTimeout", "clearTimeout",
	"setInterval", "clearInterval",
	"queueMicrotask",
]);

for (const key of Object.getOwnPropertyNames(dom.window)) {
	if (skipGlobal.has(key)) continue;
	try {
		Object.defineProperty(globalThis, key, {
			value: dom.window[key],
			writable: true,
			configurable: true,
		});
	} catch {
		// circular/non-configurable property (e.g. window.window), ignore
	}
}
Object.defineProperty(globalThis, "window", { value: dom.window, writable: true, configurable: true });

// Node.js support code Go's wasm runtime expects (mirrors wasm_exec_node.js
// shipped in $GOROOT/lib/wasm/).
globalThis.require = require;
globalThis.fs = require("fs");
globalThis.path = require("path");
globalThis.TextEncoder = require("util").TextEncoder;
globalThis.TextDecoder = require("util").TextDecoder;
globalThis.crypto ??= require("crypto");

const goroot = execSync("go env GOROOT").toString().trim();
require(path.join(goroot, "lib", "wasm", "wasm_exec.js"));

const go = new Go();
go.argv = process.argv.slice(2);
// Go's wasm runtime caps total argv+env bytes at 4096, the full process.env
// (huge PATH on Windows dev machines especially) blows that budget with
// "total length of command line and environment variables exceeds limit".
// Only pass what the runtime itself needs.
go.env = { TMPDIR: require("os").tmpdir() };
go.exit = process.exit;

WebAssembly.instantiate(fs.readFileSync(process.argv[2]), go.importObject)
	.then((result) => {
		process.on("exit", (code) => {
			if (code === 0 && !go.exited) {
				// deadlock, make Go print error and stack traces
				go._pendingEvent = { id: 0 };
				go._resume();
			}
		});
		return go.run(result.instance);
	})
	.catch((err) => {
		console.error(err);
		process.exit(1);
	});
