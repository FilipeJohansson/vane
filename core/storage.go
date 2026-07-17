//go:build js && wasm

package core

import "syscall/js"

// storageHandle is the concrete type LocalStorage returns. It carries no
// state of its own, methods go straight to the browser's localStorage on
// every call, so a handle never goes stale and is cheap to discard.
type storageHandle struct{}

// LocalStorage returns a handle onto the browser's localStorage.
func LocalStorage() storageHandle {
	return storageHandle{}
}

// Get returns the value stored under key, or ("", false) if it isn't set.
func (storageHandle) Get(key string) (string, bool) {
	v := js.Global().Get("localStorage").Call("getItem", key)
	if v.IsNull() {
		return "", false
	}
	return v.String(), true
}

// Set stores value under key.
func (storageHandle) Set(key, value string) {
	js.Global().Get("localStorage").Call("setItem", key, value)
}
