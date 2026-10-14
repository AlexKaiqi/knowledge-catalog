package arch_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// retiredPaths are capabilities the design removed. They must not come back
// under the old name; a successor gets a new name and a design entry.
var retiredPaths = []string{
	"catalog", "repository", "retrieval", "index", "interfaces",
	"connector", "integrationruntime", "cmd/kc-integration",
	"snapshot/gitea", "snapshot/filegit", "snapshot/lakefs", "snapshot/treewriter",
	"snapshot/commandlog", "snapshot/stamp", "snapshot/internal/giteaimpl",
	"scripts/system-gitea.sh", "identity/legacy.go", "cli/identity_migration.go",
}

func TestRetiredPathsStayAbsent(t *testing.T) {
	root := moduleRoot(t)
	for _, path := range retiredPaths {
		if _, err := os.Stat(filepath.Join(root, path)); err == nil {
			t.Errorf("retired path returned: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}

// Every package a guard names must exist (exactly or as the owner of a live
// subpackage). A rule about a deleted package silently checks nothing.
func TestGuardRulesNameLivePackages(t *testing.T) {
	graph := loadGraph(t)
	live := func(name string) bool {
		if _, ok := graph[name]; ok {
			return true
		}
		for pkg := range graph {
			if strings.HasPrefix(pkg, name+"/") {
				return true
			}
		}
		return false
	}
	for _, rule := range forbidden {
		for _, name := range append([]string{rule.pkg}, rule.denied...) {
			if !live(name) {
				t.Errorf("forbidden rule for %s names missing package %s", rule.pkg, name)
			}
		}
	}
	source, err := os.ReadFile(filepath.Join(moduleRoot(t), "internal", "arch", "layers_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(source), "func architectureLayer(")
	if start < 0 {
		t.Fatal("layer table not found")
	}
	end := strings.Index(string(source)[start:], "\n}\n")
	table := string(source)[start : start+end]
	seen := []string{}
	for _, match := range regexp.MustCompile(`"([a-z][a-z0-9_/]*)"`).FindAllStringSubmatch(table, -1) {
		name := match[1]
		if slices.Contains([]string{"base", "infra", "snapshot", "dataset", "object", "discovery", "app"}, name) && !live(name) {
			continue
		}
		if !live(name) && !slices.Contains(seen, name) {
			seen = append(seen, name)
			t.Errorf("layer table names missing package %s", name)
		}
	}
}
