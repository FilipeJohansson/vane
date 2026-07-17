//go:build js && wasm

// Package store holds the app's shared reactive state, the session token
// and current user, readable from any page or component (see README's
// "Global store" pattern).
package store

import (
	"syscall/js"

	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/examples/fullstack-app/src/api"
)

const tokenStorageKey = "fullstack-app.token"

var (
	Token       = core.NewSignal("")
	CurrentUser = core.NewSignal(api.User{})
)

// LoggedIn is a plain function, not a core.ComputedOf: Computed recomputation
// is scheduled asynchronously, so a guard reading it in the same tick as a
// just-called Token.Set (e.g. RestoreSession on boot) would see a stale
// cached value. Token.Get() itself is synchronous right after Set, so reading
// it directly here is what makes the guard correct on first paint.
func LoggedIn() bool {
	return Token.Get() != ""
}

func localStorage() js.Value {
	return js.Global().Get("localStorage")
}

func Login(email, password string) error {
	user, token, err := api.Login(email, password)
	if err != nil {
		return err
	}
	setSession(user, token)
	return nil
}

func Register(name, email, password string) error {
	user, token, err := api.Register(name, email, password)
	if err != nil {
		return err
	}
	setSession(user, token)
	return nil
}

// Logout clears the local session immediately and revokes the token
// server-side best-effort, so the user doesn't need to wait on the network call.
func Logout() {
	token := Token.Get()
	go api.Logout(token)
	clearSession()
}

func setSession(user api.User, token string) {
	Token.Set(token)
	CurrentUser.Set(user)
	localStorage().Call("setItem", tokenStorageKey, token)
}

func clearSession() {
	Token.Set("")
	CurrentUser.Set(api.User{})
	localStorage().Call("removeItem", tokenStorageKey)
}

// RestoreSession reads a persisted token synchronously on boot, so route
// guards see the right LoggedIn value before the first paint, then validates
// it against the backend in the background. Call once from main().
func RestoreSession() {
	item := localStorage().Call("getItem", tokenStorageKey)
	if item.IsNull() || item.String() == "" {
		return
	}
	token := item.String()
	Token.Set(token)

	go func() {
		user, err := api.Me(token)
		if err != nil {
			clearSession() // token expired/invalid server-side, drop it
			return
		}
		CurrentUser.Set(user)
	}()
}
