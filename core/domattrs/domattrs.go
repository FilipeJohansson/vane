// Package domattrs lists HTML attribute spellings that diverge from the
// actual JS/DOM property name (e.g. "class" vs "className"). Vane sets props
// by exact property name, with no attribute-to-property translation, so
// writing the HTML spelling instead of the property name silently creates a
// useless expando property and does nothing.
//
// Shared by two call sites that catch the same mistake at different points:
// the compiler (internal/compiler), which rejects a literal divergent
// attribute name in JSX source at compile time, and the core runtime
// (core/props.go), which warns at runtime for a core.SetProp/core.DynProp
// call whose key isn't a compile-time literal.
package domattrs

import "strings"

// Correct maps the lowercased HTML attribute spelling to the real DOM
// property name, for every case where they diverge.
var Correct = map[string]string{
	"class":           "className",
	"for":             "htmlFor",
	"readonly":        "readOnly",
	"maxlength":       "maxLength",
	"minlength":       "minLength",
	"colspan":         "colSpan",
	"rowspan":         "rowSpan",
	"cellpadding":     "cellPadding",
	"cellspacing":     "cellSpacing",
	"tabindex":        "tabIndex",
	"http-equiv":      "httpEquiv",
	"accept-charset":  "acceptCharset",
	"autocomplete":    "autoComplete",
	"autofocus":       "autoFocus",
	"autoplay":        "autoPlay",
	"contenteditable": "contentEditable",
	"crossorigin":     "crossOrigin",
	"datetime":        "dateTime",
	"enctype":         "encType",
	"formaction":      "formAction",
	"formenctype":     "formEncType",
	"formmethod":      "formMethod",
	"formnovalidate":  "formNoValidate",
	"formtarget":      "formTarget",
	"novalidate":      "noValidate",
	"ismap":           "isMap",
	"usemap":          "useMap",
	"srcset":          "srcSet",
}

// Lookup reports the real property name for name, a case-insensitive match
// against a known divergent HTML attribute spelling. ok is false when name
// has no known divergent spelling, or is already the correct one.
func Lookup(name string) (correct string, ok bool) {
	c, found := Correct[strings.ToLower(name)]
	if !found || c == name {
		return "", false
	}
	return c, true
}
