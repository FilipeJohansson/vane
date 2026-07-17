//go:build js && wasm

package core

import (
	"fmt"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/filipejohansson/vane/core/domattrs"
	"github.com/filipejohansson/vane/internal/dom"
)

// SetProp sets a property on a DOM element once (no reactivity).
func SetProp(el Node, key string, val any) {
	setPropValue(Unwrap(el), key, val)
}

// SetStyle applies a Style struct to el once (no reactivity).
func SetStyle(el Node, style Style) {
	style.apply(Unwrap(el).Get(dom.Style))
}

func setPropValue(el js.Value, key string, val any) {
	switch key {
	case "disabled":
		if b, ok := val.(bool); ok {
			el.Set(dom.Disabled, b)
		}
	case "value":
		setValuePreservingCursor(el, val)
	case "innerHTML", "outerHTML":
		// These properties parse their assigned string as markup (browser
		// behavior, not vane's), so silently allowing them here would open a
		// second, undocumented raw-HTML-injection path alongside
		// core.DangerousInnerHTML, with no warning and no "dangerous" name
		// to signal the risk. Block it and point at the one sanctioned,
		// explicit, grep-able escape hatch instead.
		Warn(fmt.Sprintf(
			"%s is not settable via a prop, since it silently injects raw HTML with no XSS warning. Use core.DangerousInnerHTML(html) as a child instead.",
			key))
	default:
		if strings.HasPrefix(key, "data-") || strings.HasPrefix(key, "aria-") {
			el.Call(dom.SetAttribute, key, fmt.Sprint(val))
			return
		}
		if correct, ok := domattrs.Lookup(key); ok {
			Warn(fmt.Sprintf("%q is the HTML attribute spelling, not the DOM property vane sets. Use %q instead, or it silently does nothing.", key, correct))
		}
		switch v := val.(type) {
		case bool:
			el.Set(key, v)
		case int:
			el.Set(key, strconv.Itoa(v))
		case string:
			el.Set(key, v)
		case js.Value:
			el.Set(key, v)
		default:
			el.Set(key, fmt.Sprint(v))
		}
	}
}

func setValuePreservingCursor(el js.Value, val any) {
	strVal := stringify(val)
	// Skip when the DOM value already matches, to avoid re-setting what the
	// browser just wrote natively, which would interrupt composition/autocomplete
	// and cause the input to lose focus.
	if el.Get("value").String() == strVal {
		return
	}
	if !dom.Document.Get(dom.ActiveElement).Equal(el) {
		el.Set(dom.Value, strVal)
		return
	}
	selStart := el.Get("selectionStart")
	if selStart.IsNull() || selStart.IsUndefined() {
		el.Set(dom.Value, strVal)
		return
	}
	start := selStart.Int()
	end := el.Get("selectionEnd").Int()
	el.Set(dom.Value, strVal)
	el.Set("selectionStart", start)
	el.Set("selectionEnd", end)
}
