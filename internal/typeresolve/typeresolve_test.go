package typeresolve_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/filipejohansson/vane/internal/typeresolve"
	"golang.org/x/tools/go/packages"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

// loadAndResolve compiles a temp module's main.go, loads it, and returns
// every RangeVarType found in it.
func loadAndResolve(t *testing.T, dir string) []typeresolve.RangeVarType {
	t.Helper()
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax |
			packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports,
		Dir: dir,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatalf("package(s) failed to load/typecheck")
	}
	pkg := pkgs[0]
	var all []typeresolve.RangeVarType
	for _, f := range pkg.Syntax {
		all = append(all, typeresolve.RangeVarTypesInFile(pkg, f)...)
	}
	return all
}

func soleType(t *testing.T, results []typeresolve.RangeVarType) string {
	t.Helper()
	if len(results) != 1 {
		t.Fatalf("got %d range var types, want exactly 1: %+v", len(results), results)
	}
	return results[0].Type
}

func TestBaseline_LocalStruct(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module fixture\n\ngo 1.25.0\n")
	writeFile(t, dir, "main.go", `package main

type ToDo struct {
	ID   string
	Text string
}

func F(todos []ToDo) {
	for _, t := range todos {
		_ = t
	}
}
`)
	if got := soleType(t, loadAndResolve(t, dir)); got != "ToDo" {
		t.Fatalf("got %q, want ToDo (same package: no qualifier)", got)
	}
}

func TestAliasVsDefinedType(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module fixture\n\ngo 1.25.0\n")
	writeFile(t, dir, "main.go", `package main

type ToDo struct{ ID string }
type Alias = ToDo // true alias
type Named ToDo    // defined (distinct) type

func F(aliased []Alias, named []Named) {
	for _, a := range aliased {
		_ = a
	}
	for _, n := range named {
		_ = n
	}
}
`)
	results := loadAndResolve(t, dir)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2: %+v", len(results), results)
	}
	// A true alias is its own *types.Alias node under this Go version's
	// "materialized aliases" - it prints as its own name, not the aliased
	// target's. Not a defect: an alias and its target are interchangeable at
	// every call site, so emitting either name compiles fine either way.
	if results[0].Type != "Alias" {
		t.Fatalf("alias: got %q, want Alias", results[0].Type)
	}
	if results[1].Type != "Named" {
		t.Fatalf("named: got %q, want Named (defined types are distinct)", results[1].Type)
	}
}

func TestGenericElementType(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module fixture\n\ngo 1.25.0\n")
	writeFile(t, dir, "main.go", `package main

type Box[T any] struct{ Value T }

func F(boxes []Box[int]) {
	for _, b := range boxes {
		_ = b
	}
}
`)
	if got := soleType(t, loadAndResolve(t, dir)); got != "Box[int]" {
		t.Fatalf("got %q, want Box[int] (fully instantiated, not bare Box[T])", got)
	}
}

func TestCrossPackageType_DefaultImport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module fixture\n\ngo 1.25.0\n")
	sub := filepath.Join(dir, "otherpkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, sub, "row.go", "package otherpkg\n\ntype Row struct{ ID string }\n")
	writeFile(t, dir, "main.go", `package main

import "fixture/otherpkg"

func F(rows []otherpkg.Row) {
	for _, r := range rows {
		_ = r
	}
}
`)
	if got := soleType(t, loadAndResolve(t, dir)); got != "otherpkg.Row" {
		t.Fatalf("got %q, want otherpkg.Row (not the raw import path)", got)
	}
}

func TestCrossPackageType_AliasedImport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module fixture\n\ngo 1.25.0\n")
	sub := filepath.Join(dir, "otherpkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, sub, "row.go", "package otherpkg\n\ntype Row struct{ ID string }\n")
	writeFile(t, dir, "main.go", `package main

import d "fixture/otherpkg"

func F(rows []d.Row) {
	for _, r := range rows {
		_ = r
	}
}
`)
	if got := soleType(t, loadAndResolve(t, dir)); got != "d.Row" {
		t.Fatalf("got %q, want d.Row (this file's actual alias, not the package's default name)", got)
	}
}

