package arch_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotStoreFileModuleHasIndependentOwners(t *testing.T) {
	graph := loadGraph(t)
	for _, pkg := range []string{"snapshot/dataset", "snapshot/access", "snapshot/service", "snapshot/console"} {
		if _, exists := graph[pkg]; !exists {
			t.Errorf("Snapshot Store capability %s has no implementation owner", pkg)
			continue
		}
		for dependency, trail := range graph.reachable(pkg) {
			for _, upper := range []string{"catalog", "home", "invocation", "invoke", "server", "cli", "client", "knowledge", "knowledgeapp", "retrieval", "index", "controlplane"} {
				if dependency == upper || strings.HasPrefix(dependency, upper+"/") {
					t.Errorf("%s binds the file module to external KC application %s: %v", pkg, dependency, trail)
				}
			}
		}
	}
	root := moduleRoot(t)
	for _, path := range []string{"snapshot/console/webui/package.json", "snapshot/console/webui/LICENSE", "snapshot/console/webui/NOTICE"} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Errorf("Snapshot Store console must own %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "webui", "package.json")); err == nil {
		t.Error("the full console is still owned outside the Snapshot Store module")
	}
}

func TestLegacyCatalogDoesNotRetainDatasetAuthority(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(moduleRoot(t), "catalog", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			if method, ok := decl.(*ast.FuncDecl); ok && method.Recv != nil {
				t.Errorf("legacy catalog retains domain method %s in %s; Dataset authority belongs to snapshot/dataset", method.Name, filepath.Base(path))
			}
		}
	}
}
