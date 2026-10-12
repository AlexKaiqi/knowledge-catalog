package arch_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// These are data and assembly values, not authorities. Adding a concrete
// public type requires an explicit ownership decision here.
var snapshotPublicDTOs = map[string]map[string]bool{
	"kc/snapshot":         names("Advanced CommandEntry CommandRequest DirectoryCursor FileRef LakeFSEndpoint Receipt ReceiptResult"),
	"kc/snapshot/dataset": names("Commit History LogQuery State DatasetItem KnowledgeSet KnowledgeSetCheck KnowledgeSetFileMount KnowledgeSetIssue KnowledgeSetMount KnowledgeSetOverlay KnowledgeSetRecipe KnowledgeSetSource ManagementOptions MountRoute ResolvedKnowledgeSet Snapshot VirtualFile VirtualMount"),
	"kc/snapshot/access":  names("GrantList Query Rule"),
	"kc/snapshot/service": names("Authorization Config FileCoordinate FileDirectoryRequest FileDirectoryResponse FileMountsRequest FileMountsResponse FileReadEvidence FileReadRequest FileReadResponse IdentityOptions RequestMetadata Resources ViewOptions"),
}

var snapshotDTOMethods = names("kc/snapshot/dataset.State.IsEmpty kc/snapshot/dataset.KnowledgeSetSource.IsFileEntry kc/snapshot/dataset.KnowledgeSetRecipe.Sources kc/snapshot/access.Rule.UnmarshalJSON kc/snapshot/service.Resources.OpenFiles")
var snapshotPureDTOAliases = names("DirectoryRequest DirectoryEntry DirectoryPage TreeChange TreeChangeSet CommitMetadata")

// Factory results must preserve the selected interface, including factories
// whose spelling is not New*. Never accept a private type just because its
// methods happen to implement the same interface.
var snapshotFactoryResults = map[string]string{
	"kc/snapshot.NewRegistry":                     "Registry",
	"kc/snapshot.NewCommandLedger":                "CommandLedger",
	"kc/snapshot.NewBoltCommandStore":             "CommandStore",
	"kc/snapshot.OpenBoltCommandStore":            "CommandStore",
	"kc/snapshot.NewRawWriter":                    "RawWriter",
	"kc/snapshot.OpenLakeFS":                      "Store",
	"kc/snapshot.CreateManagedLakeFS":             "Store",
	"kc/snapshot.OpenManagedLakeFS":               "Store",
	"kc/snapshot/dataset.NewService":              "Service",
	"kc/snapshot/dataset.OpenSnapshotRegistry":    "Registry",
	"kc/snapshot/dataset.CreateSnapshotRegistry":  "Registry",
	"kc/snapshot/dataset.NewDatasetDeliveredTree": "DatasetDeliveredTree",
	"kc/snapshot/access.NewService":               "Service",
	"kc/snapshot/service.NewHandler":              "Handler",
	"kc/snapshot/service.NewFileView":             "FileView",
	"kc/snapshot/service.NewLakeFSAuthenticator":  "identity.HTTPAuthenticator",
	"kc/snapshot/console.Handler":                 "http.Handler",
}

func names(list string) map[string]bool {
	out := map[string]bool{}
	for _, name := range strings.Fields(list) {
		out[name] = true
	}
	return out
}

var snapshotPublicPackages = map[string]bool{
	"kc/snapshot": true, "kc/snapshot/dataset": true,
	"kc/snapshot/access": true, "kc/snapshot/service": true,
	"kc/snapshot/console": true,
}

// Include tests and nested modules: neither may reopen a private production API.
func walkSnapshotBoundary(t *testing.T, visit func(string, *ast.File)) {
	t.Helper()
	root := moduleRoot(t)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == ".validation" || entry.Name() == ".venv" || entry.Name() == ".kc" || entry.Name() == ".session_tmps" || entry.Name() == "node_modules" || entry.Name() == "webapp" || entry.Name() == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		visit(filepath.ToSlash(rel), file)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotExternalImportsUsePublicContracts(t *testing.T) {
	walkSnapshotBoundary(t, func(path string, file *ast.File) {
		for _, imp := range file.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if name == "kc/catalog" || strings.HasPrefix(name, "kc/catalog/") {
				t.Errorf("%s uses retired Catalog forwarding path %s", path, name)
			}
			if !strings.HasPrefix(path, "snapshot/") && strings.HasPrefix(name, "kc/snapshot/") && !snapshotPublicPackages[name] {
				t.Errorf("%s bypasses Snapshot public contract through %s", path, name)
			}
		}
	})
}

