package arch_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	accesscli "kc/liveaccess/cli"
)

// Goal: enforce resource-access U13, L-01 and API-01 ownership of the
// LiveAccess core, request services and three module CLI leaves.
// Non-goals: replace behavior contracts, forbid host assembly or move Object
// reading into LiveAccess. Existing Object and layer guards remain in force.
func TestLiveAccessConcreteImplementationsStayInModule(t *testing.T) {
	for _, signal := range liveAccessOwnershipSignals(liveAccessSourceFiles(t)) {
		t.Error(signal)
	}
}

func TestLiveAccessRuntimeDependencyBoundaries(t *testing.T) {
	for _, signal := range liveAccessDependencySignals(loadGraph(t)) {
		t.Error(signal)
	}
}

func TestLiveAccessCLILeafResponsibilitiesStayWithModule(t *testing.T) {
	owned := liveAccessCLILeaves(t)
	seen := map[string]bool{}
	for path, file := range liveAccessSourceFiles(t) {
		signals, boundaries := snapshotCLIHostOwnershipSignals(path, file, owned)
		for boundary := range boundaries {
			seen[boundary] = true
		}
		for _, signal := range signals {
			t.Errorf("%s: %s", path, strings.ReplaceAll(signal, "Snapshot", "LiveAccess"))
		}
	}
	for _, boundary := range []string{"command surface", "helpEntries", "helpGroups", "applyPositionals", "runRemoteRequest", "encodeCLIProduct"} {
		if !seen[boundary] {
			t.Errorf("LiveAccess CLI guard cannot find host boundary %s", boundary)
		}
	}
}

func TestLiveAccessLayerAssignmentsAreExplicit(t *testing.T) {
	for pkg, want := range map[string]string{
		"liveaccess": "discovery", "liveaccess/discovery": "app", "liveaccess/contract": "object",
		"liveaccess/evidence": "object", "liveaccess/service": "app", "liveaccess/plugin": "app",
		"liveaccess/client": "app", "liveaccess/cli": "app", "liveaccess/fixture": "app",
	} {
		if got, declared := architectureLayer(pkg); !declared || got != want {
			t.Errorf("%s layer=%q declared=%v want=%q", pkg, got, declared, want)
		}
	}
	if _, allowed := architectureLayer("liveaccess/internal/undeclared"); allowed {
		t.Error("LiveAccess internal packages must not inherit a layer by prefix")
	}
}

func liveAccessCLILeaves(t *testing.T) snapshotCLILeaves {
	t.Helper()
	want := map[string]string{"binding show": "object-binding-show", "access": "object-access", "invoke": "object-invoke"}
	owned := snapshotCLILeaves{paths: map[string]bool{}, handlers: map[string]bool{}}
	for _, command := range accesscli.Commands() {
		if want[command.Path] != command.Handler || owned.paths[command.Path] {
			t.Fatalf("LiveAccess module has an unexpected or duplicate CLI leaf: %#v", command)
		}
		owned.paths[command.Path], owned.handlers[command.Handler] = true, true
	}
	if len(owned.paths) != len(want) {
		t.Fatalf("LiveAccess module lost selected CLI leaves: %v", owned.paths)
	}
	return owned
}

func liveAccessSourceFiles(t *testing.T) map[string]*ast.File {
	t.Helper()
	return productionSourceFiles(t, liveAccessOwnershipScope)
}

