package compiler_test

import (
	"strings"
	"testing"

	"github.com/filipejohansson/vane/internal/compiler"
)

// wrap puts a vane syntax snippet inside a minimal .vane function body.
func wrap(jsx string) string {
	return "package main\nimport \"syscall/js\"\nfunc F() js.Value {\n\treturn (\n\t\t" + jsx + "\n\t)\n}"
}

// compile returns the compiled output with header and //line directives stripped.
func compile(t *testing.T, src string) string {
	t.Helper()
	out, err := compiler.Compile(src, "test.vane")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	return stripNoise(out)
}

func stripNoise(out string) string {
	lines := strings.Split(out, "\n")
	var result []string
	for _, line := range lines {
		tr := strings.TrimSpace(line)
		if strings.HasPrefix(tr, "// Code generated") ||
			strings.HasPrefix(tr, "//go:build") ||
			strings.HasPrefix(tr, "//line") {
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

func has(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("output missing %q\n\ngot:\n%s", want, got)
	}
}

func hasNot(t *testing.T, got, unwanted string) {
	t.Helper()
	if strings.Contains(got, unwanted) {
		t.Errorf("output should not contain %q\n\ngot:\n%s", unwanted, got)
	}
}

func wantErr(t *testing.T, src string) {
	t.Helper()
	_, err := compiler.Compile(src, "test.vane")
	if err == nil {
		t.Error("expected compile error, got nil")
	}
}

//* Element creation

func TestSimpleElement(t *testing.T) {
	out := compile(t, wrap(`<div></div>`))
	has(t, out, `core.El("div")`)
}

func TestSelfClosingElement(t *testing.T) {
	out := compile(t, wrap(`<br/>`))
	has(t, out, `core.El("br")`)
}

func TestVoidElement(t *testing.T) {
	out := compile(t, wrap(`<input type="text"/>`))
	has(t, out, `core.El("input")`)
	has(t, out, `core.SetProp(_vane1, "type", "text")`)
}

//* Static props

func TestStaticStringProp(t *testing.T) {
	out := compile(t, wrap(`<div className="card"></div>`))
	has(t, out, `core.SetProp(_vane1, "className", "card")`)
}

func TestMultipleStaticProps(t *testing.T) {
	out := compile(t, wrap(`<input type="text" placeholder="Search"/>`))
	has(t, out, `core.SetProp(_vane1, "type", "text")`)
	has(t, out, `core.SetProp(_vane1, "placeholder", "Search")`)
}

func TestBooleanPropNoValue(t *testing.T) {
	// Bare boolean attribute: <input disabled/> → treated as {true} expression
	out := compile(t, wrap(`<input disabled/>`))
	has(t, out, `core.DynProp(_vane1, "disabled", func() any { return true })`)
}

//* Dynamic props

func TestDynamicProp(t *testing.T) {
	out := compile(t, wrap(`<div className={cls}></div>`))
	has(t, out, `core.DynProp(_vane1, "className", func() any { return cls })`)
}

func TestDynamicValueProp(t *testing.T) {
	out := compile(t, wrap(`<input value={v}/>`))
	has(t, out, `core.DynProp(_vane1, "value", func() any { return v })`)
}

func TestDynamicDisabledProp(t *testing.T) {
	out := compile(t, wrap(`<button disabled={loading}></button>`))
	has(t, out, `core.DynProp(_vane1, "disabled", func() any { return loading })`)
}

//* Accessibility attributes
//
// aria-*/data-* and role aren't special-cased by the compiler. They flow
// through the same generic attribute path as any other prop (core.SetProp /
// core.DynProp), which is what lets the attribute-name lexer's support for
// hyphens (readAttrName accepts '-') do all the work. These tests just guard
// that the hyphenated attribute names actually parse. A plain audit of
// core.SetProp's runtime behavior (data-/aria- routing to setAttribute,
// role/tabIndex reflecting correctly) lives in core/props_dom_test.go.

func TestAriaLabelStaticAttr(t *testing.T) {
	out := compile(t, wrap(`<button aria-label="Close"></button>`))
	has(t, out, `core.SetProp(_vane1, "aria-label", "Close")`)
}

func TestAriaExpandedDynamicAttr(t *testing.T) {
	out := compile(t, wrap(`<button aria-expanded={open}></button>`))
	has(t, out, `core.DynProp(_vane1, "aria-expanded", func() any { return open })`)
}

func TestAriaLabelledbyMultiHyphenAttrName(t *testing.T) {
	// Multiple hyphens in the attribute name (aria-labelledby, not just
	// aria-label) must lex correctly too.
	out := compile(t, wrap(`<div aria-labelledby="dialog-title"></div>`))
	has(t, out, `core.SetProp(_vane1, "aria-labelledby", "dialog-title")`)
}

func TestRoleStaticAttr(t *testing.T) {
	out := compile(t, wrap(`<div role="dialog"></div>`))
	has(t, out, `core.SetProp(_vane1, "role", "dialog")`)
}

func TestTabIndexDynamicAttr(t *testing.T) {
	out := compile(t, wrap(`<div tabIndex={idx}></div>`))
	has(t, out, `core.DynProp(_vane1, "tabIndex", func() any { return idx })`)
}

//* Key

func TestKeyExpr(t *testing.T) {
	out := compile(t, wrap(`<li key={item.ID}></li>`))
	has(t, out, `core.Unwrap(_vane1).Set("key", item.ID)`)
}

func TestKeyStaticString(t *testing.T) {
	out := compile(t, wrap(`<li key="static"></li>`))
	has(t, out, `core.Unwrap(_vane1).Set("key", "static")`)
}

//* Ref

func TestRef(t *testing.T) {
	out := compile(t, wrap(`<input ref={&myEl}/>`))
	has(t, out, `*(&myEl) = _vane1`)
}

func TestRefCallbackForm(t *testing.T) {
	// ref={func(el core.Node) {...}} is called synchronously with the
	// element, right when it's constructed. An alternative to the pointer
	// form, for code that needs to run at that exact moment.
	out := compile(t, wrap(`<div ref={func(el core.Node) { core.Focus(el) }}></div>`))
	has(t, out, `(func(el core.Node) { core.Focus(el) })(_vane1)`)
	hasNot(t, out, `*(func`)
}

func TestRefPointerFormStillWorksAlongsideCallbackForm(t *testing.T) {
	// Guards the strings.HasPrefix(value, "&") dispatch, so a leading '&'
	// must still route to pointer-assignment, not get misdetected as a callback.
	out := compile(t, wrap(`<input ref={&myEl}/>`))
	has(t, out, `*(&myEl) = _vane1`)
	hasNot(t, out, `(&myEl)(_vane1)`)
}

//* Style

func TestStyleStaticString(t *testing.T) {
	out := compile(t, wrap(`<div style="color:red"></div>`))
	has(t, out, `.Get("style").Set("cssText", "color:red")`)
}

func TestStyleStructExpr(t *testing.T) {
	out := compile(t, wrap(`<div style={core.Style{Color:"red"}}></div>`))
	has(t, out, `core.DynStyle(_vane1, func() core.Style { return core.Style{Color:"red"} })`)
}

//* Event handlers

func TestOnClick(t *testing.T) {
	out := compile(t, wrap(`<button onClick={fn}></button>`))
	has(t, out, `core.OnClick(_vane1, fn)`)
}

func TestOnInput(t *testing.T) {
	out := compile(t, wrap(`<input onInput={fn}/>`))
	has(t, out, `core.OnInput(_vane1, fn)`)
}

func TestOnChange(t *testing.T) {
	out := compile(t, wrap(`<select onChange={fn}></select>`))
	has(t, out, `core.OnChange(_vane1, fn)`)
}

func TestOnChecked(t *testing.T) {
	out := compile(t, wrap(`<input onChecked={fn}/>`))
	has(t, out, `core.OnChecked(_vane1, fn)`)
}

func TestOnSubmit(t *testing.T) {
	out := compile(t, wrap(`<form onSubmit={fn}></form>`))
	has(t, out, `core.OnSubmit(_vane1, fn)`)
}

func TestOnKeyDown(t *testing.T) {
	out := compile(t, wrap(`<input onKeyDown={fn}/>`))
	has(t, out, `core.OnKeyDown(_vane1, fn)`)
}

func TestOnKeyUp(t *testing.T) {
	out := compile(t, wrap(`<input onKeyUp={fn}/>`))
	has(t, out, `core.OnKeyUp(_vane1, fn)`)
}

func TestOnBlur(t *testing.T) {
	out := compile(t, wrap(`<input onBlur={fn}/>`))
	has(t, out, `core.OnBlur(_vane1, fn)`)
}

func TestOnFocus(t *testing.T) {
	out := compile(t, wrap(`<input onFocus={fn}/>`))
	has(t, out, `core.OnFocus(_vane1, fn)`)
}

func TestOnDblClick(t *testing.T) {
	out := compile(t, wrap(`<div onDblClick={fn}></div>`))
	has(t, out, `core.OnDblClick(_vane1, fn)`)
}

func TestOnMouseEnter(t *testing.T) {
	out := compile(t, wrap(`<div onMouseEnter={fn}></div>`))
	has(t, out, `core.OnMouseEnter(_vane1, fn)`)
}

func TestOnMouseLeave(t *testing.T) {
	out := compile(t, wrap(`<div onMouseLeave={fn}></div>`))
	has(t, out, `core.OnMouseLeave(_vane1, fn)`)
}

func TestOnScroll(t *testing.T) {
	out := compile(t, wrap(`<div onScroll={fn}></div>`))
	has(t, out, `core.OnScroll(_vane1, fn)`)
}

func TestOnPointerDown(t *testing.T) {
	out := compile(t, wrap(`<div onPointerDown={fn}></div>`))
	has(t, out, `core.OnPointerDown(_vane1, fn)`)
}

func TestOnPointerUp(t *testing.T) {
	out := compile(t, wrap(`<div onPointerUp={fn}></div>`))
	has(t, out, `core.OnPointerUp(_vane1, fn)`)
}

func TestOnPointerMove(t *testing.T) {
	out := compile(t, wrap(`<div onPointerMove={fn}></div>`))
	has(t, out, `core.OnPointerMove(_vane1, fn)`)
}

func TestOnTouchStart(t *testing.T) {
	out := compile(t, wrap(`<div onTouchStart={fn}></div>`))
	has(t, out, `core.OnTouchStart(_vane1, fn)`)
}

func TestOnTouchEnd(t *testing.T) {
	out := compile(t, wrap(`<div onTouchEnd={fn}></div>`))
	has(t, out, `core.OnTouchEnd(_vane1, fn)`)
}

func TestOnDragStart(t *testing.T) {
	out := compile(t, wrap(`<div onDragStart={fn}></div>`))
	has(t, out, `core.OnDragStart(_vane1, fn)`)
}

func TestOnDrop(t *testing.T) {
	out := compile(t, wrap(`<div onDrop={fn}></div>`))
	has(t, out, `core.OnDrop(_vane1, fn)`)
}

func TestUnknownEventAttrFails(t *testing.T) {
	// onclick instead of onClick should fail with a helpful error
	wantErr(t, wrap(`<div onclick={fn}></div>`))
}

func TestDivergentAttrNameFails(t *testing.T) {
	// class instead of className should fail with a helpful error
	wantErr(t, wrap(`<div class="card"></div>`))
}

//* Children

func TestStaticTextChild(t *testing.T) {
	out := compile(t, wrap(`<p>Hello</p>`))
	has(t, out, `core.AppendText(_vane1, "Hello")`)
}

func TestExprChild(t *testing.T) {
	out := compile(t, wrap(`<p>{count.Get()}</p>`))
	has(t, out, `core.DynChild(_vane1, func() any { return count.Get() })`)
}

func TestSpreadChild(t *testing.T) {
	out := compile(t, wrap(`<ul>{items()...}</ul>`))
	has(t, out, `core.DynList(_vane1, func() []core.Node { return items() })`)
}

func TestSpreadChildSlice(t *testing.T) {
	// []core.Node / []any / variadic children spread uses Spread+Fragment, not DynList
	out := compile(t, wrap(`<div>{children...}</div>`))
	has(t, out, `core.AppendChild(_vane1, core.Fragment(core.Spread(children)...))`)
	hasNot(t, out, `DynList`)
}

func TestNestedElements(t *testing.T) {
	out := compile(t, wrap(`<div><span>Hi</span></div>`))
	has(t, out, `core.El("div")`)
	has(t, out, `core.El("span")`)
	has(t, out, `core.AppendText`)
	has(t, out, `core.AppendChild`)
}

func TestMultipleChildren(t *testing.T) {
	out := compile(t, wrap(`<div><p>A</p><p>B</p></div>`))
	// Both paragraphs appended to the same parent
	count := strings.Count(out, `core.El("p")`)
	if count != 2 {
		t.Errorf("want 2 <p> elements, got %d", count)
	}
}

func TestMixedChildren(t *testing.T) {
	out := compile(t, wrap(`<div>Label: {value}</div>`))
	has(t, out, `core.AppendText(_vane1, "Label: ")`)
	has(t, out, `core.DynChild(_vane1, func() any { return value })`)
}

//* Fragment

func TestFragmentAtRoot(t *testing.T) {
	out := compile(t, wrap(`<><p>A</p><p>B</p></>`))
	has(t, out, `core.Fragment(`)
}

func TestFragmentNested(t *testing.T) {
	out := compile(t, wrap(`<div><><span>A</span><span>B</span></></div>`))
	// Fragment children appended directly to parent, no Fragment() call needed
	has(t, out, `core.El("span")`)
}

func TestFragmentWithExprChildren(t *testing.T) {
	out := compile(t, wrap(`<>{Foo()}{Bar()}</>`))
	// Root fragment with expr children must NOT emit core.DynChild(, ...)
	hasNot(t, out, `core.DynChild(,`)
	has(t, out, `Foo()`)
	has(t, out, `Bar()`)
	has(t, out, `core.Fragment(`)
}

//* Component calls

func TestUppercaseTagIsCompileError(t *testing.T) {
	// Components are Go function calls, not vane syntax tags: {Counter()}.
	msg := compileErr(t, wrap(`<div><Counter/></div>`))
	has(t, msg, "not valid vane syntax")
	has(t, msg, "{Counter()}")
}

//* Inline control flow

func TestForLoop(t *testing.T) {
	src := wrap(`<ul>{for _, x := range items { <li>{x}</li> }}</ul>`)
	out := compile(t, src)
	has(t, out, `core.DynList(_vane1, func() []core.Node {`)
	has(t, out, `for _, x := range items {`)
	has(t, out, `core.El("li")`)
	has(t, out, `_vaneItems`)
	has(t, out, `return _vaneItems`)
}

func TestForLoopWithSetup(t *testing.T) {
	src := wrap(`<ul>{for i, x := range items { cls := "a"; if x == "" { cls = "b" }; <li className={cls}>{x}</li> }}</ul>`)
	out := compile(t, src)
	has(t, out, `for i, x := range items {`)
	has(t, out, `cls := "a"`)
	has(t, out, `core.El("li")`)
	has(t, out, `core.DynProp`)
}

func TestIfNoElse(t *testing.T) {
	src := wrap(`<div>{if show { <p>Hi</p> }}</div>`)
	out := compile(t, src)
	has(t, out, `core.DynChild(_vane1, func() any {`)
	has(t, out, `if show {`)
	has(t, out, `core.El("p")`)
	has(t, out, `return core.Empty()`)
}

func TestIfInitStatement(t *testing.T) {
	src := wrap(`<div>{if msg := getMessage(); msg != "" { <p>{msg}</p> }}</div>`)
	out := compile(t, src)
	has(t, out, `if msg := getMessage(); msg != "" {`)
	has(t, out, `core.El("p")`)
	has(t, out, `return core.Empty()`)
}

func TestIfElse(t *testing.T) {
	src := wrap(`<div>{if ok { <p>Yes</p> } else { <p>No</p> }}</div>`)
	out := compile(t, src)
	has(t, out, `if ok {`)
	has(t, out, `} else {`)
	has(t, out, `return core.Empty()`)
}

func TestElseIf(t *testing.T) {
	src := wrap(`<div>{if a { <p>A</p> } else if b { <p>B</p> } else { <p>C</p> }}</div>`)
	out := compile(t, src)
	has(t, out, `if a {`)
	has(t, out, `} else if b {`)
	has(t, out, `} else {`)
}

func TestSwitchWithCases(t *testing.T) {
	src := wrap(`<div>{switch s { case "a": <p>A</p> default: <p>D</p> }}</div>`)
	out := compile(t, src)
	has(t, out, `switch s {`)
	has(t, out, `case "a":`)
	has(t, out, `default:`)
	has(t, out, `return core.Empty()`)
}

func TestForBodyCallExpr(t *testing.T) {
	// Body ends with a component call instead of a vane syntax element.
	src := wrap(`<ul>{for _, x := range items { makeItem(x) }}</ul>`)
	out := compile(t, src)
	has(t, out, `append(`)
	has(t, out, `makeItem(x)`)
}

func TestForBodyCallExprWithSetup(t *testing.T) {
	// Setup code on its own line before the call expression.
	src := "package main\nimport \"syscall/js\"\nfunc F() js.Value {\n\treturn (\n\t\t<ul>{for i, x := range items {\n\t\t\tidx := i\n\t\t\trenderRow(x, idx)\n\t\t}}</ul>\n\t)\n}"
	out := compile(t, src)
	has(t, out, `idx := i`)
	has(t, out, `append(`)
	has(t, out, `renderRow(x, idx)`)
}

func TestForBodyMultilineCallExpr(t *testing.T) {
	// Multi-line call (like About.vane faqItem).
	src := "package main\nimport \"syscall/js\"\nfunc F() js.Value {\n\treturn (\n\t\t<ul>{for i, s := range sections {\n\t\t\tidx := i\n\t\t\tfaqItem(s, idx, openIdx == idx, func() {\n\t\t\t\tif openIdx == idx {\n\t\t\t\t\topenIdx = -1\n\t\t\t\t} else {\n\t\t\t\t\topenIdx = idx\n\t\t\t\t}\n\t\t\t})\n\t\t}}</ul>\n\t)\n}"
	out := compile(t, src)
	has(t, out, `idx := i`)
	has(t, out, `append(`)
	has(t, out, `faqItem(`)
}

func TestIfBodyCallExpr(t *testing.T) {
	src := wrap(`<div>{if show { renderCard(data) }}</div>`)
	out := compile(t, src)
	has(t, out, `return renderCard(data)`)
}

func TestSwitchBodyCallExpr(t *testing.T) {
	src := wrap(`<div>{switch kind { case "a": renderA(x) default: renderDefault(x) }}</div>`)
	out := compile(t, src)
	has(t, out, `return renderA(x)`)
	has(t, out, `return renderDefault(x)`)
}

//* Named closures with vane syntax return (defined before vane syntax return)

func TestNamedClosureWithVaneReturn(t *testing.T) {
	src := `package main
import "syscall/js"
func F() js.Value {
	body := func() js.Value {
		if ok {
			return (<span>Yes</span>)
		}
		return (<span>No</span>)
	}
	return (<div>{body()}</div>)
}`
	out := compile(t, src)
	has(t, out, `core.El("span")`)
	has(t, out, `body()`)
}

//* Error cases

func TestMissingClosingTag(t *testing.T) {
	wantErr(t, wrap(`<div><p>unclosed</div>`))
}

func TestWrongClosingTag(t *testing.T) {
	wantErr(t, wrap(`<div></span>`))
}

//* Import injection

func TestSyscallJSInjectedWhenMissing(t *testing.T) {
	src := "package main\nfunc F() js.Value {\n\treturn (<div></div>)\n}"
	out, err := compiler.Compile(src, "test.vane")
	if err != nil {
		t.Fatal(err)
	}
	has(t, out, `"syscall/js"`)
}

func TestSyscallJSNotDuplicated(t *testing.T) {
	src := "package main\nimport \"syscall/js\"\nfunc F() js.Value { return (<div></div>) }"
	out, err := compiler.Compile(src, "test.vane")
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(out, `"syscall/js"`); count != 1 {
		t.Errorf(`"syscall/js" appears %d times, want 1`, count)
	}
}

//* Build tag

func TestBuildTagInjected(t *testing.T) {
	src := "package main\nfunc F() js.Value { return (<div></div>) }"
	out, err := compiler.Compile(src, "test.vane")
	if err != nil {
		t.Fatal(err)
	}
	has(t, out, "//go:build js && wasm")
}

func TestBuildTagNotDuplicatedWhenPresent(t *testing.T) {
	src := "//go:build js && wasm\npackage main\nfunc F() js.Value { return (<div></div>) }"
	out, err := compiler.Compile(src, "test.vane")
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(out, "//go:build js && wasm"); count != 1 {
		t.Errorf("build tag appears %d times, want 1", count)
	}
}

//* Error message format

func compileErr(t *testing.T, src string) string {
	t.Helper()
	_, err := compiler.Compile(src, "test.vane")
	if err == nil {
		t.Fatal("expected compile error, got nil")
	}
	return err.Error()
}

func TestErrorIncludesFileLineCol(t *testing.T) {
	// Wrong closing tag: <p> closed with </span>
	src := "package main\nimport \"syscall/js\"\nfunc F() js.Value {\n\treturn (\n\t\t<div><p>Hi</span></div>\n\t)\n}"
	msg := compileErr(t, src)
	has(t, msg, "test.vane:") // file
	has(t, msg, "expected </p>")
}

func TestErrorShowsSourceContext(t *testing.T) {
	src := "package main\nimport \"syscall/js\"\nfunc F() js.Value {\n\treturn (\n\t\t<div><p>Hi</span></div>\n\t)\n}"
	msg := compileErr(t, src)
	has(t, msg, "|") // context lines use | separator
}

func TestErrorShowsCaret(t *testing.T) {
	src := "package main\nimport \"syscall/js\"\nfunc F() js.Value {\n\treturn (\n\t\t<div><p>Hi</span></div>\n\t)\n}"
	msg := compileErr(t, src)
	has(t, msg, "^") // caret points at column
}

func TestErrorWrongClosingTagHint(t *testing.T) {
	src := "package main\nimport \"syscall/js\"\nfunc F() js.Value {\n\treturn (\n\t\t<div></span>\n\t)\n}"
	msg := compileErr(t, src)
	has(t, msg, "expected </div>")
	has(t, msg, "Hint:")
}

func TestErrorVaneOutsideReturn(t *testing.T) {
	src := "package main\nimport \"syscall/js\"\nfunc F() js.Value {\n\tv := (<p>Hi</p>)\n\treturn v\n}"
	msg := compileErr(t, src)
	has(t, msg, "vane syntax element outside return")
	has(t, msg, "return ( ... )")
}

func TestErrorUnknownEventAttrSuggestsCorrection(t *testing.T) {
	msg := compileErr(t, wrap(`<div onclick={fn}></div>`))
	has(t, msg, `"onclick"`)
	has(t, msg, "onClick")
}

func TestErrorUnknownEventAttrHasPosition(t *testing.T) {
	msg := compileErr(t, wrap(`<div onclick={fn}></div>`))
	has(t, msg, "test.vane:")
	has(t, msg, "^")
}

func TestErrorDivergentAttrNameSuggestsCorrection(t *testing.T) {
	msg := compileErr(t, wrap(`<div class="card"></div>`))
	has(t, msg, `"class"`)
	has(t, msg, "className")
}

func TestErrorDivergentAttrNameHtmlFor(t *testing.T) {
	msg := compileErr(t, wrap(`<label for="email"></label>`))
	has(t, msg, `"for"`)
	has(t, msg, "htmlFor")
}

func TestErrorUnterminatedElement(t *testing.T) {
	src := "package main\nimport \"syscall/js\"\nfunc F() js.Value {\n\treturn (<div><p>unclosed</div>)\n}"
	msg := compileErr(t, src)
	has(t, msg, "expected </p>")
	has(t, msg, "Hint:")
}

//* Non-vane passthrough

func TestNonVanePassthrough(t *testing.T) {
	src := "package main\nfunc add(a, b int) int { return a + b }"
	out, err := compiler.Compile(src, "test.vane")
	if err != nil {
		t.Fatal(err)
	}
	has(t, out, "func add(a, b int) int { return a + b }")
}

func TestStringLiteralsUntouched(t *testing.T) {
	// String containing vane-like text should not be compiled
	src := "package main\nfunc F() string { return \"<div>not jsx</div>\" }"
	out, err := compiler.Compile(src, "test.vane")
	if err != nil {
		t.Fatal(err)
	}
	has(t, out, `"<div>not jsx</div>"`)
}

//* return nil sugar

func TestReturnNilRewrittenToJsValue(t *testing.T) {
	src := "package main\nimport \"syscall/js\"\nfunc F() js.Value {\n\tif cond {\n\t\treturn nil\n\t}\n\treturn (\n\t\t<div></div>\n\t)\n}"
	out := compile(t, src)
	has(t, out, "return js.Undefined()")
	hasNot(t, out, "return nil")
}

func TestReturnNilInsideFuncFuncClosureNotRewritten(t *testing.T) {
	// return nil inside func() func() (e.g. Effect cleanup) must NOT be rewritten
	src := "package main\nimport \"syscall/js\"\nfunc F() js.Value {\n\tcore.Effect(func() func() {\n\t\treturn nil\n\t})\n\treturn (\n\t\t<div></div>\n\t)\n}"
	out := compile(t, src)
	has(t, out, "return nil") // func() func() cleanup preserved
}

func TestReturnNilInsideJsValueClosureRewritten(t *testing.T) {
	// return nil inside a var-assigned func() js.Value IS rewritten
	src := "package main\nimport \"syscall/js\"\nfunc F() js.Value {\n\tnode := func() js.Value {\n\t\treturn nil\n\t}\n\t_ = node\n\treturn (\n\t\t<div></div>\n\t)\n}"
	out := compile(t, src)
	has(t, out, "return js.Undefined()")
}

func TestReturnNilRewrittenToCoreEmpty(t *testing.T) {
	// core.Node is the current vane convention (js.Value is the legacy path
	// covered by TestReturnNilRewrittenToJsValue), so return nil must rewrite
	// to core.Empty(), not js.Undefined().
	src := "package main\nfunc F() core.Node {\n\tif cond {\n\t\treturn nil\n\t}\n\treturn (\n\t\t<div></div>\n\t)\n}"
	out := compile(t, src)
	has(t, out, "return core.Empty()")
	hasNot(t, out, "return nil")
	hasNot(t, out, "js.Undefined()")
}

func TestReturnNilInsideCoreNodeClosureRewritten(t *testing.T) {
	// return nil inside a var-assigned func() core.Node IS rewritten
	src := "package main\nfunc F() core.Node {\n\tnode := func() core.Node {\n\t\treturn nil\n\t}\n\t_ = node\n\treturn (\n\t\t<div></div>\n\t)\n}"
	out := compile(t, src)
	has(t, out, "return core.Empty()")
}

func TestReturnNilInsideNonElementClosureNotRewritten(t *testing.T) {
	// return nil inside a func that returns neither core.Node nor js.Value
	// (e.g. an error-returning helper) must NOT be rewritten.
	src := "package main\nfunc F() core.Node {\n\thelper := func() error {\n\t\treturn nil\n\t}\n\t_ = helper\n\treturn (\n\t\t<div></div>\n\t)\n}"
	out := compile(t, src)
	has(t, out, "func() error {\n\t\treturn nil\n\t}")
}

//* Inline func literals in vane syntax children

func TestInlineIIFEWithVane(t *testing.T) {
	// {func() js.Value { return (<p>...</p>) }()} inside vane syntax children must compile
	src := `package main
import "syscall/js"
import "github.com/filipejohansson/vane/core"
func F() js.Value {
	name := core.NewSignal("")
	return (
		<div>
			{func() js.Value {
				if n := name.Get(); n != "" {
					return (<p className="greeting">Hello!</p>)
				}
				return nil
			}()}
		</div>
	)
}`
	out := compile(t, src)
	has(t, out, `core.El("p")`)
	has(t, out, `core.SetProp`)
	has(t, out, `return js.Undefined()`)
}

func TestInlineIIFEReturnNilNotRewrittenInNonJsValueNested(t *testing.T) {
	// return nil inside a non-js.Value nested func inside the IIFE must NOT be rewritten
	src := `package main
import "syscall/js"
import "github.com/filipejohansson/vane/core"
func F() js.Value {
	return (
		<div>
			{func() js.Value {
				_ = func() { return }
				return (<span>x</span>)
			}()}
		</div>
	)
}`
	out := compile(t, src)
	has(t, out, `core.El("span")`)
}

func TestBareUncalledFuncLiteralErrors(t *testing.T) {
	src := `package main
import "syscall/js"
func F() js.Value {
	return (
		<div>
			{func() js.Value {
				return (<p>hello</p>)
			}}
		</div>
	)
}`
	_, err := compiler.Compile(src, "test.vane")
	if err == nil {
		t.Fatal("expected compile error for bare uncalled func literal, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "never called") {
		t.Errorf("error should mention 'never called', got: %s", msg)
	}
	if !strings.Contains(msg, "()") {
		t.Errorf("error should hint at adding '()', got: %s", msg)
	}
}

func TestInlineFuncLiteralAsArgument(t *testing.T) {
	// func literal passed as argument inside vane syntax child expression, now works
	src := `package main
import "syscall/js"
import "github.com/filipejohansson/vane/core"
import "github.com/filipejohansson/vane/core/router"
func F() js.Value {
	return (
		<nav>
			{router.ActiveLink("/", func(active bool) js.Value {
				cls := "nav-link"
				if active { cls = "nav-link active" }
				return (<a href="#/" className={cls}>Home</a>)
			})}
		</nav>
	)
}`
	out := compile(t, src)
	has(t, out, `core.El("a")`)
	has(t, out, `cls`)
}

func TestReturnInForBlockErrors(t *testing.T) {
	src := `package main
import "syscall/js"
import "github.com/filipejohansson/vane/core"
func F() js.Value {
	items := []string{"a", "b"}
	return (
		<ul>
			{for _, item := range items {
				return (<li>{item}</li>)
			}}
		</ul>
	)
}`
	_, err := compiler.Compile(src, "test.vane")
	if err == nil {
		t.Fatal("expected compile error for return inside {for}, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "return") || !strings.Contains(msg, "for") {
		t.Errorf("error should mention 'return' and 'for', got: %s", msg)
	}
}

func TestReturnInIfBlockErrors(t *testing.T) {
	src := `package main
import "syscall/js"
import "github.com/filipejohansson/vane/core"
func F() js.Value {
	show := true
	return (
		<div>
			{if show {
				return (<p>hello</p>)
			}}
		</div>
	)
}`
	_, err := compiler.Compile(src, "test.vane")
	if err == nil {
		t.Fatal("expected compile error for return inside {if}, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "return") || !strings.Contains(msg, "if") {
		t.Errorf("error should mention 'return' and 'if', got: %s", msg)
	}
}

func TestUnclosedTagErrorPointsToOpenTag(t *testing.T) {
	// File ends without closing <span>, so the error should point to the opening tag, not EOF
	src := "package main\nimport \"syscall/js\"\nfunc F() js.Value {\n\treturn (\n\t\t<div>\n\t\t\t<span>hello\n"
	_, err := compiler.Compile(src, "test.vane")
	if err == nil {
		t.Fatal("expected compile error for unclosed tag, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "span") {
		t.Errorf("error should mention the unclosed tag 'span', got: %s", msg)
	}
	// Error must point to line 6 (the opening <span>), not beyond
	if !strings.Contains(msg, "test.vane:6:") {
		t.Errorf("error should point to line 6 (the opening <span>), got: %s", msg)
	}
}

func TestComparisonLtAfterNewlineHint(t *testing.T) {
	src := `package main
import "syscall/js"
func F() js.Value {
	x := 5
	return (
		<div>
			{for i := 0; i < 3; i++ {
				x
				<notATag.field
			}}
		</div>
	)
}`
	_, err := compiler.Compile(src, "test.vane")
	if err == nil {
		t.Fatal("expected compile error for invalid tag, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "comparison") {
		t.Errorf("error should mention comparison operator hint, got: %s", msg)
	}
}

func TestCtrlFlowBodyLineDirectives(t *testing.T) {
	// Verify that //line directives are emitted inside {for} and {if} bodies
	// so Go type errors inside ctrl flow blocks map to the correct .vane line.
	src := `package main
import "syscall/js"
func F(items []string) js.Value {
	return (
		<div>
			{for _, item := range items {
				<li>{item}</li>
			}}
			{if len(items) > 0 {
				<span>has items</span>
			}}
		</div>
	)
}`
	out, err := compiler.Compile(src, "test.vane")
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	// Generated code must contain //line directives inside the DynList and DynChild closures
	// pointing to the .vane file. Check that "test.vane:7" (the <li> line) appears.
	if !strings.Contains(out, "test.vane:7") {
		t.Errorf("expected //line directive pointing to line 7 (<li> inside {for}), output:\n%s", out)
	}
	// Check that "test.vane:10" (the <span> line) appears.
	if !strings.Contains(out, "test.vane:10") {
		t.Errorf("expected //line directive pointing to line 10 (<span> inside {if}), output:\n%s", out)
	}
}

func TestLineCommentsInJSXChildrenIgnored(t *testing.T) {
	src := `package main
import "syscall/js"
func F() js.Value {
	return (
		<div>
			// just a comment
			<span>hello</span>
		</div>
	)
}`
	out := compile(t, src)
	if strings.Contains(out, "// just a comment") {
		t.Errorf("line comment should be stripped from JSX children, got:\n%s", out)
	}
}

func TestBlockCommentsInJSXChildrenIgnored(t *testing.T) {
	src := `package main
import "syscall/js"
func F() js.Value {
	return (
		<div>
			/* Desktop Nav */
			<nav>content</nav>
		</div>
	)
}`
	out := compile(t, src)
	if strings.Contains(out, "Desktop Nav") {
		t.Errorf("block comment should be stripped from JSX children, got:\n%s", out)
	}
}

func TestMultilineBlockCommentsInJSXChildrenIgnored(t *testing.T) {
	src := `package main
import "syscall/js"
func F() js.Value {
	return (
		<div>
			/*
			 * Desktop Nav
			 * second line
			 */
			<nav>content</nav>
		</div>
	)
}`
	out := compile(t, src)
	if strings.Contains(out, "Desktop Nav") {
		t.Errorf("multiline block comment should be stripped from JSX children, got:\n%s", out)
	}
	if strings.Contains(out, "second line") {
		t.Errorf("multiline block comment body should be stripped from JSX children, got:\n%s", out)
	}
}

func TestDoubleSlashInURLTextNotTreatedAsComment(t *testing.T) {
	src := `package main
import "syscall/js"
func F() js.Value {
	return (<code>http://localhost:8080</code>)
}`
	out := compile(t, src)
	if !strings.Contains(out, `http://localhost:8080`) {
		t.Errorf("// inside a run of text should not be treated as a comment, got:\n%s", out)
	}
}

func TestBlockCommentMarkersInProseTextNotStripped(t *testing.T) {
	src := `package main
import "syscall/js"
func F() js.Value {
	return (<p>50% /* not a real comment */ done</p>)
}`
	out := compile(t, src)
	if !strings.Contains(out, `50% /* not a real comment */ done`) {
		t.Errorf("/* */ inside a run of text should not be stripped, got:\n%s", out)
	}
}
