// Command apisurface generates and checks Vane's public API surface golden
// files (see API_STABILITY.md's "Enforcement" section): one deterministic,
// sorted text dump per package (core, core/router, core/signal,
// core/domattrs), including exported methods and struct fields, plus
// Deprecated/EXPERIMENTAL marker presence per symbol.
//
// core and core/router only expose their real surface under
// GOOS=js GOARCH=wasm; Dump loads packages under that target explicitly.
//
// Usage:
//
//	go run ./internal/apisurface          # check current surface against the golden files
//	go run ./internal/apisurface -write   # regenerate the golden files from current source
package main

import (
	"fmt"
	"go/doc"
	"go/types"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Package describes one of Vane's frozen public API packages and the golden
// file (under goldens/, repo root) that holds its surface dump.
type Package struct {
	ImportPath string
	Golden     string // filename under goldens/
}

// Packages are the packages that make up Vane's frozen public API surface,
// per API_STABILITY.md, each with its own golden file.
var Packages = []Package{
	{"github.com/filipejohansson/vane/core", "core.golden"},
	{"github.com/filipejohansson/vane/core/router", "core-router.golden"},
	{"github.com/filipejohansson/vane/core/signal", "core-signal.golden"},
	{"github.com/filipejohansson/vane/core/domattrs", "core-domattrs.golden"},
}

// Surface is one package's dumped exported surface: deterministic, sorted,
// newline-terminated text.
type Surface struct {
	Package Package
	Content string
}

// Dump loads Packages from the module rooted at dir and returns each
// package's exported surface separately, one Surface per Package, in the
// same order as Packages.
func Dump(dir string) ([]Surface, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax |
			packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports,
		Dir: dir,
		Env: append(os.Environ(), "GOOS=js", "GOARCH=wasm"),
	}
	importPaths := make([]string, len(Packages))
	for i, p := range Packages {
		importPaths[i] = p.ImportPath
	}
	pkgs, err := packages.Load(cfg, importPaths...)
	if err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}
	if len(pkgs) != len(Packages) {
		return nil, fmt.Errorf("expected %d packages, got %d", len(Packages), len(pkgs))
	}
	byPath := make(map[string]*packages.Package, len(pkgs))
	for _, pkg := range pkgs {
		byPath[pkg.PkgPath] = pkg
	}

	surfaces := make([]Surface, len(Packages))
	for i, p := range Packages {
		pkg, ok := byPath[p.ImportPath]
		if !ok {
			return nil, fmt.Errorf("%s: not found among loaded packages", p.ImportPath)
		}
		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("%s: %v", pkg.PkgPath, pkg.Errors[0])
		}
		lines, err := dumpPackage(pkg)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", pkg.PkgPath, err)
		}
		sort.Strings(lines)
		surfaces[i] = Surface{Package: p, Content: strings.Join(lines, "\n") + "\n"}
	}
	return surfaces, nil
}

func dumpPackage(pkg *packages.Package) ([]string, error) {
	docPkg, err := doc.NewFromFiles(pkg.Fset, pkg.Syntax, pkg.PkgPath, doc.AllDecls)
	if err != nil {
		return nil, fmt.Errorf("building doc package: %w", err)
	}
	markers := markerIndex(docPkg)

	scope := pkg.Types.Scope()
	var lines []string
	qual := types.RelativeTo(pkg.Types)

	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}

		switch obj := obj.(type) {
		case *types.TypeName:
			lines = append(lines, formatLine("type "+types.ObjectString(obj, qual), markers[name]))
			lines = append(lines, dumpNamedMembers(obj, qual, markers)...)
		default:
			lines = append(lines, formatLine(types.ObjectString(obj, qual), markers[name]))
		}
	}
	return lines, nil
}

// dumpNamedMembers dumps the exported methods (value and pointer receiver)
// and, for struct types, exported fields of a top-level named type.
func dumpNamedMembers(tn *types.TypeName, qual types.Qualifier, markers map[string]string) []string {
	named, ok := tn.Type().(*types.Named)
	if !ok {
		return nil
	}

	var lines []string
	seen := map[string]bool{}

	for i := 0; i < named.NumMethods(); i++ {
		m := named.Method(i)
		if !m.Exported() {
			continue
		}
		key := tn.Name() + "." + m.Name()
		if seen[key] {
			continue
		}
		seen[key] = true
		lines = append(lines, formatLine(types.ObjectString(m, qual), markers[key]))
	}

	if st, ok := named.Underlying().(*types.Struct); ok {
		for i := 0; i < st.NumFields(); i++ {
			f := st.Field(i)
			if !f.Exported() {
				continue
			}
			key := tn.Name() + "." + f.Name()
			sig := fmt.Sprintf("field %s.%s %s", tn.Name(), f.Name(), types.TypeString(f.Type(), qual))
			lines = append(lines, formatLine(sig, markers[key]))
		}
	}

	return lines
}

func formatLine(signature, marker string) string {
	if marker != "" {
		return signature + " [" + marker + "]"
	}
	return signature
}

// markerIndex maps a symbol name (or "Type.Member") to "deprecated" or
// "experimental" based on the Go convention (a doc comment paragraph
// starting with "Deprecated:") and Vane's own EXPERIMENTAL: prefix
// convention (API_STABILITY.md), by scanning the go/doc-parsed doc
// comments of top-level declarations and their methods/fields.
func markerIndex(pkg *doc.Package) map[string]string {
	idx := map[string]string{}

	mark := func(name, docText string) {
		if m := markerFor(docText); m != "" {
			idx[name] = m
		}
	}

	mark(pkg.Name, pkg.Doc)
	for _, c := range pkg.Consts {
		for _, n := range c.Names {
			mark(n, c.Doc)
		}
	}
	for _, v := range pkg.Vars {
		for _, n := range v.Names {
			mark(n, v.Doc)
		}
	}
	for _, f := range pkg.Funcs {
		mark(f.Name, f.Doc)
	}
	for _, t := range pkg.Types {
		mark(t.Name, t.Doc)
		for _, f := range t.Funcs {
			mark(f.Name, f.Doc)
		}
		for _, m := range t.Methods {
			mark(t.Name+"."+m.Name, m.Doc)
		}
	}
	return idx
}

func markerFor(docText string) string {
	docText = strings.TrimSpace(docText)
	if strings.HasPrefix(docText, "EXPERIMENTAL:") {
		return "experimental"
	}
	for _, para := range strings.Split(docText, "\n\n") {
		if strings.HasPrefix(strings.TrimSpace(para), "Deprecated:") {
			return "deprecated"
		}
	}
	return ""
}
