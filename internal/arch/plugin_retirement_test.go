package arch_test

import (
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

// Retired per-Repository plugin selection (accept-retire-repository-plugin-selection):
// no function or type selects instances per Repository, no code reads or writes
// the plugin choice in Repository metadata, and no scene DSL names an access
// instance for one Repository. Instances come only from deployment.
var retiredRepositoryPluginSelection = []string{
	"SelectRepositoryPlugins", "RepositoryPluginsKey", "RepositoryPluginConfig", "RepositoryPluginConfigs",
	"ParseRepositoryPlugins", "AdoptRepository",
}

// Spelled in two parts so the guard's own definition is not a reintroduction.
const retiredRepositoryPluginMetadataKey = "kc." + "plugins"

const retiredSceneAccessSelection = "with LiveAccess instance"

func TestRepositoryPluginSelectionIsRetiredFromCodeAndScenes(t *testing.T) {
	root := moduleRoot(t)
	var signals []string
	err := walkRepositorySource(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
		switch {
		case strings.HasSuffix(path, ".go"):
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			signals = append(signals, repositoryPluginSelectionSignals(relative, file)...)
		case strings.HasSuffix(path, ".feature"):
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			signals = append(signals, sceneAccessSelectionSignals(relative, string(raw))...)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, signal := range signals {
		t.Error(signal)
	}
}

func TestRepositoryPluginSelectionGuardRejectsReintroduction(t *testing.T) {
	for _, fixture := range []struct{ path, source, signal string }{
		{"discovery/adoption/config.go", "package adoption; func SelectRepositoryPlugins() {}", "declares retired SelectRepositoryPlugins"},
		{"home/plugins.go", "package home; type RepositoryPluginConfig struct{}", "declares retired RepositoryPluginConfig"},
		{"cli/fixture_test.go", "package cli; import p \"kc/discovery/plugin\"; var _ = p.RepositoryPluginsKey", "references retired RepositoryPluginsKey"},
		{"liveaccess/fixture/adoption.go", "package fixture; func AdoptRepository() {}", "declares retired AdoptRepository"},
		{"home/stores.go", "package home; var key = \"kc.plugins\"", "Repository metadata plugin key"},
	} {
		t.Run(fixture.signal, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), fixture.path, fixture.source, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			if signals := repositoryPluginSelectionSignals(fixture.path, file); !slices.ContainsFunc(signals, func(s string) bool { return strings.Contains(s, fixture.signal) }) {
				t.Fatalf("retirement guard did not report %q: %v", fixture.signal, signals)
			}
		})
	}
	if signals := sceneAccessSelectionSignals("x/_build/construct.feature", "Given existing repository kr://a with LiveAccess instance main\n"); len(signals) == 0 {
		t.Fatal("retirement guard accepted a per-Repository LiveAccess scene selection")
	}
	clean, err := parser.ParseFile(token.NewFileSet(), "home/plugins.go", "package home; type PluginsConfig map[string]int; var s = \"kc.plugins.extra\"", parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	if signals := repositoryPluginSelectionSignals("home/plugins.go", clean); len(signals) != 0 {
		t.Fatalf("deployment plugin configuration was reported as retired selection: %v", signals)
	}
}

func repositoryPluginSelectionSignals(path string, file *ast.File) []string {
	var signals []string
	ast.Inspect(file, func(node ast.Node) bool {
		switch item := node.(type) {
		case *ast.FuncDecl:
			if slices.Contains(retiredRepositoryPluginSelection, item.Name.Name) {
				signals = append(signals, path+" declares retired "+item.Name.Name)
			}
		case *ast.TypeSpec:
			if slices.Contains(retiredRepositoryPluginSelection, item.Name.Name) {
				signals = append(signals, path+" declares retired "+item.Name.Name)
			}
		case *ast.ValueSpec:
			for _, name := range item.Names {
				if slices.Contains(retiredRepositoryPluginSelection, name.Name) {
					signals = append(signals, path+" declares retired "+name.Name)
				}
			}
		case *ast.SelectorExpr:
			if slices.Contains(retiredRepositoryPluginSelection, item.Sel.Name) {
				signals = append(signals, path+" references retired "+item.Sel.Name)
			}
		case *ast.BasicLit:
			if item.Kind == token.STRING {
				if value, err := strconv.Unquote(item.Value); err == nil && value == retiredRepositoryPluginMetadataKey {
					signals = append(signals, path+" names the retired Repository metadata plugin key")
				}
			}
		}
		return true
	})
	return signals
}

func sceneAccessSelectionSignals(path, content string) []string {
	var signals []string
	for number, line := range strings.Split(content, "\n") {
		if strings.Contains(line, retiredSceneAccessSelection) {
			signals = append(signals, path+":"+strconv.Itoa(number+1)+" selects a LiveAccess instance for one Repository")
		}
	}
	return signals
}
