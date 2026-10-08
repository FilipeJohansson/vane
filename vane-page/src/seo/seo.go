// Package seo builds absolute URLs for the head tags (canonical, og:url).
package seo

import (
	"strings"

	"github.com/filipejohansson/vane/core"
)

// CanonicalURL returns the absolute URL of a site path such as "/docs/lists".
// It honors the deploy base path boot.js exposes as window.__vaneBasePath, so
// the result is right at the domain root and under a sub-path alike.
func CanonicalURL(path string) string {
	base := strings.TrimSuffix(core.Window().Get("__vaneBasePath").String(), "/")
	return core.Window().Get("location").Get("origin").String() + base + path
}
