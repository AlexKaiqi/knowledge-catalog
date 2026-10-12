package arch_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

var registrationOwners = names("home client server invocation snapshot/dataset snapshot/service snapshot/access invoke hook gate controlplane")
var retiredRegistrationNames = names("catalog catalogstate catalogbinding catalogprovider catalogservice catalogid catalogids catalogs catalogbindings catalogregistries currentcatalog selectedcatalog defaultcatalogid catalogspath newcatalog opencatalog createcatalog pickcatalog")
var singletonRegistrationFields = map[string]string{
	"home.DeploymentConfig.Registry":      "RegistryBinding",
	"home.Home.Datasets":                  "kc/snapshot/dataset.Service",
	"home.Home.Registry":                  "kc/snapshot/dataset.Registry",
	"home.HomeFile.Registry":              "HomeRegistry",
	"snapshot/service.Resources.Datasets": "kc/snapshot/dataset.Service",
}

// A service has one registration authority. A Catalog selector or namespace
// collection must not return through a deployment, transport, or Dataset port.
func TestSingleRegistryPublicContracts(t *testing.T) {
	walkSnapshotBoundary(t, func(path string, file *ast.File) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		owner := filepath.ToSlash(filepath.Dir(path))
		if !registrationOwners[owner] {
			return
		}
		for _, signal := range registrationContractSignals(owner, file) {
			t.Errorf("%s: %s", path, signal)
		}
	})
}

func registrationContractSignals(owner string, file *ast.File) []string {
	imports := snapshotImportNames(file)
	var signals []string
	for _, decl := range file.Decls {
		switch item := decl.(type) {
		case *ast.FuncDecl:
			if item.Name.IsExported() && retiredRegistrationNames[strings.ToLower(item.Name.Name)] {
				signals = append(signals, "retired Catalog entry "+item.Name.Name)
			}
			if owner == "snapshot/dataset" && (item.Name.Name == "OpenSnapshotRegistry" || item.Name.Name == "CreateSnapshotRegistry") {
				var params []ast.Expr
				for _, field := range item.Type.Params.List {
					count := max(1, len(field.Names))
					for range count {
						params = append(params, field.Type)
					}
				}
				if len(params) != 2 || registrationTypeName(params[0], imports) != "kc/snapshot.Store" || registrationTypeName(params[1], imports) != "string" {
					signals = append(signals, item.Name.Name+" must take Snapshot authority and ref only")
				}
			}
		case *ast.GenDecl:
			for _, spec := range item.Specs {
				typ, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if typ.Name.IsExported() && retiredRegistrationNames[strings.ToLower(typ.Name.Name)] {
					signals = append(signals, "retired Catalog type "+typ.Name.Name)
				}
				switch shape := typ.Type.(type) {
				case *ast.StructType:
					signals = append(signals, registrationFieldSignals(owner, typ.Name.Name, shape, imports)...)
				case *ast.InterfaceType:
					if owner == "snapshot/dataset" && typ.Name.Name == "Service" {
						for _, method := range shape.Methods.List {
							for _, name := range method.Names {
								if name.Name == "Archive" || name.Name == "Archived" {
									signals = append(signals, "retired registration lifecycle "+name.Name)
								}
							}
						}
					}
				}
			}
		}
	}
	return signals
}

func registrationFieldSignals(owner, typ string, shape *ast.StructType, imports map[string]string) []string {
	var signals []string
	for _, field := range shape.Fields.List {
		if field.Tag != nil {
			raw, err := strconv.Unquote(field.Tag.Value)
			if err == nil {
				for _, format := range []string{"json", "yaml"} {
					name, _, _ := strings.Cut(reflect.StructTag(raw).Get(format), ",")
					if retiredRegistrationNames[strings.ToLower(name)] {
						signals = append(signals, "retired Catalog wire selector "+format+":"+name)
					}
				}
			}
		}
		for _, name := range field.Names {
			if retiredRegistrationNames[strings.ToLower(name.Name)] {
				signals = append(signals, "retired Catalog selector "+name.Name)
			}
			key := owner + "." + typ + "." + name.Name
			if expected, selected := singletonRegistrationFields[key]; selected && registrationTypeName(field.Type, imports) != expected {
				signals = append(signals, fmt.Sprintf("%s must remain a singleton %s", key, expected))
			}
		}
	}
	return signals
}

func registrationTypeName(expr ast.Expr, imports map[string]string) string {
	if selector, ok := expr.(*ast.SelectorExpr); ok {
		if alias, ok := selector.X.(*ast.Ident); ok {
			return imports[alias.Name] + "." + selector.Sel.Name
		}
	}
	return snapshotTypeName(expr)
}

func TestSingleRegistryGuardRejectsSelectorsAndNamespaceCollections(t *testing.T) {
	for _, tc := range []struct{ name, owner, source, want string }{
		{"deploymentCollection", "home", "type DeploymentConfig struct { Registry []RegistryBinding }", "singleton"},
		{"datasetNamespaceMap", "home", "type Home struct { Datasets map[string]dataset.Service }", "singleton"},
		{"pointerToRegistry", "home", "type Home struct { Registry *dataset.Registry }", "singleton"},
		{"serviceNamespaceMap", "snapshot/service", "type Resources struct { Datasets map[string]dataset.Service }", "singleton"},
		{"privateSelector", "server", "type coordinate struct { catalog string }", "retired Catalog selector"},
		{"renamedWireSelector", "client", "type Coordinate struct { Scope string `json:\"CaTaLoG,omitempty\"` }", "wire selector"},
		{"catalogFactory", "home", "func NewCatalog() {}", "retired Catalog entry"},
		{"registrySelectionArgument", "snapshot/dataset", "func OpenSnapshotRegistry(store snapshot.Store, catalogID, ref string) {}", "authority and ref only"},
		{"registryArchive", "snapshot/dataset", "type Service interface { Archive() error }", "retired registration lifecycle"},
		{"validSingletons", "home", "type Home struct { Datasets dataset.Service; Registry dataset.Registry }", ""},
		{"validRegistryFactory", "snapshot/dataset", "func OpenSnapshotRegistry(store snapshot.Store, ref string) {}", ""},
		{"repositoryArchive", "snapshot", "type Store interface { Archive() error }", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "package fixture\nimport (\"kc/snapshot\"; \"kc/snapshot/dataset\")\n" + tc.source
			file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
			if err != nil {
				t.Fatal(err)
			}
			signals := registrationContractSignals(tc.owner, file)
			if tc.want == "" {
				if len(signals) != 0 {
					t.Fatalf("valid singleton contract rejected: %v", signals)
				}
				return
			}
			if !strings.Contains(strings.Join(signals, "\n"), tc.want) {
				t.Fatalf("regression escaped guard: want %s, got %v", tc.want, signals)
			}
		})
	}
}
