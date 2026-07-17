//go:build js && wasm

package core

import (
	"errors"

	"syscall/js"
)

// WriteClipboardText writes s to the system clipboard via the async
// Clipboard API. Returns an error immediately if the API is unavailable
// (older browsers, or a non-secure/non-HTTPS context, which the Clipboard
// API requires); otherwise it's fire-and-forget, matching the browser's own
// writeText, which resolves or rejects a promise this doesn't wait on.
func WriteClipboardText(s string) error {
	clipboard := js.Global().Get("navigator").Get("clipboard")
	if clipboard.IsUndefined() || clipboard.IsNull() {
		return errors.New("clipboard: Clipboard API unavailable (requires a secure context)")
	}
	clipboard.Call("writeText", s)
	return nil
}
