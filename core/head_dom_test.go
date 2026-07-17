//go:build js && wasm

package core_test

// Tests in this file run against a jsdom-backed DOM via
// tools/wasmtest/wasm_test_exec.js, no real browser needed. See
// internal_docs/testing.md for how to run them.
//
// document.head/document.title are real jsdom singletons shared across the
// whole test binary. Every test wraps core.Head in signal.RunScoped and
// disposes the scope in t.Cleanup. Head's own dispose logic reverts
// exactly what it changed (restores the previous title, removes elements it
// created, resets previous content on ones it only updated), which both
// keeps tests isolated and exercises the cleanup path itself.

import (
	"syscall/js"
	"testing"
	"time"

	"github.com/filipejohansson/vane/core"
	"github.com/filipejohansson/vane/core/signal"
)

func headDoc(t *testing.T) js.Value {
	t.Helper()
	return js.Global().Get("document")
}

func runHeadScoped(t *testing.T, cfg core.HeadConfig) {
	t.Helper()
	scope := signal.RunScoped(func() {
		core.Head(func() core.HeadConfig { return cfg })
	})
	t.Cleanup(scope.Dispose)
}

func TestHeadSetsDocumentTitle(t *testing.T) {
	runHeadScoped(t, core.HeadConfig{Title: "User Detail"})
	if got := headDoc(t).Get("title").String(); got != "User Detail" {
		t.Errorf("document.title = %q, want %q", got, "User Detail")
	}
}

func TestHeadRestoresPreviousTitleOnDispose(t *testing.T) {
	headDoc(t).Set("title", "Original Title")
	t.Cleanup(func() { headDoc(t).Set("title", "") })

	scope := signal.RunScoped(func() {
		core.Head(func() core.HeadConfig { return core.HeadConfig{Title: "Temporary Title"} })
	})
	if got := headDoc(t).Get("title").String(); got != "Temporary Title" {
		t.Fatalf("document.title = %q, want %q", got, "Temporary Title")
	}

	scope.Dispose()

	if got := headDoc(t).Get("title").String(); got != "Original Title" {
		t.Errorf("document.title after dispose = %q, want %q (restored)", got, "Original Title")
	}
}

func TestHeadSetsDescriptionMeta(t *testing.T) {
	runHeadScoped(t, core.HeadConfig{Description: "A test description"})

	el := headDoc(t).Get("head").Call("querySelector", `meta[name="description"]`)
	if !el.Truthy() {
		t.Fatal("no <meta name=\"description\"> found in <head>")
	}
	if got := el.Get("content").String(); got != "A test description" {
		t.Errorf("content = %q, want %q", got, "A test description")
	}
}

// TestHeadSetsOpenGraphTags uses querySelector with an attribute selector
// (meta[property="..."]) rather than checking el.Get("property"), since "property"
// has no IDL reflection on HTMLMetaElement (Open Graph invented it, no spec
// defines it), so only the real HTML attribute set via setAttribute makes
// this selector match. Regression guard for a bug where upsertMetaProp used
// el.Set("property", ...), which silently created an inert JS expando
// property instead of the attribute crawlers actually read.
func TestHeadSetsOpenGraphTags(t *testing.T) {
	runHeadScoped(t, core.HeadConfig{
		OGTitle: "OG Title",
		OGImage: "/img/og.png",
		OGType:  "website",
	})

	head := headDoc(t).Get("head")
	cases := map[string]string{
		"og:title": "OG Title",
		"og:image": "/img/og.png",
		"og:type":  "website",
	}
	for prop, want := range cases {
		el := head.Call("querySelector", `meta[property="`+prop+`"]`)
		if !el.Truthy() {
			t.Errorf("no <meta property=%q> found", prop)
			continue
		}
		if got := el.Get("content").String(); got != want {
			t.Errorf("meta[property=%q].content = %q, want %q", prop, got, want)
		}
	}
}

func TestHeadSetsCanonicalLink(t *testing.T) {
	runHeadScoped(t, core.HeadConfig{Canonical: "https://example.com/page"})

	el := headDoc(t).Get("head").Call("querySelector", `link[rel="canonical"]`)
	if !el.Truthy() {
		t.Fatal("no <link rel=\"canonical\"> found in <head>")
	}
	if got := el.Get("href").String(); got != "https://example.com/page" {
		t.Errorf("href = %q, want %q", got, "https://example.com/page")
	}
}

func TestHeadSetsArbitraryMetaAndLink(t *testing.T) {
	runHeadScoped(t, core.HeadConfig{
		Meta: map[string]string{"author": "vane"},
		Link: map[string]string{"icon": "/favicon.ico"},
	})

	head := headDoc(t).Get("head")
	metaEl := head.Call("querySelector", `meta[name="author"]`)
	if !metaEl.Truthy() || metaEl.Get("content").String() != "vane" {
		t.Error("meta[name=author] not set correctly")
	}
	linkEl := head.Call("querySelector", `link[rel="icon"]`)
	if !linkEl.Truthy() {
		t.Fatal("link[rel=icon] not found")
	}
	// .href reflects the resolved absolute URL (standard DOM behavior for
	// URL-valued attributes), so check the raw attribute for the literal value.
	if got := linkEl.Call("getAttribute", "href").String(); got != "/favicon.ico" {
		t.Errorf("href attribute = %q, want %q", got, "/favicon.ico")
	}
}

