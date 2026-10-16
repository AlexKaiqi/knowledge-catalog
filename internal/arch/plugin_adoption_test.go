package arch_test

import (
	"fmt"
	"go/ast"
	"path/filepath"
	"slices"
	"testing"
)

var pluginAdoptionSurface = []string{
	"PluginAffordance", "PluginConfig", "PluginInstanceConfig", "PluginScope", "PluginSelection",
	"PluginsConfig", "SourcePlugin",
}

func TestPluginAdoptionContractsHaveOneLowLevelDefinition(t *testing.T) {
	for _, signal := range pluginAdoptionOwnershipSignals(productionSourceFiles(t, nil)) {
		t.Error(signal)
	}
}

func TestPluginAdoptionCannotDependOnConsumersOrExecution(t *testing.T) {
	if layer, declared := architectureLayer("discovery/adoption"); !declared || layer != "base" {
		t.Fatalf("common adoption contract has no explicit base layer: %q %v", layer, declared)
	}
	for _, signal := range pluginAdoptionDependencySignals(loadGraph(t)) {
		t.Error(signal)
	}
}

func pluginAdoptionDependencySignals(graph depGraph) []string {
	var signals []string
	for target, trail := range graph.reachable("discovery/adoption") {
		if target != "kernel" {
			signals = append(signals, fmt.Sprintf("common adoption contract reaches execution or a consumer through %v", trail))
		}
	}
	return signals
}

func TestLiveAccessConsumesCommonAdoptionWithoutIndexRegistry(t *testing.T) {
	graph := loadGraph(t)
	for _, owner := range []string{"liveaccess/service", "liveaccess/plugin", "liveaccess/cli"} {
		reach := graph.reachable(owner)
		if _, shared := reach["discovery/adoption"]; !shared {
			t.Errorf("%s does not consume its shared adoption contract", owner)
		}
		for _, denied := range []string{"discovery/plugin", "discovery/projection", "declarativeindex"} {
			if trail, blocked := reachDenied(reach, denied); blocked {
				t.Errorf("LiveAccess %s reaches the index registry/runtime through %v", owner, trail)
			}
		}
	}
}

func pluginAdoptionOwnershipSignals(files map[string]*ast.File) []string {
	found, public := map[string]int{}, map[string]bool{}
	var signals []string
	for path, file := range files {
		owner := filepath.ToSlash(filepath.Dir(path))
		ast.Inspect(file, func(node ast.Node) bool {
			names, concrete := liveAccessDeclaredNames(node)
			if spec, ok := node.(*ast.TypeSpec); ok {
				// A named map or struct type is a definition; only aliases copy.
				concrete = !spec.Assign.IsValid()
			}
			for _, name := range names {
				if owner == "discovery/adoption" && ast.IsExported(name) {
					public[name] = true
				}
				if !slices.Contains(pluginAdoptionSurface, name) {
					continue
				}
				if owner == "discovery/adoption" {
					found[name]++
					if !concrete {
						signals = append(signals, fmt.Sprintf("%s must own the concrete shared adoption %s", path, name))
					}
				} else if !pluginAdoptionCompatibleDeclaration(owner, node, file, name) {
					signals = append(signals, fmt.Sprintf("%s copies or misroutes shared adoption %s", path, name))
				}
			}
			return true
		})
	}
	for _, name := range pluginAdoptionSurface {
		if found[name] != 1 {
			signals = append(signals, fmt.Sprintf("shared adoption %s must have one definition, found %d", name, found[name]))
		}
	}
	for name := range public {
		if !slices.Contains(pluginAdoptionSurface, name) {
			signals = append(signals, "common adoption exported surface adds unselected "+name)
		}
	}
	slices.Sort(signals)
	return signals
}

func pluginAdoptionCompatibleDeclaration(owner string, node ast.Node, file *ast.File, name string) bool {
	allowed := owner == "discovery/plugin" || name == "PluginAffordance" && (owner == "liveaccess/cli" || owner == "declarativeindex/capability")
	if !allowed {
		return false
	}
	switch declared := node.(type) {
	case *ast.TypeSpec:
		return declared.Assign.IsValid() && pluginAdoptionSelector(declared.Type, file, name)
	case *ast.FuncDecl:
		if declared.Body == nil || len(declared.Body.List) != 1 {
			return false
		}
		returned, ok := declared.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(returned.Results) != 1 {
			return false
		}
		call, ok := returned.Results[0].(*ast.CallExpr)
		return ok && pluginAdoptionSelector(call.Fun, file, name)
	}
	return false
}

func pluginAdoptionSelector(expression ast.Expr, file *ast.File, name string) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && snapshotImportNames(file)[pkg.Name] == "kc/discovery/adoption"
}
