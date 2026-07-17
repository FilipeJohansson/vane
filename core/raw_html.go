//go:build js && wasm

package core

import "github.com/filipejohansson/vane/internal/dom"

// DangerousInnerHTML parses html as raw markup and returns it as a Node.
// Use it in child position like any other {expr}, including mixed in among
// other vane-syntax siblings, since it produces a fragment with no wrapper
// element (same technique as Fragment):
//
//	return (
//	    <div>
//	        <h1>{title}</h1>
//	        {core.DangerousInnerHTML(bodyHTML)}
//	        <footer>...</footer>
//	    </div>
//	)
//
// SECURITY: html is inserted into the DOM via a <template>'s innerHTML, and the
// browser parses and renders it as real markup. Never pass user-controlled
// input here without sanitizing it first; doing so is a textbook XSS
// vulnerability. <script> tags inside html do not execute (a documented
// quirk of innerHTML parsing), but event handler attributes (onclick="...")
// and other markup-based attack vectors still do.
func DangerousInnerHTML(html string) Node {
	tmpl := dom.Document.Call(dom.CreateElement, "template")
	tmpl.Set("innerHTML", html)
	return wrap(tmpl.Get("content"))
}