func productionSourceFiles(t *testing.T, selected func(string) bool) map[string]*ast.File {
	t.Helper()
	files := map[string]*ast.File{}
	for owner := range loadGraph(t) {
		if selected != nil && !selected(owner) {
			continue
		}
		paths, err := filepath.Glob(filepath.Join(moduleRoot(t), owner, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			files[filepath.ToSlash(filepath.Join(owner, filepath.Base(path)))] = file
		}
	}
	return files
}

func liveAccessOwnershipScope(owner string) bool {
	for _, scope := range []string{"liveaccess", "object", "discovery/plugin", "discovery/adoption", "declarativeindex/cli", "invocation", "cli"} {
		if deniedMatch(scope, owner) {
			return true
		}
	}
	return false
}

func liveAccessOwnershipSignals(files map[string]*ast.File) []string {
	owners := map[string]string{
		"HTTPStateLookup": "liveaccess", "NewHTTPStateLookup": "liveaccess", "NewHTTPStateLookupWithOptions": "liveaccess",
		"LiveAccessHandler": "liveaccess", "NewLiveAccessHandler": "liveaccess", "RegisterLiveAccess": "liveaccess", "WithLiveAccess": "liveaccess",
		"ResolvedBinding": "liveaccess", "ResolveBinding": "liveaccess", "ResolveDeclaration": "liveaccess",
		"Plugins": "liveaccess/service", "OpenPlugins": "liveaccess/service", "ValueFailure": "liveaccess/service",
		"HydrateRepositoryRead": "liveaccess/service", "BeginReadEvidence": "liveaccess/service", "RetainReadEvidence": "liveaccess/service",
		"OpenResultStore": "liveaccess/service", "AuditEntries": "liveaccess/service", "Collector": "liveaccess/evidence", "NewCollector": "liveaccess/evidence",
	}
	found := map[string][]string{}
	var signals []string
	for path, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			names, concrete := liveAccessDeclaredNames(node)
			for _, name := range names {
				owner, selected := owners[name]
				if !selected {
					continue
				}
				found[name] = append(found[name], path)
				if filepath.ToSlash(filepath.Dir(path)) != owner || !concrete {
					signals = append(signals, fmt.Sprintf("%s must define concrete %s in %s; aliases and forwarding values do not own implementations", path, name, owner))
				}
			}
			return true
		})
	}
	for name, owner := range owners {
		if len(found[name]) != 1 {
			signals = append(signals, fmt.Sprintf("%s must have one concrete %s declaration, found %v", owner, name, found[name]))
		}
	}
	slices.Sort(signals)
	return signals
}

func liveAccessDeclaredNames(node ast.Node) ([]string, bool) {
	switch declared := node.(type) {
	case *ast.TypeSpec:
		_, concrete := declared.Type.(*ast.StructType)
		return []string{declared.Name.Name}, concrete && !declared.Assign.IsValid()
	case *ast.FuncDecl:
		if declared.Recv == nil {
			return []string{declared.Name.Name}, declared.Body != nil
		}
	case *ast.ValueSpec:
		names := make([]string, 0, len(declared.Names))
		for _, name := range declared.Names {
			names = append(names, name.Name)
		}
		return names, false
	}
	return nil, false
}

func liveAccessDependencySignals(graph depGraph) []string {
	var signals []string
	// Preserve every existing core rule that denies the whole LiveAccess owner,
	// including its subpackages. New CLI/SDK checks do not replace this set.
	for _, rule := range forbidden {
		if slices.Contains(rule.denied, "liveaccess") {
			if trail, blocked := reachDenied(graph.reachable(rule.pkg), "liveaccess"); blocked {
				signals = append(signals, fmt.Sprintf("base consumer %s reaches LiveAccess through %v", rule.pkg, trail))
			}
		}
	}
	for _, consumer := range []string{"liveaccess/cli", "liveaccess/client"} {
		for target, trail := range graph.reachable(consumer) {
			if liveAccessRuntimePackage(target) || liveAccessHostOrRuntime(target) {
				signals = append(signals, fmt.Sprintf("independent %s reaches runtime %s through %v", consumer, target, trail))
			}
		}
	}
	for _, owner := range []string{"liveaccess/service", "liveaccess/plugin", "liveaccess/evidence", "liveaccess/handler"} {
		for _, target := range []string{"home", "invocation", "server", "httpsurface", "cli", "client", "identity", "object/writer", "discovery/plugin"} {
			if trail, blocked := reachDenied(graph.reachable(owner), target); blocked {
				signals = append(signals, fmt.Sprintf("LiveAccess owner %s depends on host %s through %v", owner, target, trail))
			}
		}
	}
	slices.Sort(signals)
	return signals
}

func liveAccessRuntimePackage(pkg string) bool {
	return deniedMatch("liveaccess", pkg) && !deniedMatch("liveaccess/client", pkg) && !deniedMatch("liveaccess/cli", pkg) && !deniedMatch("liveaccess/contract", pkg)
}

func liveAccessHostOrRuntime(pkg string) bool {
	for _, target := range []string{"home", "invocation", "server", "cli", "client", "identity", "object/service", "object/reader", "object/writer", "object/maintenance", "snapshot/service", "discovery/service", "discovery/plugin", "declarativeindex/plugin", "declarativeindex/control", "declarativeindex/projection"} {
		if deniedMatch(target, pkg) {
			return true
		}
	}
	return false
}
