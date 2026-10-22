package arch_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// This checks source ownership, not whether a renamed registration entry can
// still reach an implementation that remains in the generic contract package.
func TestDeclarativeIndexOwnsConcreteProjectionAndControl(t *testing.T) {
	files := declarativeIndexSourceFiles(t, false)
	for _, signal := range declarativeIndexOwnershipSignals(files) {
		t.Error(signal)
	}
	for _, retired := range []string{"discovery/opensearch", "discovery/internal/opensearchimpl", "declarativeindex/internal/opensearchimpl"} {
		if _, err := os.Stat(filepath.Join(moduleRoot(t), retired)); err == nil {
			t.Errorf("retired concrete index owner %s still exists", retired)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}

// Shared request scopes must be the same context identity for the generic
// fan-out and the chosen provider. A copied helper silently resets that scope.
func TestDeclarativeIndexKeepsSharedProjectionContractsSingle(t *testing.T) {
	for _, signal := range declarativeIndexSharedContractSignals(declarativeIndexSourceFiles(t, false)) {
		t.Error(signal)
	}
}

func TestDeclarativeIndexImplementationDependencyBoundaries(t *testing.T) {
	for _, signal := range declarativeIndexDependencySignals(loadGraph(t)) {
		t.Error(signal)
	}
}

func TestDeclarativeIndexConcreteTestsFollowImplementationOwner(t *testing.T) {
	for path, file := range declarativeIndexSourceFiles(t, true) {
		for _, signal := range declarativeIndexGenericTestSignals(path, file) {
			t.Error(signal)
		}
	}
}

func TestDeclarativeIndexControlDoesNotOwnObjectCoordinator(t *testing.T) {
	for path, file := range declarativeIndexSourceFiles(t, false) {
		for _, signal := range declarativeIndexObjectCoordinatorSignals(path, file) {
			t.Error(signal)
		}
	}
}

func declarativeIndexSourceFiles(t *testing.T, tests bool) map[string]*ast.File {
	t.Helper()
	root := moduleRoot(t)
	files := map[string]*ast.File{}
	for _, owner := range []string{"discovery", "declarativeindex"} {
		err := filepath.WalkDir(filepath.Join(root, owner), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") != tests {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			relative, _ := filepath.Rel(root, path)
			files[filepath.ToSlash(relative)] = file
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}

func declarativeIndexGenericTestSignals(path string, file *ast.File) []string {
	owner := filepath.ToSlash(filepath.Dir(path))
	if !deniedMatch("discovery/projection", owner) && !deniedMatch("discovery/plugin", owner) {
		return nil
	}
	var signals []string
	for _, imported := range file.Imports {
		pkg, _ := strconv.Unquote(imported.Path.Value)
		if declarativeIndexRuntimePackage(strings.TrimPrefix(pkg, "kc/")) {
			signals = append(signals, fmt.Sprintf("%s tests concrete runtime %s outside its implementation owner", path, pkg))
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		var symbol string
		switch value := node.(type) {
		case *ast.CallExpr:
			if id, ok := value.Fun.(*ast.Ident); ok {
				symbol = id.Name
			} else if selector, ok := value.Fun.(*ast.SelectorExpr); ok {
				symbol = selector.Sel.Name
			}
		case *ast.CompositeLit:
			if id, ok := value.Type.(*ast.Ident); ok {
				symbol = id.Name
			} else if selector, ok := value.Type.(*ast.SelectorExpr); ok {
				symbol = selector.Sel.Name
			}
		}
		if slices.Contains([]string{"Index", "Controller", "TargetStore", "NewIndexEngine", "NewController", "NewTargetStore", "PlanRetrieval"}, symbol) {
			signals = append(signals, fmt.Sprintf("%s constructs concrete %s outside declarativeindex implementation tests", path, symbol))
		}
		return true
	})
	return signals
}

func declarativeIndexObjectCoordinatorSignals(path string, file *ast.File) []string {
	if !deniedMatch("declarativeindex/control", filepath.ToSlash(filepath.Dir(path))) {
		return nil
	}
	aliases := map[string]bool{}
	for _, imported := range file.Imports {
		pkg, _ := strconv.Unquote(imported.Path.Value)
		if pkg != "kc/object/maintenance" {
			continue
		}
		name := "maintenance"
		if imported.Name != nil {
			name = imported.Name.Name
		}
		aliases[name] = true
	}
	var signals []string
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || (selector.Sel.Name != "Coordinator" && selector.Sel.Name != "NewCoordinator") {
			return true
		}
		if id, ok := selector.X.(*ast.Ident); ok && aliases[id.Name] {
			signals = append(signals, fmt.Sprintf("%s owns Object maintenance %s inside index control; compose independent consumers at the plugin/host", path, selector.Sel.Name))
		}
		return true
	})
	return signals
}

func declarativeIndexOwnershipSignals(files map[string]*ast.File) []string {
	var signals []string
	implementations := map[string]string{
		"Index": "declarativeindex/projection", "NewIndexEngine": "declarativeindex/projection",
		"ProjectionSpec": "declarativeindex/projection", "RetrievalPlan": "declarativeindex/projection",
		"RetrievalFragment": "declarativeindex/projection", "PlanRetrieval": "declarativeindex/projection",
		"Controller": "declarativeindex/control", "NewController": "declarativeindex/control",
		"TargetStore": "declarativeindex/control", "NewTargetStore": "declarativeindex/control",
	}
	found := map[string][]string{}
	for path, file := range files {
		owner := filepath.ToSlash(filepath.Dir(path))
		if deniedMatch("discovery/opensearch", owner) || deniedMatch("discovery/internal/opensearchimpl", owner) || deniedMatch("declarativeindex/internal/opensearchimpl", owner) {
			signals = append(signals, fmt.Sprintf("%s retains a retired concrete index implementation or adapter shim", path))
		}
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if value.Recv == nil {
					if expected, concrete := implementations[value.Name.Name]; concrete {
						found[value.Name.Name] = append(found[value.Name.Name], path)
						if owner != expected {
							signals = append(signals, fmt.Sprintf("%s declares concrete factory %s outside %s", path, value.Name, expected))
						}
					}
				} else if deniedMatch("discovery/projection", owner) {
					for _, receiver := range value.Recv.List {
						ast.Inspect(receiver.Type, func(node ast.Node) bool {
							if id, ok := node.(*ast.Ident); ok && implementations[id.Name] != "" {
								signals = append(signals, fmt.Sprintf("%s retains concrete %s method %s", path, id.Name, value.Name))
							}
							return true
						})
					}
				}
			case *ast.GenDecl:
				for _, spec := range value.Specs {
					switch declared := spec.(type) {
					case *ast.TypeSpec:
						if expected, concrete := implementations[declared.Name.Name]; concrete {
							found[declared.Name.Name] = append(found[declared.Name.Name], path)
							_, actualStruct := declared.Type.(*ast.StructType)
							if owner != expected || declared.Assign.IsValid() || !actualStruct {
								signals = append(signals, fmt.Sprintf("%s must define the real %s struct in %s, without an implementation alias", path, declared.Name, expected))
							}
						}
					case *ast.ValueSpec:
						for _, name := range declared.Names {
							if expected, concrete := implementations[name.Name]; concrete {
								found[name.Name] = append(found[name.Name], path)
								signals = append(signals, fmt.Sprintf("%s retains %s as a factory forwarding value instead of its real owner %s", path, name, expected))
							}
						}
					}
				}
			}
		}
	}
	for name, owner := range implementations {
		if len(found[name]) != 1 {
			signals = append(signals, fmt.Sprintf("%s must have exactly one concrete %s declaration, found %v", owner, name, found[name]))
		}
	}
	slices.Sort(signals)
	return signals
}

