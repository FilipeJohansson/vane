//go:build js && wasm

package core

import "syscall/js"

// LocalStorageGet returns the value stored under key in the browser's
// localStorage, or ("", false) if it isn't set.
func LocalStorageGet(key string) (string, bool) {
	v := js.Global().Get("localStorage").Call("getItem", key)
	if v.IsNull() {
		return "", false
	}
	return v.String(), true
}

// LocalStorageSet stores value under key in the browser's localStorage.
func LocalStorageSet(key, value string) {
	js.Global().Get("localStorage").Call("setItem", key, value)
}
