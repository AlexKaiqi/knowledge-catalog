package arch_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Kernel is deliberately smaller than "all shared types": it owns errors,
// digests, and Snapshot coordinates. Layer ② identity and declarations belong
// to knowledge even when putting them in kernel would avoid an import.
func TestLayerOwnedDeclarationsDoNotDriftBack(t *testing.T) {
	checkTopLevelDeclarations(t, "kernel", map[string]struct{}{
		"ObjectID": {}, "Address": {}, "AddressKind": {},
		"KnowledgeRef": {}, "PinnedKnowledgeRef": {},
		"ProvenanceEnvelope": {}, "OriginKind": {}, "AlgorithmRef": {},
		"ParsedSchemaRef": {}, "SchemaObjectPrefix": {},
		"FormatKnowledgeRef": {}, "FormatPinnedRef": {},
		"ParseSchemaRef": {}, "IsSchemaObject": {},
	})
	checkTopLevelDeclarations(t, "knowledge/reader", map[string]struct{}{
		"Projection": {}, "IndexDescriptor": {}, "NewProjection": {},
	})
	checkInterfaceMethods(t, "knowledge", "ReadStore", map[string]struct{}{
		"Search": {}, "Probe": {}, "Retrieve": {},
	})
}

// This catches a second failure mode if a knowledge type is ever added back to
// kernel: Snapshot and Catalog still may not consume it through that shortcut.
func TestSnapshotAndCatalogDoNotUseKnowledgeKernelTypes(t *testing.T) {
	forbidden := map[string]struct{}{
		"ObjectID": {}, "Address": {}, "KnowledgeRef": {},
		"ProvenanceEnvelope": {}, "OriginKind": {}, "AlgorithmRef": {},
		"SchemaRef": {}, "AddressKind": {},
	}
	for _, pkg := range []string{"snapshot", "catalog", "catalog/worktree"} {
		t.Run(pkg, func(t *testing.T) {
			checkKernelSelectors(t, pkg, forbidden)
		})
	}
}

func checkTopLevelDeclarations(t *testing.T, pkg string, forbidden map[string]struct{}) {
	t.Helper()
	root := moduleRoot(t)
	dir := filepath.Join(root, pkg)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			switch item := decl.(type) {
			case *ast.FuncDecl:
				if _, denied := forbidden[item.Name.Name]; denied {
					pos := fset.Position(item.Pos())
					t.Errorf("%s declares layer-owned symbol %s at %s:%d", pkg, item.Name.Name, entry.Name(), pos.Line)
				}
			case *ast.GenDecl:
				for _, spec := range item.Specs {
					var name string
					switch value := spec.(type) {
					case *ast.TypeSpec:
						name = value.Name.Name
					case *ast.ValueSpec:
						for _, ident := range value.Names {
							if _, denied := forbidden[ident.Name]; denied {
								pos := fset.Position(ident.Pos())
								t.Errorf("%s declares layer-owned symbol %s at %s:%d", pkg, ident.Name, entry.Name(), pos.Line)
							}
						}
					}
					if _, denied := forbidden[name]; denied {
						pos := fset.Position(spec.Pos())
						t.Errorf("%s declares layer-owned symbol %s at %s:%d", pkg, name, entry.Name(), pos.Line)
					}
				}
			}
		}
	}
}

func checkInterfaceMethods(t *testing.T, pkg, interfaceName string, forbidden map[string]struct{}) {
	t.Helper()
	dir := filepath.Join(moduleRoot(t), pkg)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || typeSpec.Name.Name != interfaceName {
					continue
				}
				iface, ok := typeSpec.Type.(*ast.InterfaceType)
				if !ok {
					t.Fatalf("%s.%s is no longer an interface", pkg, interfaceName)
				}
				for _, field := range iface.Methods.List {
					for _, name := range field.Names {
						if _, denied := forbidden[name.Name]; denied {
							pos := fset.Position(name.Pos())
							t.Errorf("%s.%s owns retrieval method %s at %s:%d", pkg, interfaceName, name.Name, entry.Name(), pos.Line)
						}
					}
				}
			}
		}
	}
}

func checkKernelSelectors(t *testing.T, pkg string, forbidden map[string]struct{}) {
	t.Helper()
	root := moduleRoot(t)
	dir := filepath.Join(root, pkg)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		kernelNames := map[string]struct{}{}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil || importPath != modulePath+"/kernel" {
				continue
			}
			name := "kernel"
			if spec.Name != nil {
				name = spec.Name.Name
			}
			kernelNames[name] = struct{}{}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			if _, ok := kernelNames[ident.Name]; !ok {
				return true
			}
			if _, denied := forbidden[selector.Sel.Name]; denied {
				pos := fset.Position(selector.Pos())
				t.Errorf("%s uses layer ② kernel type %s at %s:%d", pkg, selector.Sel.Name, entry.Name(), pos.Line)
			}
			return true
		})
	}
}