func TestCrossPackageType_DotImport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module fixture\n\ngo 1.25.0\n")
	sub := filepath.Join(dir, "otherpkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, sub, "row.go", "package otherpkg\n\ntype Row struct{ ID string }\n")
	writeFile(t, dir, "main.go", `package main

import . "fixture/otherpkg"

func F(rows []Row) {
	for _, r := range rows {
		_ = r
	}
}
`)
	if got := soleType(t, loadAndResolve(t, dir)); got != "Row" {
		t.Fatalf("got %q, want bare Row (dot-imported: no qualifier at all)", got)
	}
}

func TestPointerVsValueElementType(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module fixture\n\ngo 1.25.0\n")
	writeFile(t, dir, "main.go", `package main

type ToDo struct{ ID string }

func F(values []ToDo, pointers []*ToDo) {
	for _, v := range values {
		_ = v
	}
	for _, p := range pointers {
		_ = p
	}
}
`)
	results := loadAndResolve(t, dir)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2: %+v", len(results), results)
	}
	if results[0].Type != "ToDo" {
		t.Fatalf("value: got %q, want ToDo", results[0].Type)
	}
	if results[1].Type != "*ToDo" {
		t.Fatalf("pointer: got %q, want *ToDo (not silently normalized to value)", results[1].Type)
	}
}

func TestInterfaceTypedElement(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module fixture\n\ngo 1.25.0\n")
	writeFile(t, dir, "main.go", `package main

type Labeled interface{ Label() string }

func F(items []Labeled) {
	for _, x := range items {
		_ = x
	}
}
`)
	if got := soleType(t, loadAndResolve(t, dir)); got != "Labeled" {
		t.Fatalf("got %q, want Labeled (the interface itself, not a concrete implementer)", got)
	}
}

func TestEmbeddedPromotedField(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module fixture\n\ngo 1.25.0\n")
	writeFile(t, dir, "main.go", `package main

type Base struct{ Label string }
type ToDo struct {
	Base
	Done bool
}

func F(todos []ToDo) {
	for _, t := range todos {
		_ = t.Label // promoted field access must type-check
	}
}
`)
	if got := soleType(t, loadAndResolve(t, dir)); got != "ToDo" {
		t.Fatalf("got %q, want ToDo", got)
	}
}

func TestShadowedVariable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module fixture\n\ngo 1.25.0\n")
	writeFile(t, dir, "main.go", `package main

type Outer struct{ X int }
type ToDo struct{ ID string }

func F(outerT Outer, todos []ToDo) {
	t := outerT // outer "t", unrelated type, already in scope
	_ = t
	for _, t := range todos { // shadows the outer t inside this block only
		_ = t
	}
}
`)
	// Resolution finds the RangeStmt structurally (its own Value ident's
	// Defs entry), not by scanning for a name - so the outer, unrelated `t`
	// is never a candidate here at all.
	if got := soleType(t, loadAndResolve(t, dir)); got != "ToDo" {
		t.Fatalf("got %q, want ToDo (must resolve the shadowed inner t, not outer Outer)", got)
	}
}

func TestRangeOverFunctionCall(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module fixture\n\ngo 1.25.0\n")
	writeFile(t, dir, "main.go", `package main

type ToDo struct{ ID string }

func fetchTodos() []ToDo { return nil }

func F() {
	for _, t := range fetchTodos() {
		_ = t
	}
}
`)
	if got := soleType(t, loadAndResolve(t, dir)); got != "ToDo" {
		t.Fatalf("got %q, want ToDo (resolved via the call's return type)", got)
	}
}

