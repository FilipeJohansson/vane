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
// type, at the byte offset - within its own file - of that range
// statement's `for` keyword.
type RangeVarType struct {
	Offset int    // byte offset of the `for` keyword, within its own file
	Type   string // resolved concrete type, qualified for that file
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
	ast.Inspect(file, func(n ast.Node) bool {
		rs, ok := n.(*ast.RangeStmt)
		if !ok {
			return true
		}
		id, ok := rs.Value.(*ast.Ident)
		if !ok {
			return true
		}
		obj := pkg.TypesInfo.Defs[id]
		if obj == nil {
			// rs.Value assigns an already-declared variable (range ... = ...
			// rather than range ... := ...); Uses covers that case.
			obj = pkg.TypesInfo.Uses[id]
		}
		v, ok := obj.(*types.Var)
		if !ok {
			return true
		}
		results = append(results, RangeVarType{
			Offset: pkg.Fset.Position(rs.For).Offset,
			Type:   types.TypeString(v.Type(), qualifier),
		})
		return true
	})
	return results
}
