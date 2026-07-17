//go:build js && wasm

package store

import "github.com/filipejohansson/vane/core"

const themeStorageKey = "vane-theme"

// Theme is the site-wide dark/light setting. ThemeToggle is the only
// component with UI for it, but the <html data-theme> attribute it drives
// cascades to every component through CSS custom properties, so it's real
// app-wide state and belongs here rather than local to one component.
var Theme = core.NewSignal(loadStoredTheme())

func loadStoredTheme() string {
	if stored, ok := core.LocalStorage().Get(themeStorageKey); ok && stored != "" {
		return stored
	}
	return "dark"
}

// PersistTheme writes t to localStorage under the same key Theme was
// loaded from.
func PersistTheme(t string) {
	core.LocalStorage().Set(themeStorageKey, t)
}