func TestHeadRemovesElementsItCreatedOnDispose(t *testing.T) {
	head := headDoc(t).Get("head")
	selector := `meta[name="description"]`

	if head.Call("querySelector", selector).Truthy() {
		t.Fatal("test setup invalid: a description meta already exists in <head>")
	}

	scope := signal.RunScoped(func() {
		core.Head(func() core.HeadConfig { return core.HeadConfig{Description: "temp"} })
	})
	if !head.Call("querySelector", selector).Truthy() {
		t.Fatal("meta[name=description] was not created")
	}

	scope.Dispose()

	if head.Call("querySelector", selector).Truthy() {
		t.Error("meta[name=description] still present after dispose, but Head must remove elements it created, not just clear content")
	}
}

func TestHeadUpsertReusesExistingElementAndRestoresItsContent(t *testing.T) {
	head := headDoc(t).Get("head")
	existing := headDoc(t).Call("createElement", "meta")
	existing.Set("name", "description")
	existing.Set("content", "pre-existing content")
	head.Call("appendChild", existing)
	t.Cleanup(func() { existing.Call("remove") })

	scope := signal.RunScoped(func() {
		core.Head(func() core.HeadConfig { return core.HeadConfig{Description: "overridden"} })
	})

	// Must reuse the existing node (upsert), not create a duplicate.
	matches := head.Call("querySelectorAll", `meta[name="description"]`)
	if got := matches.Get("length").Int(); got != 1 {
		t.Fatalf("found %d meta[name=description] elements, want 1 (must upsert, not duplicate)", got)
	}
	if got := existing.Get("content").String(); got != "overridden" {
		t.Fatalf("content = %q, want %q", got, "overridden")
	}

	scope.Dispose()

	if got := existing.Get("content").String(); got != "pre-existing content" {
		t.Errorf("content after dispose = %q, want %q (must restore, not remove, a pre-existing element)", got, "pre-existing content")
	}
}

// TestHeadLayoutAndChildScopesCoexistWithoutClobbering guards the claim on
// vane-page's Head Management page that a Layout and its child
// route can both call core.Head without stepping on each other: each Head
// call tracks its own restores in its own closure, so disposing one scope
// must not touch what the other scope set.
func TestHeadLayoutAndChildScopesCoexistWithoutClobbering(t *testing.T) {
	originalTitle := headDoc(t).Get("title").String()
	t.Cleanup(func() { headDoc(t).Set("title", originalTitle) })

	layoutScope := signal.RunScoped(func() {
		core.Head(func() core.HeadConfig {
			return core.HeadConfig{OGImage: "/img/og.png", OGType: "website"}
		})
	})
	t.Cleanup(layoutScope.Dispose)

	childScope := signal.RunScoped(func() {
		core.Head(func() core.HeadConfig {
			return core.HeadConfig{Title: "User 42"}
		})
	})

	head := headDoc(t).Get("head")
	if got := headDoc(t).Get("title").String(); got != "User 42" {
		t.Fatalf("document.title = %q, want %q", got, "User 42")
	}
	ogImage := head.Call("querySelector", `meta[property="og:image"]`)
	if !ogImage.Truthy() || ogImage.Get("content").String() != "/img/og.png" {
		t.Fatal("layout's og:image missing before child dispose")
	}

	childScope.Dispose()

	if got := headDoc(t).Get("title").String(); got != originalTitle {
		t.Errorf("document.title after child dispose = %q, want %q (child's own change reverted)", got, originalTitle)
	}
	ogImage = head.Call("querySelector", `meta[property="og:image"]`)
	if !ogImage.Truthy() || ogImage.Get("content").String() != "/img/og.png" {
		t.Error("layout's og:image was removed by disposing the child scope, but Head scopes must be independent")
	}
	ogType := head.Call("querySelector", `meta[property="og:type"]`)
	if !ogType.Truthy() || ogType.Get("content").String() != "website" {
		t.Error("layout's og:type was removed by disposing the child scope, but Head scopes must be independent")
	}
}

func TestHeadReactsToSignalChangeWithoutDuplicatingElements(t *testing.T) {
	title := core.NewSignal("First Title")
	scope := signal.RunScoped(func() {
		core.Head(func() core.HeadConfig { return core.HeadConfig{Title: title.Get()} })
	})
	t.Cleanup(scope.Dispose)

	if got := headDoc(t).Get("title").String(); got != "First Title" {
		t.Fatalf("document.title = %q, want %q", got, "First Title")
	}

	title.Set("Second Title")
	if !signal.WaitEffects(time.Second) {
		t.Fatal("effects did not settle within timeout")
	}

	if got := headDoc(t).Get("title").String(); got != "Second Title" {
		t.Errorf("document.title after signal change = %q, want %q", got, "Second Title")
	}
}