func TestSnapshotContractsDoNotExposePrivateTypes(t *testing.T) {
	walkSnapshotBoundary(t, func(path string, file *ast.File) {
		pkg := "kc/" + filepath.ToSlash(filepath.Dir(path))
		if !snapshotPublicPackages[pkg] || strings.HasSuffix(path, "_test.go") {
			return
		}
		imports := map[string]string{}
		for _, imp := range file.Imports {
			name, _ := strconv.Unquote(imp.Path.Value)
			alias := filepath.Base(name)
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			imports[alias] = name
		}
		inspect := func(node ast.Node) {
			if node == nil {
				return
			}
			ast.Inspect(node, func(n ast.Node) bool {
				selector, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				base, ok := selector.X.(*ast.Ident)
				if !ok {
					return true
				}
				dependency := imports[base.Name]
				pure := pkg == "kc/snapshot" && dependency == "kc/snapshot/internal/storecontract" && snapshotPureDTOAliases[selector.Sel.Name]
				if !pure && (strings.Contains(dependency, "/internal/") || (strings.HasPrefix(dependency, "kc/snapshot/") && !snapshotPublicPackages[dependency])) {
					t.Errorf("%s exposes private type %s.%s (%s)", path, base.Name, selector.Sel.Name, dependency)
				}
				return true
			})
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Name.IsExported() {
					inspect(d.Type)
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							inspect(s.Type)
						}
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if name.IsExported() {
								inspect(s.Type)
								if d.Tok != token.CONST {
									for _, value := range s.Values {
										inspect(value)
									}
								}
								break
							}
						}
					}
				}
			}
		}
	})
}

func TestSnapshotRetiredImplementationsStayAbsent(t *testing.T) {
	root := moduleRoot(t)
	for _, path := range []string{"catalog", "snapshot/gitea", "snapshot/filegit", "snapshot/lakefs", "snapshot/treewriter", "snapshot/commandlog", "snapshot/stamp", "snapshot/internal/giteaimpl", "scripts/system-gitea.sh"} {
		matches, err := filepath.Glob(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) > 0 {
			t.Errorf("retired implementation or alternate contract path remains: %s", path)
		}
	}
	for _, pkg := range []string{"dataset", "access", "service", "console"} {
		matches, err := filepath.Glob(filepath.Join(root, "snapshot", pkg, "contract.go"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 1 {
			t.Errorf("snapshot/%s requires contract.go", pkg)
		}
	}
}

func TestSnapshotPublicValuesAndFactoriesKeepImplementationsPrivate(t *testing.T) {
	packages := map[string][]*ast.File{}
	walkSnapshotBoundary(t, func(path string, file *ast.File) {
		pkg := "kc/" + filepath.ToSlash(filepath.Dir(path))
		if snapshotPublicPackages[pkg] && !strings.HasSuffix(path, "_test.go") {
			packages[pkg] = append(packages[pkg], file)
		}
	})
	for pkg, files := range packages {
		for _, violation := range snapshotSurfaceViolations(pkg, files) {
			t.Error(violation)
		}
	}
}

func snapshotSurfaceViolations(pkg string, files []*ast.File) []string {
	types := map[string]ast.Expr{}
	for _, file := range files {
		for _, decl := range file.Decls {
			general, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range general.Specs {
				if typ, ok := spec.(*ast.TypeSpec); ok {
					types[typ.Name.Name] = typ.Type
				}
			}
		}
	}
	var out []string
	for _, file := range files {
		imports := snapshotImportNames(file)
		inspect := func(symbol string, expr ast.Expr) {
			out = append(out, snapshotSignatureViolations(pkg, symbol, expr, types, imports)...)
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				inspect(d.Name.Name, d.Type)
				out = append(out, snapshotFunctionViolations(pkg, d, types)...)
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if !s.Name.IsExported() {
							continue
						}
						inspect(s.Name.Name, s.Type)
						if st, ok := s.Type.(*ast.StructType); ok {
							out = append(out, snapshotDTOViolations(pkg, s.Name.Name, st)...)
						} else {
							out = append(out, snapshotTypeShapeViolations(pkg, s, imports)...)
						}
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if !name.IsExported() {
								continue
							}
							inspect(name.Name, s.Type)
							if d.Tok == token.VAR && (pkg != "kc/snapshot/dataset" || name.Name != "EmptyState") {
								out = append(out, fmt.Sprintf("%s.%s exposes an unselected mutable variable or function passthrough", pkg, name.Name))
							} else if d.Tok == token.VAR && !snapshotStateDefaultIsData(s) {
								out = append(out, fmt.Sprintf("%s.%s must remain a State data initializer", pkg, name.Name))
							}
						}
					}
				}
			}
		}
	}
	return out
}

