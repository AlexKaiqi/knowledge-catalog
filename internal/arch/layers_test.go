// Package arch holds the executable form of docs/LAYERS.md.
//
// The layering rules used to live only in prose, so a violation cost nothing
// until someone read the import list by hand. These tests fail the build
// instead. Adding a dependency listed here is a protocol decision, not a
// refactor: change the doc and the rule together, or find another seam.
package arch_test

import (
	"bytes"
	"go/ast"
	"go/format"
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
		denied: []string{"knowledge", "repository", "knowledge/writer", "knowledge/reader", "catalog", "retrieval", "index", "controlplane", "connector", "hook", "gate", "snapshot/commandlog", "snapshot/treewriter", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli"},
		why:    "layer ⓪ knows only path/blob/tree/commit/ref/CAS; optional upper capabilities assert against it",
	},
	{
		pkg:    "knowledge",
		denied: []string{"repository", "knowledge/writer", "knowledge/reader", "catalog", "retrieval", "index", "controlplane", "connector", "hook", "gate", "snapshot/treewriter", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli"},
		why:    "layer ② contracts may depend on Snapshot coordinates but not their callers or adapters",
	},
	{
		pkg:    "catalog",
		denied: []string{"knowledge", "repository", "retrieval", "index", "knowledge/reader", "knowledge/writer", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli"},
		why:    "layer ① composes repo refs and Workspace recipes; it must not know object_id, Aspect, IndexPlan, or any concrete store",
	},
	{
		pkg:    "knowledge/writer",
		denied: []string{"repository", "retrieval", "index", "catalog", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli"},
		why:    "the write surface must not depend on retrieval derivations, composition, or a concrete store",
	},
	{
		pkg:    "knowledge/reader",
		denied: []string{"repository", "knowledge/maintenance", "retrieval", "index", "catalog", "knowledge/writer", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli"},
		why:    "layer ② exact assembly consumes coordinates it is handed; maintenance scanning and index execution stay outside the consumer reader",
	},
	{
		pkg:    "knowledge/serving",
		denied: []string{"repository", "retrieval", "index", "catalog", "knowledge/writer", "connector", "hook", "gate", "snapshot/treewriter", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli", "client"},
		why:    "consumer Knowledge Serving may compose Reader with an injected State port, but must not own providers, credentials, composition, writes, or retrieval",
	},
	{
		pkg:    "snapshot/treewriter",
		denied: []string{"knowledge", "knowledge/reader", "knowledge/writer", "catalog", "retrieval", "index", "controlplane", "connector", "snapshot/gitea", "snapshot/dolt", "cli"},
		why:    "literal tree mutation is layer ⓪ and must not acquire Knowledge or composition semantics",
	},
	{
		pkg:    "retrieval",
		denied: []string{"catalog", "knowledge/writer", "index", "controlplane", "connector", "snapshot/treewriter", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli"},
		why:    "layer ③ logical retrieval contracts may consume Knowledge declarations but not providers or application wiring",
	},
	{
		pkg:    "index",
		denied: []string{"repository", "catalog", "knowledge/writer", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli"},
		why:    "layer ③ subscribes through catalog.Hook; it must not import the Catalog or a concrete store",
	},
	{
		pkg:    "connector",
		denied: []string{"repository", "catalog", "knowledge/writer", "knowledge/reader", "index", "controlplane", "hook", "gate", "snapshot/gitea", "snapshot/dolt", "retrieval/opensearch", "cli"},
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
		denied: []string{"snapshot", "knowledge", "repository", "catalog", "knowledge/writer", "knowledge/reader", "index", "snapshot/gitea", "snapshot/dolt"},
		why:    "gitdir is git plumbing shared by layer ⓪ adapters and the layer ① registry; it must stay below both",
	},
	{
		pkg:    "internal/repofile",
		denied: []string{"repository", "catalog", "knowledge/writer", "knowledge/reader", "index", "snapshot/gitea", "snapshot/dolt"},
		why:    "repofile is the on-disk unit format, not a store",
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
		"snapshot/treewriter", "snapshot/gitea", "snapshot/dolt",
		"retrieval/opensearch", "retrieval/llmhttp", "observability",
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

// TestEveryProductionPackageHasAnAllowedLayer makes package additions fail
// closed. The matrix checks direct edges; the focused transitive rules above
// retain the protocol's higher-value indirect dependency assertions.
func TestEveryProductionPackageHasAnAllowedLayer(t *testing.T) {
	graph := loadGraph(t)
	allowed := map[string]map[string]bool{
		"base":      {"base": true},
		"infra":     {"base": true, "infra": true},
		"snapshot":  {"base": true, "infra": true, "snapshot": true},
		"catalog":   {"base": true, "infra": true, "snapshot": true, "catalog": true},
		"knowledge": {"base": true, "infra": true, "snapshot": true, "knowledge": true},
		"retrieval": {"base": true, "infra": true, "snapshot": true, "knowledge": true, "retrieval": true},
		"app":       {"base": true, "infra": true, "snapshot": true, "catalog": true, "knowledge": true, "retrieval": true, "app": true},
	}
	for pkg, deps := range graph {
		from, ok := architectureLayer(pkg)
		if !ok {
			t.Errorf("production package %s is not assigned to an architecture layer", pkg)
			continue
		}
		for _, dep := range deps {
			to, classified := architectureLayer(dep)
			if !classified {
				t.Errorf("dependency %s imported by %s is not assigned to an architecture layer", dep, pkg)
				continue
			}
			if !allowed[from][to] {
				t.Errorf("layer edge %s(%s) -> %s(%s) is not allowed", pkg, from, dep, to)
			}
		}
	}
}

func architectureLayer(pkg string) (string, bool) {
	switch pkg {
	case "kernel":
		return "base", true
	case "internal/gitdir", "internal/journal", "internal/jsonfile", "internal/treepath":
		return "infra", true
	case "snapshot", "snapshot/commandlog", "snapshot/dolt", "snapshot/gitea", "snapshot/treewriter":
		return "snapshot", true
	case "catalog":
		return "catalog", true
	case "internal/repofile", "knowledge", "knowledge/dolt", "knowledge/maintenance", "knowledge/reader",
		"knowledge/serving", "knowledge/unitcodec", "knowledge/writer", "observability":
		return "knowledge", true
	case "retrieval", "retrieval/opensearch", "retrieval/llmhttp", "index":
		return "retrieval", true
	case "cli", "client", "cmd/kc", "cmd/kcfs", "connector", "controlplane", "gate", "hook",
		"internal/telemetry", "internal/testkit", "workspacefs":
		return "app", true
	default:
		return "", false
	}
}

// Concrete authority adapters are selectable in exactly one composition-root
// file. Provider implementation packages and their own conformance tests are
// the only other places allowed to mention those adapters.
func TestConcreteAuthorityImportsAreConfined(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
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
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		for _, spec := range file.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			if imported != modulePath+"/snapshot/dolt" && imported != modulePath+"/snapshot/gitea" && imported != modulePath+"/knowledge/dolt" {
				continue
			}
			allowed := rel == "cli/authority_drivers.go"
			switch imported {
			case modulePath + "/snapshot/dolt":
				allowed = allowed || strings.HasPrefix(rel, "snapshot/dolt/") || strings.HasPrefix(rel, "knowledge/dolt/")
			case modulePath + "/snapshot/gitea":
				allowed = allowed || strings.HasPrefix(rel, "snapshot/gitea/")
			case modulePath + "/knowledge/dolt":
				allowed = allowed || strings.HasPrefix(rel, "knowledge/dolt/")
			}
			if !allowed {
				t.Errorf("concrete authority import %s is forbidden in %s; use provider-neutral ports", imported, rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Consumer paths may inspect only already-published exact-basis projections.
// Projection maintenance and authority enumeration belong to explicit
// operations/controller code, never to READ/SEARCH/RELATIONS execution.
func TestConsumerPathsDoNotMaintainProjectionOrScanAuthority(t *testing.T) {
	root := moduleRoot(t)
	files := []string{
		"cli/verbs_read.go", "cli/workspace_search.go", "index/search.go", "index/relations.go",
		"knowledge/reader/reader.go", "knowledge/reader/repository_service.go", "knowledge/serving/serving.go",
	}
	forbiddenCalls := map[string]bool{
		"Ensure": true, "EnsureAt": true, "Rebuild": true, "Apply": true,
		"ScanSnapshotPage": true, "ChangedObjectIDs": true, "FastChangedObjectIDs": true, "ObjectIDsPage": true,
	}
	fset := token.NewFileSet()
	for _, rel := range files {
		file, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if ok && forbiddenCalls[selector.Sel.Name] {
				t.Errorf("consumer path %s calls forbidden maintenance method %s", rel, selector.Sel.Name)
			}
			return true
		})
	}
}

// Semantic object caches are an upper-product concern. Request-local maps are
// allowed, but Reader and Snapshot adapter structs/package variables must not
// retain ObjectID/KnowledgeRef -> KnowledgeValue/CanonicalRelation state.
func TestLowerLayersDoNotDeclareSemanticObjectCaches(t *testing.T) {
	root := moduleRoot(t)
	prefixes := []string{"knowledge/reader", "snapshot"}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		inScope := false
		for _, prefix := range prefixes {
			if rel == prefix+".go" || strings.HasPrefix(rel, prefix+"/") {
				inScope = true
				break
			}
		}
		if !inScope {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, declaration := range file.Decls {
			gen, ok := declaration.(*ast.GenDecl)
			if !ok || (gen.Tok != token.TYPE && gen.Tok != token.VAR) {
				continue
			}
			var rendered bytes.Buffer
			if err := format.Node(&rendered, fset, gen); err != nil {
				return err
			}
			text := rendered.String()
			identity := strings.Contains(text, "ObjectID") || strings.Contains(text, "KnowledgeRef")
			body := strings.Contains(text, "KnowledgeValue") || strings.Contains(text, "CanonicalRelation")
			if identity && body && strings.Contains(text, "map[") {
				t.Errorf("lower layer %s declares a semantic object cache: %s", rel, strings.TrimSpace(text))
			}
			if strings.Contains(text, "canonicalCache") || strings.Contains(text, "ObjectRetriever") {
				t.Errorf("lower layer %s reintroduces a retired semantic cache abstraction", rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
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
		if _, exists := graph[pkg]; !exists {
			// A package with only standard-library imports is still production
			// architecture. Register it before walking kc-local edges so a new
			// package cannot bypass explicit layer assignment by having no edges.
			graph[pkg] = nil
		}
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