func declarativeIndexSharedContractSignals(files map[string]*ast.File) []string {
	types := []string{"IndexDescriptor", "IndexSync", "StateSync", "ObservationSource", "ChangeNotice", "SearchBudget"}
	functions := []string{"WithSearchBudget", "SearchBudgetExhausted", "WithCandidateScope"}
	found := map[string][]string{}
	var signals []string
	for path, file := range files {
		owner := filepath.ToSlash(filepath.Dir(path))
		for _, declaration := range file.Decls {
			var names []string
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if value.Recv == nil && slices.Contains(functions, value.Name.Name) {
					names = append(names, value.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range value.Specs {
					switch declared := spec.(type) {
					case *ast.TypeSpec:
						if slices.Contains(types, declared.Name.Name) && !declared.Assign.IsValid() {
							names = append(names, declared.Name.Name)
						}
					case *ast.ValueSpec:
						for _, name := range declared.Names {
							if slices.Contains(functions, name.Name) {
								names = append(names, name.Name)
							}
						}
					}
				}
			}
			for _, name := range names {
				found[name] = append(found[name], path)
				if owner != "discovery/projection" {
					signals = append(signals, fmt.Sprintf("%s duplicates or relocates shared projection contract %s", path, name))
				}
			}
		}
	}
	for _, name := range append(types, functions...) {
		if len(found[name]) != 1 {
			signals = append(signals, fmt.Sprintf("shared discovery/projection contract %s must have one definition, found %v", name, found[name]))
		}
	}
	slices.Sort(signals)
	return signals
}

func declarativeIndexRuntimePackage(pkg string) bool {
	return deniedMatch("declarativeindex", pkg) && !deniedMatch("declarativeindex/client", pkg) && !deniedMatch("declarativeindex/cli", pkg) && !deniedMatch("declarativeindex/capability", pkg)
}

func declarativeIndexDependencySignals(graph depGraph) []string {
	var signals []string
	for owner, dependencies := range graph {
		// Home constructs contributions. Its configuration subpackage names the
		// provider config shape a deployment selects, which is still a
		// declaration, not an engine. Other external production packages use
		// the generic ports or owned SDKs, including the HTTP/CLI transports.
		if owner != "home" && owner != "home/config" && (!deniedMatch("declarativeindex", owner) || deniedMatch("declarativeindex/client", owner) || deniedMatch("declarativeindex/cli", owner) || deniedMatch("declarativeindex/capability", owner)) {
			for _, target := range dependencies {
				if declarativeIndexRuntimePackage(target) {
					signals = append(signals, fmt.Sprintf("%s directly imports concrete index implementation %s outside assembly", owner, target))
				}
			}
		}
		generic := deniedMatch("discovery", owner) && !deniedMatch("discovery/opensearch", owner) && !deniedMatch("discovery/internal/opensearchimpl", owner)
		client := slices.Contains([]string{"client", "object/client", "object/cli", "snapshot/client", "snapshot/cli", "declarativeindex/client", "declarativeindex/cli", "declarativeindex/capability"}, owner)
		if generic || client {
			for target, trail := range graph.reachable(owner) {
				if declarativeIndexRuntimePackage(target) {
					signals = append(signals, fmt.Sprintf("generic consumer %s reaches concrete index implementation through %v", owner, trail))
				}
			}
		}
		if deniedMatch("declarativeindex/projection", owner) {
			if trail, found := reachDenied(graph.reachable(owner), "declarativeindex/control"); found {
				signals = append(signals, fmt.Sprintf("index query implementation %s depends on control through %v", owner, trail))
			}
		}
	}
	slices.Sort(signals)
	return signals
}