func snapshotImportNames(file *ast.File) map[string]string {
	imports := map[string]string{}
	for _, imp := range file.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		alias := filepath.Base(path)
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		imports[alias] = path
	}
	return imports
}

func snapshotSignatureViolations(pkg, symbol string, expr ast.Expr, types map[string]ast.Expr, imports map[string]string) []string {
	var out []string
	var inspect func(ast.Node)
	inspect = func(node ast.Node) {
		if node == nil {
			return
		}
		ast.Inspect(node, func(n ast.Node) bool {
			switch value := n.(type) {
			case *ast.Field:
				inspect(value.Type) // Field/parameter names are not referenced types.
				return false
			case *ast.Ident:
				if _, local := types[value.Name]; local && !value.IsExported() {
					out = append(out, fmt.Sprintf("%s.%s exposes local private type %s", pkg, symbol, value.Name))
				}
			case *ast.SelectorExpr:
				base, ok := value.X.(*ast.Ident)
				if ok && !snapshotSignatureDependencyAllowed(pkg, imports[base.Name], value.Sel.Name) {
					out = append(out, fmt.Sprintf("%s.%s exposes unselected signature dependency %s.%s", pkg, symbol, imports[base.Name], value.Sel.Name))
				}
				return false
			}
			return true
		})
	}
	inspect(expr)
	return out
}

func snapshotSignatureDependencyAllowed(pkg, dependency, symbol string) bool {
	if pkg == "kc/snapshot" && dependency == "kc/snapshot/internal/storecontract" {
		return snapshotPureDTOAliases[symbol]
	}
	if strings.HasPrefix(dependency, "kc/") {
		allowed := map[string]map[string]bool{
			"kc/snapshot":         names("kc/kernel"),
			"kc/snapshot/dataset": names("kc/kernel kc/snapshot"),
			"kc/snapshot/access":  {},
			"kc/snapshot/service": names("kc/identity kc/kernel kc/snapshot kc/snapshot/access kc/snapshot/dataset"),
			"kc/snapshot/console": {},
		}
		return allowed[pkg][dependency]
	}
	// Standard packages have no domain in their first path component.
	return dependency != "" && !strings.Contains(strings.Split(dependency, "/")[0], ".")
}

func snapshotTypeShapeViolations(pkg string, typ *ast.TypeSpec, imports map[string]string) []string {
	if snapshotPublicDTOs[pkg][typ.Name.Name] {
		return []string{fmt.Sprintf("%s.%s must remain a DTO struct", pkg, typ.Name.Name)}
	}
	switch shape := typ.Type.(type) {
	case *ast.InterfaceType, *ast.FuncType:
		return nil
	case *ast.Ident:
		if names("bool string int int8 int16 int32 int64 uint uint8 uint16 uint32 uint64 uintptr byte rune float32 float64 complex64 complex128")[shape.Name] {
			return nil
		}
	case *ast.SelectorExpr:
		base, ok := shape.X.(*ast.Ident)
		if ok && pkg == "kc/snapshot" && imports[base.Name] == "kc/snapshot/internal/storecontract" && snapshotPureDTOAliases[shape.Sel.Name] {
			return nil
		}
	}
	return []string{fmt.Sprintf("%s.%s exposes an unselected concrete public type shape", pkg, typ.Name.Name)}
}

