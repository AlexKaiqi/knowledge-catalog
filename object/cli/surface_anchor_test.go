package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestObjectCLIPublicSurfaceIsClosed(t *testing.T) {
	want := []string{"BindOperands", "Command", "Commands", "Context", "Executor", "ModelHelp", "New", "ReceiptWaitBudget", "Shape"}
	packages, err := parser.ParseDir(token.NewFileSet(), ".", func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			for _, declaration := range file.Decls {
				switch declaration := declaration.(type) {
				case *ast.FuncDecl:
					if declaration.Recv == nil && declaration.Name.IsExported() {
						got = append(got, declaration.Name.Name)
					}
				case *ast.GenDecl:
					for _, spec := range declaration.Specs {
						switch spec := spec.(type) {
						case *ast.TypeSpec:
							if spec.Name.IsExported() {
								got = append(got, spec.Name.Name)
							}
						case *ast.ValueSpec:
							for _, name := range spec.Names {
								if name.IsExported() {
									got = append(got, name.Name)
								}
							}
						}
					}
				}
			}
		}
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Object CLI public declarations drifted: got=%v want=%v", got, want)
	}
}