// TestQualifierRoundTrips confirms the resolved type string is actually
// valid, type-checking Go when spliced into a real function-literal
// parameter - not just a plausible-looking name.
func TestQualifierRoundTrips(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module fixture\n\ngo 1.25.0\n")
	sub := filepath.Join(dir, "otherpkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, sub, "row.go", "package otherpkg\n\ntype Row struct{ ID string }\n")
	writeFile(t, dir, "main.go", `package main

import d "fixture/otherpkg"

func F(rows []*d.Row) {
	for _, r := range rows {
		_ = r
	}
}
`)
	got := soleType(t, loadAndResolve(t, dir))
	if got != "*d.Row" {
		t.Fatalf("got %q, want *d.Row", got)
	}

	// Splice it into a second, independent package and confirm it actually
	// compiles as a function-literal parameter type.
	dir2 := t.TempDir()
	writeFile(t, dir2, "go.mod", "module fixture\n\ngo 1.25.0\n")
	sub2 := filepath.Join(dir2, "otherpkg")
	if err := os.MkdirAll(sub2, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, sub2, "row.go", "package otherpkg\n\ntype Row struct{ ID string }\n")
	writeFile(t, dir2, "main.go", fmt.Sprintf(`package main

import d "fixture/otherpkg"

func F(rows []*d.Row) {
	render := func(r %s) string { return r.ID }
	_ = render
}
`, got))
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports,
		Dir: dir2,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatalf("resolved type %q does not produce compiling Go", got)
	}
}

// parseSrc parses src (a full Go source file) for the WalkNamedRangeValues
// tests below - a plain go/parser.ParseFile, no go/types/go/packages needed,
// since WalkNamedRangeValues works on syntax alone (see internal/lsp's own
// hover-based caller, which never has type info either).
func parseSrc(t *testing.T, src string) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "", src, 0)
	if err != nil {
		t.Fatalf("parser.ParseFile: %v", err)
	}
	return f
}

func walkedNames(t *testing.T, src string) []string {
	t.Helper()
	var names []string
	typeresolve.WalkNamedRangeValues(parseSrc(t, src), func(_ *ast.RangeStmt, id *ast.Ident) {
		names = append(names, id.Name)
	})
	return names
}

// TestWalkNamedRangeValues_NamedValue is the ordinary case both
// RangeVarTypesInFile and internal/lsp's findKeyedForCandidates key their
// resolution off of - the exact shape they used to duplicate their own
// parallel AST walks to detect.
func TestWalkNamedRangeValues_NamedValue(t *testing.T) {
	got := walkedNames(t, "package main\nfunc F(xs []int) {\n\tfor _, x := range xs {\n\t\t_ = x\n\t}\n}\n")
	if len(got) != 1 || got[0] != "x" {
		t.Fatalf("got %v, want [x]", got)
	}
}

// TestWalkNamedRangeValues_BlankValueSkipped confirms an explicit blank
// value variable ("for _, _ := range") is excluded - it names nothing worth
// keying on, and both callers relied on this exact exclusion.
func TestWalkNamedRangeValues_BlankValueSkipped(t *testing.T) {
	got := walkedNames(t, "package main\nfunc F(xs []int) {\n\tfor _, _ = range xs {\n\t}\n}\n")
	if len(got) != 0 {
		t.Fatalf("got %v, want none (blank value)", got)
	}
}

// TestWalkNamedRangeValues_IndexOnlySkipped confirms a range with no value
// variable at all ("for i := range" or bare "for range") yields nothing -
// there's no per-item value to key a list on.
func TestWalkNamedRangeValues_IndexOnlySkipped(t *testing.T) {
	got := walkedNames(t, "package main\nfunc F(xs []int) {\n\tfor i := range xs {\n\t\t_ = i\n\t}\n\tfor range xs {\n\t}\n}\n")
	if len(got) != 0 {
		t.Fatalf("got %v, want none (index-only/bare range)", got)
	}
}

// TestWalkNamedRangeValues_MultipleAndNested confirms every named-value
// range in a file is found, including one nested inside another - a single
// AST walk must not stop at the first match or miss inner statements.
func TestWalkNamedRangeValues_MultipleAndNested(t *testing.T) {
	got := walkedNames(t, `package main

func F(rows [][]int) {
	for _, r := range rows {
		for _, n := range r {
			_ = n
		}
	}
	for _, r := range rows {
		_ = r
	}
}
`)
	want := []string{"r", "n", "r"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