func snapshotStateDefaultIsData(value *ast.ValueSpec) bool {
	if len(value.Values) != 1 {
		return false
	}
	data, ok := value.Values[0].(*ast.CompositeLit)
	return ok && snapshotTypeName(data.Type) == "State"
}

func snapshotDTOViolations(pkg, name string, st *ast.StructType) []string {
	if !snapshotPublicDTOs[pkg][name] {
		return []string{fmt.Sprintf("%s.%s exposes an unselected concrete behavior/data type", pkg, name)}
	}
	var out []string
	for _, field := range st.Fields.List {
		for _, fieldName := range field.Names {
			// Resources carries one private callback copied from its public Around
			// assembly port. It is not a second authority or storage state.
			if !fieldName.IsExported() && !(pkg == "kc/snapshot/service" && name == "Resources" && fieldName.Name == "around") {
				out = append(out, fmt.Sprintf("%s.%s embeds private state %s in a public DTO", pkg, name, fieldName.Name))
			}
		}
	}
	return out
}

func snapshotFunctionViolations(pkg string, fn *ast.FuncDecl, types map[string]ast.Expr) []string {
	var out []string
	if fn.Recv != nil {
		name := strings.TrimPrefix(snapshotTypeName(fn.Recv.List[0].Type), "*")
		if _, concrete := types[name].(*ast.StructType); concrete && ast.IsExported(name) && !snapshotDTOMethods[pkg+"."+name+"."+fn.Name.Name] {
			out = append(out, fmt.Sprintf("%s.%s.%s adds behavior to an exposed concrete type", pkg, name, fn.Name.Name))
		}
		return out
	}
	key := pkg + "." + fn.Name.Name
	want, selected := snapshotFactoryResults[key]
	constructor := strings.HasPrefix(fn.Name.Name, "New") || strings.HasPrefix(fn.Name.Name, "Open") || strings.HasPrefix(fn.Name.Name, "Create")
	if constructor && !selected {
		return []string{fmt.Sprintf("%s is an unselected public factory", key)}
	}
	if selected && (fn.Type.Results == nil || len(fn.Type.Results.List) == 0 || snapshotTypeName(fn.Type.Results.List[0].Type) != want) {
		out = append(out, fmt.Sprintf("%s must return selected interface %s", key, want))
	}
	if selected && !strings.Contains(want, ".") {
		if _, isInterface := types[want].(*ast.InterfaceType); !isInterface {
			out = append(out, fmt.Sprintf("%s result %s must be a declared interface", key, want))
		}
	}
	return out
}

func snapshotTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return snapshotTypeName(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + snapshotTypeName(t.X)
	}
	return ""
}

func TestSnapshotNeverExposesRawBackendOrHostCheckoutEscape(t *testing.T) {
	walkSnapshotBoundary(t, func(path string, file *ast.File) {
		if !strings.HasPrefix(path, "snapshot/") {
			return
		}
		ast.Inspect(file, func(node ast.Node) bool {
			var name string
			switch n := node.(type) {
			case *ast.FuncDecl:
				name = n.Name.Name
			case *ast.Field:
				if _, callable := n.Type.(*ast.FuncType); callable && len(n.Names) == 1 {
					name = n.Names[0].Name
				}
			}
			if name == "Raw" || name == "RootDir" {
				t.Errorf("%s exposes retired %s escape", path, name)
			}
			return true
		})
	})
}

