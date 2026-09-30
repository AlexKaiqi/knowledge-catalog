// Command export-audit lists the exported top-level symbols of every
// production package together with the number of references from OTHER
// packages (production and test code separately). It is the B8-review
// instrument: export faces must be counted from the AST, never from `go doc`
// text output (35-line truncation hid 25 aliases there).
//
// The report is advisory. Zero-consumer exports are narrowing candidates, but
// wire/JSON shapes (DTOs marshalled without a Go-side field reference) and
// protocol-contract surfaces legitimately show zero Go consumers; each flag
// needs human review before narrowing.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type symbol struct {
	kind, name string
}

type pkgInfo struct {
	dir      string
	importID string // package name as seen by importers (dir base or declared)
	alias    string // declared package name (defaults to importID)
	syms     map[string]symbol
	prodUse  map[string]int
	testUse  map[string]int
	imports  map[string]bool // package names this pkg imports (for scoping)
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	pkgs := map[string]*pkgInfo{}
	var order []string
	// Pass 1: collect every package's exported symbols and import names.
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() || strings.Contains(path, "/.git") {
			return nil
		}
		entries, _ := os.ReadDir(path)
		hasGo := false
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".go") {
				hasGo = true
				break
			}
		}
		if !hasGo {
			return nil
		}
		p := &pkgInfo{dir: path, syms: map[string]symbol{}, prodUse: map[string]int{}, testUse: map[string]int{}, imports: map[string]bool{}}
		alias := filepath.Base(path)
		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, filepath.Join(path, name), nil, 0)
			if err != nil || f.Name == nil {
				continue
			}
			alias = f.Name.Name
			collectSymbols(f, p.syms)
		}
		p.importID = filepath.Base(path)
		p.alias = alias
		pkgs[p.importID] = p
		order = append(order, p.importID)
		return nil
	})
	// Pass 2: count qualified references to each package's symbols from every
	// other package (production + test files), scoped by import alias.
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() || strings.Contains(path, "/.git") {
			return nil
		}
		entries, _ := os.ReadDir(path)
		self := pkgs[filepath.Base(path)]
		for _, e := range entries {
			name := e.Name()
			if !strings.HasSuffix(name, ".go") {
				continue
			}
			isTest := strings.HasSuffix(name, "_test.go")
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, filepath.Join(path, name), nil, 0)
			if err != nil || f.Name == nil {
				continue
			}
			self.imports[f.Name.Name] = true
			// import name -> target package export set
			targets := map[string]*pkgInfo{}
			for _, imp := range f.Imports {
				pkgName := importPathBase(imp.Path.Value)
				if target, ok := pkgs[pkgName]; ok && target.alias != f.Name.Name {
					targets[importAlias(imp, pkgName)] = target
				}
			}
			if len(targets) == 0 {
				continue
			}
			ast.Inspect(f, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				id, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				target, ok := targets[id.Name]
				if !ok {
					return true
				}
				if _, isSym := target.syms[sel.Sel.Name]; isSym {
					if self.importID == target.importID {
						return true
					}
					if isTest {
						target.testUse[sel.Sel.Name]++
					} else {
						target.prodUse[sel.Sel.Name]++
					}
				}
				return true
			})
		}
		return nil
	})
	sort.Strings(order)
	flagged := 0
	for _, id := range order {
		p := pkgs[id]
		names := make([]string, 0, len(p.syms))
		for n := range p.syms {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			if p.prodUse[n] == 0 {
				flagged++
				fmt.Printf("%s\t%s %s\tprod=0 test=%d\tNARROW-CANDIDATE\n", p.dir, p.syms[n].kind, n, p.testUse[n])
			}
		}
	}
	fmt.Fprintf(os.Stderr, "packages=%d narrow-candidates=%d (prod=0; test-only and wire shapes need human review)\n", len(order), flagged)
}

func collectSymbols(f *ast.File, out map[string]symbol) {
	for _, d := range f.Decls {
		switch dd := d.(type) {
		case *ast.FuncDecl:
			if dd.Recv == nil && dd.Name.IsExported() {
				out[dd.Name.Name] = symbol{"func", dd.Name.Name}
			}
		case *ast.GenDecl:
			for _, spec := range dd.Specs {
				switch ss := spec.(type) {
				case *ast.TypeSpec:
					if ss.Name.IsExported() {
						out[ss.Name.Name] = symbol{"type", ss.Name.Name}
					}
				case *ast.ValueSpec:
					for _, id := range ss.Names {
						if id.IsExported() {
							kind := "var"
							if dd.Tok == token.CONST {
								kind = "const"
							}
							out[id.Name] = symbol{kind, id.Name}
						}
					}
				}
			}
		}
	}
}

func importPathBase(lit string) string {
	p := strings.Trim(lit, `"`)
	parts := strings.Split(p, "/")
	return parts[len(parts)-1]
}

func importAlias(imp *ast.ImportSpec, base string) string {
	if imp.Name != nil && imp.Name.Name != "" && imp.Name.Name != "_" {
		return imp.Name.Name
	}
	return base
}
