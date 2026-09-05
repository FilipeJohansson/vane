//go:build js && wasm

package core

import (
	"fmt"
	"syscall/js"

	"github.com/filipejohansson/vane/core/signal"
	"github.com/filipejohansson/vane/internal/dom"
)

// HeadConfig describes the document head state for one component scope.
// Fields left empty are ignored, so partial updates are fine.
type HeadConfig struct {
	Title       string // document.title
	Description string // <meta name="description">
	Canonical   string // <link rel="canonical">

	// Open Graph tags are used by social media platforms to generate rich link previews.
	// Check https://ogp.me/ for the full spec.

	OGTitle           string // <meta property="og:title">
	OGImage           string // <meta property="og:image">
	OGType            string // <meta property="og:type">
	OGURL             string // <meta property="og:url">
	OGAudio           string // <meta property="og:audio">
	OGDescription     string // <meta property="og:description">
	OGDeterminer      string // <meta property="og:determiner">
	OGLocale          string // <meta property="og:locale">
	OGLocaleAlternate string // <meta property="og:locale:alternate">
	OGSiteName        string // <meta property="og:site_name">
	OGVideo           string // <meta property="og:video">

	// Meta and Link allow arbitrary <meta> and <link> tags to be added to the document head.

	Meta map[string]string // extra <meta name=...> → content
	Link map[string]string // extra <link rel=...> → href
}

// Head applies fn() to the document <head> reactively.
// fn is re-run whenever a signal it reads changes.
// On unmount (or before each re-run) the previous values are restored,
// so Layout and child route can both call Head without clobbering each other.
//
//	core.Head(func() core.HeadConfig {
//	    return core.HeadConfig{
//	        Title:       "User " + params.Get()["id"],
//	        Description: "Team member detail",
//	    }
//	})
func Head(fn func() HeadConfig) {
	var restores []func()

	runCleanup := func() {
		for _, r := range restores {
			r()
		}
		restores = nil
	}

	signal.Effect(func() {
		runCleanup()
		cfg := fn()
		restores = applyHeadConfig(cfg)
	})

	signal.RegisterDispose(runCleanup)
}

// applyHeadConfig upserts the relevant DOM nodes for cfg and returns
// a slice of restore functions that undo each change.
func applyHeadConfig(cfg HeadConfig) []func() {
	var restores []func()

	if cfg.Title != "" {
		prev := dom.Document.Get("title").String()
		dom.Document.Set("title", cfg.Title)
		restores = append(restores, func() { dom.Document.Set("title", prev) })
	}
	if cfg.Description != "" {
		restores = append(restores, upsertMetaName("description", cfg.Description))
	}
	if cfg.Canonical != "" {
		restores = append(restores, upsertLink("canonical", "href", cfg.Canonical))
	}

	if cfg.OGTitle != "" {
		restores = append(restores, upsertMetaProp("og:title", cfg.OGTitle))
	}
	if cfg.OGImage != "" {
		restores = append(restores, upsertMetaProp("og:image", cfg.OGImage))
	}
	if cfg.OGType != "" {
		restores = append(restores, upsertMetaProp("og:type", cfg.OGType))
	}
	if cfg.OGURL != "" {
		restores = append(restores, upsertMetaProp("og:url", cfg.OGURL))
	}
	if cfg.OGAudio != "" {
		restores = append(restores, upsertMetaProp("og:audio", cfg.OGAudio))
	}
	if cfg.OGDescription != "" {
		restores = append(restores, upsertMetaProp("og:description", cfg.OGDescription))
	}
	if cfg.OGDeterminer != "" {
		restores = append(restores, upsertMetaProp("og:determiner", cfg.OGDeterminer))
	}
	if cfg.OGLocale != "" {
		restores = append(restores, upsertMetaProp("og:locale", cfg.OGLocale))
	}
	if cfg.OGLocaleAlternate != "" {
		restores = append(restores, upsertMetaProp("og:locale:alternate", cfg.OGLocaleAlternate))
	}
	if cfg.OGSiteName != "" {
		restores = append(restores, upsertMetaProp("og:site_name", cfg.OGSiteName))
	}
	if cfg.OGVideo != "" {
		restores = append(restores, upsertMetaProp("og:video", cfg.OGVideo))
	}

	for name, content := range cfg.Meta {
		n, c := name, content
		restores = append(restores, upsertMetaName(n, c))
	}
	for rel, href := range cfg.Link {
		r, h := rel, href
		restores = append(restores, upsertLink(r, "href", h))
	}

	return restores
}

// upsertMetaName upserts <meta name="{name}" content="{content}"> in <head>.
func upsertMetaName(name, content string) func() {
	return upsertHeadEl("meta", fmt.Sprintf(`meta[name="%s"]`, name), func(el js.Value) {
		el.Set("name", name)
		el.Set("content", content)
	}, func(el js.Value) string { return el.Get("content").String() },
		func(el js.Value, prev string) { el.Set("content", prev) })
}

// upsertMetaProp upserts <meta property="{prop}" content="{content}"> in <head>.
// "property" is an Open Graph convention, not a standard HTML attribute. It
// has no IDL reflection in any browser, so it must go through setAttribute
// (el.Set("property", ...) would silently create an inert JS expando
// property instead of the actual HTML attribute crawlers read).
func upsertMetaProp(prop, content string) func() {
	return upsertHeadEl("meta", fmt.Sprintf(`meta[property="%s"]`, prop), func(el js.Value) {
		el.Call(dom.SetAttribute, "property", prop)
		el.Set("content", content)
	}, func(el js.Value) string { return el.Get("content").String() },
		func(el js.Value, prev string) { el.Set("content", prev) })
}

// upsertLink upserts <link rel="{rel}" {attr}="{val}"> in <head>.
func upsertLink(rel, attr, val string) func() {
	return upsertHeadEl("link", fmt.Sprintf(`link[rel="%s"]`, rel), func(el js.Value) {
		el.Set("rel", rel)
		el.Set(attr, val)
	}, func(el js.Value) string { return el.Get(attr).String() },
		func(el js.Value, prev string) { el.Set(attr, prev) })
}

// upsertHeadEl finds or creates a <head> element matching selector.
// If created: restore removes it. If updated: restore resets previous value.
func upsertHeadEl(tag, selector string, set func(js.Value), get func(js.Value) string, reset func(js.Value, string)) func() {
	head := dom.Document.Get("head")
	el := head.Call(dom.QuerySelector, selector)
	if el.IsNull() {
		el = dom.Document.Call(dom.CreateElement, tag)
		set(el)
		head.Call(dom.AppendChild, el)
		return func() { el.Call("remove") }
	}
	prev := get(el)
	set(el)
	return func() { reset(el, prev) }
}