func TestSnapshotPublicSurfaceGuardDetectsIndirectLeaks(t *testing.T) {
	for _, tc := range []struct{ name, source, signal string }{
		{"private-result", "type writer struct{}; func Write() *writer { return nil }", "local private type writer"},
		{"concrete-factory", "type Registry struct{}; func NewRegistry() *Registry { return nil }", "selected interface Registry"},
		{"function-variable", "var OpenLakeFS = backend.OpenExisting", "function passthrough"},
		{"private-alias", "type writer struct{}; type Writer = writer", "local private type writer"},
		{"private-embedded-type", "type transport struct{}; type FileRef struct { *transport }", "local private type transport"},
		{"stateful-dto", "type FileRef struct { state bool }", "private state"},
		{"dto-authority-method", "type FileRef struct{}; func (f FileRef) Save() {}", "adds behavior"},
		{"dto-pointer-authority-method", "type FileRef struct{}; func (f *FileRef) Save() {}", "adds behavior"},
		{"concrete-interface-alias", "type Registry string; func NewRegistry() Registry { return \"\" }", "must be a declared interface"},
		{"external-concrete-alias", `import "net/http"; type Client = http.Client`, "concrete public type shape"},
		{"upper-layer-signature", `import "kc/knowledge"; type FileRef struct{ Value knowledge.Object }`, "unselected signature dependency"},
		{"selected-data-and-ports", `import contracts "kc/snapshot/internal/storecontract"; type FileRef struct{Path string}; type TreeChange = contracts.TreeChange; type WriteObserver func(string) error; type TreeReader interface {ReadFile(string) ([]byte,error)}; const Driver = contracts.DefaultRef`, ""},
		{"capability-interface-discovery", `type TreeReader interface { ReadFile(string) ([]byte,error) }; func TreeReaderOf(store any) (TreeReader,bool) { reader,ok := store.(TreeReader); return reader,ok }`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "surface.go", "package snapshot; "+tc.source, 0)
			if err != nil {
				t.Fatal(err)
			}
			violations := strings.Join(snapshotSurfaceViolations("kc/snapshot", []*ast.File{file}), "\n")
			if tc.signal == "" && violations != "" {
				t.Fatalf("legitimate contract rejected: %s", violations)
			}
			if tc.signal != "" && !strings.Contains(violations, tc.signal) {
				t.Fatalf("missing %q signal: %s", tc.signal, violations)
			}
		})
	}
}

func TestSnapshotSharedContractDTOsRemainPure(t *testing.T) {
	walkSnapshotBoundary(t, func(path string, file *ast.File) {
		if !strings.HasPrefix(path, "snapshot/internal/storecontract/") || strings.HasSuffix(path, "_test.go") {
			return
		}
		types := map[string]ast.Expr{}
		for _, decl := range file.Decls {
			if general, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range general.Specs {
					if typ, ok := spec.(*ast.TypeSpec); ok {
						types[typ.Name.Name] = typ.Type
					}
				}
			}
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil && snapshotPureDTOAliases[strings.TrimPrefix(snapshotTypeName(d.Recv.List[0].Type), "*")] {
					t.Errorf("%s adds implementation method %s to a shared pure DTO", path, d.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					typ, ok := spec.(*ast.TypeSpec)
					if !ok || !snapshotPureDTOAliases[typ.Name.Name] {
						continue
					}
					st, ok := typ.Type.(*ast.StructType)
					if !ok {
						t.Errorf("%s.%s must remain a pure DTO struct", path, typ.Name.Name)
						continue
					}
					for _, violation := range snapshotSignatureViolations("kc/snapshot", typ.Name.Name, st, types, snapshotImportNames(file)) {
						t.Error(violation)
					}
					ast.Inspect(st, func(node ast.Node) bool {
						switch node.(type) {
						case *ast.FuncType, *ast.InterfaceType, *ast.ChanType:
							t.Errorf("%s.%s carries a behavior port in a shared literal DTO", path, typ.Name.Name)
						}
						return true
					})
					for _, field := range st.Fields.List {
						if len(field.Names) == 0 {
							t.Errorf("%s.%s embeds an implementation in a shared DTO", path, typ.Name.Name)
						}
						for _, name := range field.Names {
							if !name.IsExported() {
								t.Errorf("%s.%s retains private state %s", path, typ.Name.Name, name.Name)
							}
						}
					}
				}
			}
		}
	})
}
