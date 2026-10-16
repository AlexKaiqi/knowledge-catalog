package arch_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// These are ownership requirements, separate from the preserved public
// publication/discovery journey. Missing modules are not business failures.
func TestObjectCapabilityModulesOwnTheirImplementations(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, module := range []string{"object", "liveaccess/handler", "liveaccess", "discovery", "systemrepo"} {
		files, err := filepath.Glob(filepath.Join(root, module, "*.go"))
		if err != nil || len(files) == 0 {
			t.Errorf("target owner %s has no implementation: %v", module, err)
		}
	}
	for _, obsolete := range []string{"retrieval", "index", "interfaces"} {
		if _, err := os.Stat(filepath.Join(root, obsolete)); !os.IsNotExist(err) {
			t.Errorf("obsolete owner %s still exists; a forwarding layer is not the target architecture", obsolete)
		}
	}
}

func TestValidationEntrypointsDoNotUseRetiredObjectOwners(t *testing.T) {
	root := moduleRoot(t)
	retired := regexp.MustCompile(`\./(?:index|retrieval|discovery/opensearch|discovery/internal/opensearchimpl)(?:/|[\s"']|$)`)
	check := func(path string) error {
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if retired.Match(body) {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("validation entrypoint %s still executes a retired object owner", rel)
		}
		return nil
	}
	if err := check(filepath.Join(root, "Makefile")); err != nil {
		t.Fatal(err)
	}
	if err := filepath.WalkDir(filepath.Join(root, "scripts"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || (filepath.Ext(path) != ".sh" && filepath.Ext(path) != ".py") {
			return err
		}
		return check(path)
	}); err != nil {
		t.Fatal(err)
	}
}

// Public depth is independent of the permission to depend on an owner. The
// layer matrix checks direction; this checks that consumers use actual public
// roots or explicitly selected first-level capabilities, including tests/tools.
func TestObjectCapabilityPublicDepth(t *testing.T) {
	root := moduleRoot(t)
	public := map[string]map[string]bool{
		"object":           {"reader": true, "writer": true, "service": true, "format": true, "semanticview": true, "cache": true, "maintenance": true, "client": true, "cli": true},
		"discovery":        {"adoption": true, "space": true, "service": true, "plugin": true, "projection": true, "datasets": true, "llmhttp": true, "client": true, "cli": true},
		"declarativeindex": {"client": true, "cli": true, "capability": true, "plugin": true, "projection": true, "control": true, "opensearch": true},
		"liveaccess":       {"contract": true, "service": true, "discovery": true, "cli": true, "client": true, "evidence": true, "plugin": true, "fixture": true, "handler": true, "dispatch": true},
		"systemrepo":       {},
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		from := strings.Split(filepath.ToSlash(rel), "/")[0]
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			local, ok := strings.CutPrefix(imported, "kc/")
			if !ok {
				continue
			}
			parts := strings.Split(local, "/")
			allowed, owned := public[parts[0]]
			if !owned || from == parts[0] {
				continue
			}
			if len(parts) > 2 || (len(parts) == 2 && !allowed[parts[1]]) {
				t.Errorf("%s imports non-public capability path %s", rel, imported)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestObjectRootsDoNotImportTheirDependentImplementations(t *testing.T) {
	graph := loadGraph(t)
	for _, root := range []string{"object", "liveaccess/handler", "discovery", "liveaccess", "systemrepo"} {
		for _, dependency := range graph[root] {
			if !strings.HasPrefix(dependency, root+"/") {
				continue
			}
			if path, ok := graph.reachable(dependency)[root]; ok {
				t.Errorf("root %s imports implementation %s that depends on it: %v", root, dependency, path)
			}
		}
	}
}

// A receipt is a base wire contract. Returning one must not pull Writer
// execution into independently adopted SDKs or their authenticated callers.
func TestObjectSDKDependencyGuardRejectsWriterImplementation(t *testing.T) {
	var denied []string
	for _, rule := range forbidden {
		if rule.pkg == "object/client" {
			denied = rule.denied
		}
	}
	if len(denied) == 0 {
		t.Fatal("Object SDK has no dependency boundary")
	}
	for _, tc := range []struct {
		target string
		want   bool
	}{{"object", false}, {"object/writer", true}, {"object/writer/internal/ledger", true}} {
		t.Run(tc.target, func(t *testing.T) {
			graph := depGraph{"object/client": {"sdk-bridge"}, "sdk-bridge": {tc.target}}
			violation := false
			for _, target := range denied {
				if _, found := reachDenied(graph.reachable("object/client"), target); found {
					violation = true
				}
			}
			if violation != tc.want {
				t.Fatalf("SDK dependency %s: violation=%v want=%v", tc.target, violation, tc.want)
			}
		})
	}
}
