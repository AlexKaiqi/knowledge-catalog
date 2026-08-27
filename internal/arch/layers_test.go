// Package arch holds the executable form of docs/LAYERS.md.
//
// The layering rules used to live only in prose, so a violation cost nothing
// until someone read the import list by hand. These tests fail the build
// instead. Adding a dependency listed here is a protocol decision, not a
// refactor: change the doc and the rule together, or find another seam.
package arch_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "kc"

// forbidden is "package X must not reach package Y, directly or transitively".
var forbidden = []struct {
	pkg    string
	denied []string
	why    string
}{
	{
		pkg:    "kernel",
		denied: []string{"snapshot", "knowledge", "repository", "knowledge/writer", "knowledge/reader", "catalog", "retrieval", "index", "controlplane"},
		why:    "kernel is identity and contracts only; it sits under every layer",
	},
	{
		pkg:    "snapshot",
		denied: []string{"knowledge", "repository", "knowledge/writer", "knowledge/reader", "catalog", "retrieval", "index", "controlplane", "connector", "hook", "gate", "snapshot/commandlog", "snapshot/treewriter", "snapshot/filegit", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli"},
		why:    "layer ⓪ knows only path/blob/tree/commit/ref/CAS; optional upper capabilities assert against it",
	},
	{
		pkg:    "knowledge",
		denied: []string{"repository", "knowledge/writer", "knowledge/reader", "catalog", "retrieval", "index", "controlplane", "connector", "hook", "gate", "snapshot/treewriter", "snapshot/filegit", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli"},
		why:    "layer ② contracts may depend on Snapshot coordinates but not their callers or adapters",
	},
	{
		pkg:    "catalog",
		denied: []string{"knowledge", "repository", "retrieval", "index", "knowledge/reader", "knowledge/writer", "connector", "hook", "snapshot/treewriter", "snapshot/filegit", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli"},
		why:    "layer ① composes repo refs and Workspace recipes; it must not know object_id, Aspect, IndexPlan, or any concrete store",
	},
	{
		pkg:    "knowledge/writer",
		denied: []string{"repository", "retrieval", "index", "catalog", "connector", "hook", "snapshot/treewriter", "snapshot/filegit", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli"},
		why:    "the write surface must not depend on retrieval derivations, composition, or a concrete store",
	},
	{
		pkg:    "knowledge/reader",
		denied: []string{"repository", "retrieval", "index", "catalog", "knowledge/writer", "connector", "hook", "snapshot/treewriter", "snapshot/filegit", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli"},
		why:    "layer ② assembly consumes coordinates it is handed; index/ is the ③ implementation above it",
	},
	{
		pkg:    "knowledge/serving",
		denied: []string{"repository", "retrieval", "index", "catalog", "knowledge/writer", "connector", "hook", "gate", "snapshot/treewriter", "snapshot/filegit", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli", "client"},
		why:    "consumer Knowledge Serving may compose Reader with an injected State port, but must not own providers, credentials, composition, writes, or retrieval",
	},
	{
		pkg:    "snapshot/treewriter",
		denied: []string{"knowledge", "knowledge/reader", "knowledge/writer", "catalog", "retrieval", "index", "controlplane", "connector", "snapshot/filegit", "snapshot/gitea", "snapshot/dolt", "cli"},
		why:    "literal tree mutation is layer ⓪ and must not acquire Knowledge or composition semantics",
	},
	{
		pkg:    "retrieval",
		denied: []string{"catalog", "knowledge/writer", "index", "controlplane", "connector", "snapshot/treewriter", "snapshot/filegit", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli"},
		why:    "layer ③ logical retrieval contracts may consume Knowledge declarations but not providers or application wiring",
	},
	{
		pkg:    "index",
		denied: []string{"repository", "catalog", "knowledge/writer", "snapshot/filegit", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli"},
		why:    "layer ③ subscribes through catalog.Hook; it must not import the Catalog or a concrete store",
	},
	{
		pkg:    "connector",
		denied: []string{"repository", "catalog", "knowledge/writer", "knowledge/reader", "index", "controlplane", "hook", "gate", "snapshot/filegit", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli"},
		why:    "the Collector reconciliation helper only produces ChangeSets; the wall-out caller drives source access and Writer",
	},
	{
		pkg:    "hook",
		denied: []string{"catalog", "knowledge/writer", "knowledge/reader", "index", "controlplane", "snapshot", "knowledge", "repository", "cli"},
		why:    "outbound hooks are a CLI concern; the protocol packages must not call user systems",
	},
	{
		pkg:    "gate",
		denied: []string{"catalog", "knowledge/writer", "knowledge/reader", "index", "controlplane", "hook", "cli"},
		why:    "a gate is a pure Check over a pinned Preview, not a hook and not a plane",
	},
	{
		pkg:    "snapshot/commandlog",
		denied: []string{"snapshot", "knowledge", "catalog", "retrieval", "index", "controlplane", "connector", "cli"},
		why:    "the shared command ledger owns replay mechanics, not any write surface's domain payload",
	},
	{
		pkg:    "internal/gitdir",
		denied: []string{"snapshot", "knowledge", "repository", "catalog", "knowledge/writer", "knowledge/reader", "index", "snapshot/filegit", "snapshot/gitea", "snapshot/dolt"},
		why:    "gitdir is git plumbing shared by layer ⓪ adapters and the layer ① registry; it must stay below both",
	},
	{
		pkg:    "internal/repofile",
		denied: []string{"repository", "catalog", "knowledge/writer", "knowledge/reader", "index", "snapshot/filegit", "snapshot/gitea", "snapshot/dolt"},
		why:    "repofile is the on-disk unit format, not a store",
	},
	{
		pkg:    "snapshot/filegit",
		denied: []string{"repository", "knowledge", "internal/repofile", "catalog", "knowledge/writer", "knowledge/reader", "index", "controlplane", "retrieval/opensearch", "cli"},
		why:    "the FileGit adapter exposes only Snapshot paths, commits, refs, history, and CAS",
	},
	{
		pkg:    "snapshot/gitea",
		denied: []string{"repository", "knowledge", "internal/repofile", "catalog", "knowledge/writer", "knowledge/reader", "index", "controlplane", "retrieval/opensearch", "cli"},
		why:    "the Gitea adapter exposes only Snapshot paths, commits, refs, history, and CAS",
	},
	{
		pkg:    "snapshot/dolt",
		denied: []string{"repository", "knowledge", "internal/repofile", "catalog", "knowledge/writer", "knowledge/reader", "index", "controlplane", "retrieval/opensearch", "cli"},
		why:    "the Dolt adapter exposes only Snapshot paths, commits, refs, history, and CAS",
	},
}

func TestForbiddenDependencies(t *testing.T) {
	graph := loadGraph(t)
	for _, rule := range forbidden {
		t.Run(rule.pkg, func(t *testing.T) {
			reach := graph.reachable(rule.pkg)
			for _, denied := range rule.denied {
				if path, ok := reach[denied]; ok {
					t.Errorf("%s must not depend on %s: %s\n  path: %s",
						rule.pkg, denied, rule.why, strings.Join(path, " -> "))
				}
			}
		})
	}
}

// Client is an application boundary above every protocol layer. Login state,
// credentials, HTTP transport, and token refresh must never leak downward into
// Snapshot, Catalog, Knowledge, retrieval contracts, or their adapters.
func TestProtocolLayersDoNotDependOnClient(t *testing.T) {
	graph := loadGraph(t)
	for _, pkg := range []string{
		"kernel", "snapshot", "knowledge", "catalog", "knowledge/writer", "knowledge/reader", "knowledge/serving",
		"retrieval", "index", "controlplane", "connector", "hook", "gate",
		"snapshot/treewriter", "snapshot/filegit", "snapshot/gitea", "snapshot/dolt",
		"retrieval/opensearch", "observability",
	} {
		if path, ok := graph.reachable(pkg)["client"]; ok {
			t.Errorf("%s must not depend on client login or transport state\n  path: %s", pkg, strings.Join(path, " -> "))
		}
	}
}

// TestCatalogStaysOffTheKnowledgeProtocol is the rule that regressed before:
// catalog/ reached reader/ and index/ through a concrete adapter, so
// linking layer ① dragged in layers ② and ③.
func TestCatalogStaysOffTheKnowledgeProtocol(t *testing.T) {
	graph := loadGraph(t)
	want := []string{"internal/gitdir", "internal/journal", "internal/jsonfile", "kernel", "snapshot"}
	got := keys(graph.reachable("catalog"))
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("catalog dependency set changed\n  got:  %v\n  want: %v\nIf this is intended, update docs/LAYERS.md in the same change.", got, want)
	}
}

func TestRemovedRepositoryPackageDoesNotReturn(t *testing.T) {
	root := moduleRoot(t)
	if _, err := os.Stat(filepath.Join(root, "repository")); err == nil {
		t.Fatal("repository/ mixes layer ⓪ and ②; keep contracts in snapshot/ and knowledge/")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	graph := loadGraph(t)
	for pkg, deps := range graph {
		if slices.Contains(deps, "repository") {
			t.Errorf("production package %s imports removed repository package; import snapshot or knowledge directly", pkg)
		}
	}
}

func keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

type depGraph map[string][]string

// reachable returns every kc package reachable from pkg, mapped to one witness
// import path so a failure says how the edge was formed.
func (g depGraph) reachable(pkg string) map[string][]string {
	out := map[string][]string{}
	var walk func(at string, trail []string)
	walk = func(at string, trail []string) {
		for _, next := range g[at] {
			if _, seen := out[next]; seen {
				continue
			}
			path := append(slices.Clone(trail), next)
			out[next] = path
			walk(next, path)
		}
	}
	walk(pkg, []string{pkg})
	return out
}

// loadGraph parses import statements of every non-test file under the module
// root. It reads source rather than shelling out to `go list` so the rule holds
// even for packages that do not compile yet.
func loadGraph(t *testing.T) depGraph {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	graph := depGraph{}
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		pkg := filepath.ToSlash(rel)
		for _, spec := range file.Imports {
			target, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil || !strings.HasPrefix(target, modulePath+"/") {
				continue
			}
			dep := strings.TrimPrefix(target, modulePath+"/")
			if !slices.Contains(graph[pkg], dep) {
				graph[pkg] = append(graph[pkg], dep)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(graph) == 0 {
		t.Fatalf("no kc packages found under %s", root)
	}
	return graph
}

func skipDir(name string) bool {
	switch name {
	case ".git", ".data", ".venv", ".kc", "node_modules", "docs", "scripts":
		return true
	}
	return false
}
