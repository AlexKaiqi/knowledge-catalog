package plugin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// surfaceAnchor is the exported-surface closed set (CAP-ORG-01 B3, §2.2 ④).
// Every package-level exported declaration of this package is pinned here:
// adding or removing one must edit this list in the same change, so the
// contribution contract cannot drift silently away from its package README.
type surfaceAnchorSuite []string

var surfaceAnchor = surfaceAnchorSuite{
	"IndexContribution",
	"IndexMaintenance",
	"IndexPluginEntry",
	"IndexPlugins",
	"IndexQueries",
	"OpenIndexPlugins",
	"ParseRepositoryPlugins",
	"PluginAffordance",
	"PluginScope",
	"PluginSelection",
	"ProjectionController",
	"ProjectionDescriber",
	"ProjectionStateView",
	"ProjectionSynchronizer",
	"RelationProjection",
	"RepositoryPluginConfig",
	"RepositoryPluginConfigs",
	"RepositoryPluginsKey",
	"SearchProjection",
	"SelectRepositoryPlugins",
	"SemanticWindowProjection",
	"SourcePlugin",
}

// TestExportedSurfaceMatchesAnchor parses the package's production files and
// asserts set equality between the exported package-level declarations and the
// anchor list.
func TestExportedSurfaceMatchesAnchor(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					if d.Recv == nil && d.Name.IsExported() {
						got[d.Name.Name] = true
					}
				case *ast.GenDecl:
					for _, spec := range d.Specs {
						switch s := spec.(type) {
						case *ast.TypeSpec:
							if s.Name.IsExported() {
								got[s.Name.Name] = true
							}
						case *ast.ValueSpec:
							for _, name := range s.Names {
								if name.IsExported() {
									got[name.Name] = true
								}
							}
						}
					}
				}
			}
		}
	}
	want := map[string]bool{}
	for _, name := range surfaceAnchor {
		if want[name] {
			t.Errorf("anchor lists %q twice", name)
		}
		want[name] = true
	}
	var added, removed []string
	for name := range got {
		if !want[name] {
			added = append(added, name)
		}
	}
	for name := range want {
		if !got[name] {
			removed = append(removed, name)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	if len(added) > 0 || len(removed) > 0 {
		t.Errorf("exported surface drifted from the anchor (add or remove the entry in surface_anchor_test.go in the same change): new=%v removed=%v", added, removed)
	}
}
