// Package typeresolve resolves the concrete type of a `for _, x := range
// ...` loop's value variable via go/types, for callers that need to embed
// that type by name in newly generated Go source (a function-literal
// parameter, for instance, which Go's grammar requires an explicit type on -
// unlike a plain `:=` declaration, there's no elision form). Isolated in its
// own package, kept out of internal/compiler, since resolving a type
// requires a fully loaded, type-checked package (golang.org/x/tools/go/packages)
// - a real, non-trivial cost unsuitable for a per-keystroke editor compile.
package typeresolve

import (
	"go/ast"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/packages"
)

// FileQualifier returns a types.Qualifier reflecting how f actually imports
// things: the package's own name for an ordinary import, f's own alias for
// an aliased import, "" for a dot-imported package (referenced with no
// qualifier at all in that file's source), and "" for sameAs itself (the
// package the resolved type string will be spliced into source for - no
// qualifier needed for a type declared in that same package).
//
// Using a types.Type's own String() instead of this qualifier is unsafe: it
// prints a type's full import path ("fixture/otherpkg.Row"), which is not
// valid Go source syntax - Go source needs the local identifier
// ("otherpkg.Row", or whatever alias this specific file uses).
func FileQualifier(f *ast.File, sameAs *types.Package) types.Qualifier {
	aliasByPath := map[string]string{}
	dotImports := map[string]bool{}
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || imp.Name == nil {
			continue
		}
		switch imp.Name.Name {
		case "_":
			// Blank import: never referenced by generated code.
		case ".":
			dotImports[path] = true
		default:
			aliasByPath[path] = imp.Name.Name
		}
	}
	return func(p *types.Package) string {
		if sameAs != nil && p == sameAs {
			return ""
		}
		if dotImports[p.Path()] {
			return ""
		}
		if alias, ok := aliasByPath[p.Path()]; ok {
			return alias
		}
		return p.Name()
	}
}

// RangeVarType is one resolved `for _, x := range ...` loop value variable's
// type. Filename/Line locate that range statement's `for` keyword - go/token
// resolves these through any `//line` directive in effect at that position,
// so for code compiled with such directives (as Vane's own compiler emits),
// Filename/Line already name the *original* source file and line the
// directive points at, not the physical file being parsed. Column is
// deliberately not exposed: a `//line` directive only pins column 0 of its
// own line to a specific target column, and go/token then advances the
// reported column linearly by counting characters on the physical line from
// there - which only reflects a real source column when the physical and
// logical lines are character-for-character identical up to that point.
// Vane's generated `for` lines add their own leading indentation that has
// no counterpart in the original source, so the reported column would be
// off by exactly that indentation width. Line-level granularity, plus a
// caller searching that one line's own text for "for", is what stays
// reliable.
type RangeVarType struct {
	Filename string // resolved filename of the `for` keyword
	Line     int    // resolved 1-based line number of the `for` keyword
	Type     string // resolved concrete type, qualified for that file
}

// RangeVarTypesInFile resolves the type of every `for _, x := range ...`
// loop's value variable in file, using pkg's already-computed type
// information (pkg.TypesInfo, pkg.Fset - load pkg with
// packages.NeedTypes|NeedTypesInfo|NeedSyntax|NeedDeps|NeedImports first).
// Each result's type is qualified against file's own imports (FileQualifier)
// with pkg.Types as the no-qualifier package, since generated code always
// lives in the same package as the .vane file it came from.
//
// Resolves every range loop found, not just ones a caller cares about -
// harmless for loops nobody looks up, and avoids needing a separate pass to
// first decide which loops matter.
func RangeVarTypesInFile(pkg *packages.Package, file *ast.File) []RangeVarType {
	qualifier := FileQualifier(file, pkg.Types)
	var results []RangeVarType
	WalkNamedRangeValues(file, func(rs *ast.RangeStmt, id *ast.Ident) {
		obj := pkg.TypesInfo.Defs[id]
		if obj == nil {
			// rs.Value assigns an already-declared variable (range ... = ...
			// rather than range ... := ...); Uses covers that case.
			obj = pkg.TypesInfo.Uses[id]
		}
		v, ok := obj.(*types.Var)
		if !ok {
			return
		}
		pos := pkg.Fset.Position(rs.For)
		results = append(results, RangeVarType{
			Filename: pos.Filename,
			Line:     pos.Line,
			Type:     types.TypeString(v.Type(), qualifier),
		})
	})
	return results
}

// WalkNamedRangeValues calls fn for every for-range statement in file whose
// value variable is a named, non-blank identifier ("for _, x := range" - not
// "for range", not "for i := range" with no value, not an explicit blank
// "for _, _ := range"). The shared "is this loop's value worth keying on"
// test both this package's own type-info-based resolution and
// internal/lsp's hover-based one start from.
func WalkNamedRangeValues(file *ast.File, fn func(rs *ast.RangeStmt, id *ast.Ident)) {
	ast.Inspect(file, func(n ast.Node) bool {
		rs, ok := n.(*ast.RangeStmt)
		if !ok {
			return true
		}
		id, ok := rs.Value.(*ast.Ident)
		if !ok || id.Name == "" || id.Name == "_" {
			return true
		}
		fn(rs, id)
		return true
	})
}
