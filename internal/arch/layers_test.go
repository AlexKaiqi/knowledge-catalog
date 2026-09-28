// Package arch holds the executable form of docs/reviewed/core-architecture.md.
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
		denied: []string{"knowledge", "repository", "knowledge/writer", "knowledge/reader", "catalog", "retrieval", "index", "controlplane", "connector", "hook", "gate", "snapshot/commandlog", "snapshot/treewriter", "snapshot/gitea", "snapshot/lakefs", "retrieval/opensearch", "cli"},
		why:    "layer ⓪ knows only path/blob/tree/commit/ref/CAS; optional upper capabilities assert against it",
	},
	{
		pkg:    "knowledge",
		denied: []string{"repository", "knowledge/writer", "knowledge/reader", "catalog", "retrieval", "index", "controlplane", "connector", "hook", "gate", "snapshot/treewriter", "snapshot/gitea", "retrieval/opensearch", "cli"},
		why:    "layer ② contracts may depend on Snapshot coordinates but not their callers or adapters",
	},
	{
		pkg:    "catalog",
		denied: []string{"knowledge", "repository", "retrieval", "index", "knowledge/reader", "knowledge/writer", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "retrieval/opensearch", "cli"},
		why:    "layer ① composes repo refs and Workspace recipes; it must not know object_id, Aspect, IndexPlan, or any concrete store",
	},
	{
		pkg:    "catalog/worktree",
		denied: []string{"knowledge", "repository", "retrieval", "index", "knowledge/reader", "knowledge/writer", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "retrieval/opensearch", "cli"},
		why:    "host git checkout consumes Catalog recipes and pins; it is still layer ① and must not know object_id, Aspect, or any concrete store",
	},
	{
		pkg:    "knowledge/writer",
		denied: []string{"repository", "retrieval", "index", "catalog", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "retrieval/opensearch", "cli"},
		why:    "the write surface must not depend on retrieval derivations, composition, or a concrete store",
	},
	{
		pkg:    "knowledge/reader",
		denied: []string{"repository", "knowledge/maintenance", "retrieval", "index", "catalog", "knowledge/writer", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "retrieval/opensearch", "cli", "delivery"},
		why:    "layer ② exact assembly consumes coordinates it is handed; maintenance scanning, index execution, and caller-visible delivery stay outside the consumer reader",
	},
	{
		pkg:    "knowledge/serving",
		denied: []string{"repository", "retrieval", "index", "catalog", "knowledge/writer", "connector", "hook", "gate", "snapshot/treewriter", "snapshot/gitea", "retrieval/opensearch", "cli", "client", "delivery"},
		why:    "consumer Knowledge Serving may compose Reader with an injected State port, but must not own providers, credentials, composition, writes, retrieval, or caller-visible delivery",
	},
	{
		pkg:    "knowledgeapp",
		denied: []string{"home", "cli", "httpsurface", "client", "snapshot/gitea", "retrieval/opensearch"},
		why:    "the typed application core composes protocol ports; it must not parse transports, open deployments, or select concrete providers",
	},
	{
		pkg:    "snapshot/treewriter",
		denied: []string{"knowledge", "knowledge/reader", "knowledge/writer", "catalog", "retrieval", "index", "controlplane", "connector", "snapshot/gitea", "cli"},
		why:    "literal tree mutation is layer ⓪ and must not acquire Knowledge or composition semantics",
	},
	{
		pkg:    "retrieval",
		denied: []string{"catalog", "knowledge/writer", "index", "controlplane", "connector", "snapshot/treewriter", "snapshot/gitea", "retrieval/opensearch", "cli", "delivery"},
		why:    "layer ③ logical retrieval contracts may consume Knowledge declarations but not providers, application wiring, or caller-visible delivery",
	},
	{
		pkg:    "index",
		denied: []string{"repository", "catalog", "knowledge/writer", "snapshot/gitea", "retrieval/opensearch", "cli", "delivery"},
		why:    "layer ③ subscribes through catalog.Hook; it must not import the Catalog, a concrete store, or caller-visible delivery",
	},
	{
		pkg:    "integrationruntime",
		denied: []string{"home", "knowledge/writer", "snapshot/gitea", "snapshot/treewriter", "cli"},
		why:    "the wall-out integration runtime only uses reconciliation and typed client APIs, never Server state or authority implementations",
	},
	{
		pkg:    "connector",
		denied: []string{"repository", "catalog", "knowledge/writer", "knowledge/reader", "index", "controlplane", "hook", "gate", "snapshot/gitea", "retrieval/opensearch", "cli"},
		why:    "the Collector reconciliation helper only produces ChangeSets; the wall-out caller drives source access and Writer",
	},
	{
		pkg:    "hook",
		denied: []string{"catalog", "knowledge/writer", "knowledge/reader", "index", "controlplane", "snapshot", "knowledge", "repository", "cli"},
		why:    "outbound hooks are a CLI concern; the protocol packages must not call user systems",
	},
	{
		pkg:    "delivery",
		denied: []string{"cli", "catalog", "index", "retrieval", "retrieval/opensearch", "client", "hook", "gate", "knowledge/reader", "knowledge/writer", "knowledge/serving"},
		why:    "delivery rewrites a hydrated envelope for the caller; it must not locate, index, assemble Canonical, or read allow.json",
	},
	{
		pkg:    "snapshot/commandlog",
		denied: []string{"snapshot", "knowledge", "catalog", "retrieval", "index", "controlplane", "connector", "cli"},
		why:    "the shared command ledger owns replay mechanics, not any write surface's domain payload",
	},
	{
		pkg:    "internal/gitdir",
		denied: []string{"snapshot", "knowledge", "repository", "catalog", "knowledge/writer", "knowledge/reader", "index", "snapshot/gitea"},
		why:    "gitdir is git plumbing shared by layer ⓪ adapters and the layer ① registry; it must stay below both",
	},
	{
		pkg:    "internal/repofile",
		denied: []string{"repository", "catalog", "knowledge/writer", "knowledge/reader", "index", "snapshot/gitea"},
		why:    "repofile is the on-disk unit format, not a store",
	},
	{
		pkg:    "snapshot/gitea",
		denied: []string{"repository", "knowledge", "internal/repofile", "catalog", "knowledge/writer", "knowledge/reader", "index", "controlplane", "retrieval/opensearch", "cli"},
		why:    "the Gitea adapter exposes only Snapshot paths, commits, refs, history, and CAS",
	},
	{
		pkg:    "snapshot/lakefs",
		denied: []string{"repository", "knowledge", "internal/repofile", "catalog", "knowledge/writer", "knowledge/reader", "index", "controlplane", "retrieval/opensearch", "cli"},
		why:    "the LakeFS adapter exposes only Snapshot paths, commits, refs, history, and CAS",
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

func TestArchitectureGuardIncludesProductionScripts(t *testing.T) {
	graph := loadGraph(t)
	if _, ok := graph["scripts/fixture-deployment"]; !ok {
		t.Fatal("production scripts are outside the architecture import graph")
	}
}

func TestIntegrationRuntimeDoesNotOpenCatalogOrServer(t *testing.T) {
	// The typed client shares Catalog DTOs, but the runtime must not import
	// Catalog operations or Server application state directly.
	for _, dependency := range loadGraph(t)["integrationruntime"] {
		if dependency == "catalog" || dependency == "home" || dependency == "cli" || dependency == "knowledge/reader" {
			t.Errorf("integrationruntime directly imports Server boundary %s", dependency)
		}
	}
}

// Client is an application boundary above every protocol layer. Login state,
// credentials, HTTP transport, and token refresh must never leak downward into
// Snapshot, Catalog, Knowledge, retrieval contracts, or their adapters.
func TestProtocolLayersDoNotDependOnClient(t *testing.T) {
	graph := loadGraph(t)
	for _, pkg := range []string{
		"kernel", "snapshot", "knowledge", "catalog", "catalog/worktree", "knowledge/writer", "knowledge/reader", "knowledge/serving",
		"retrieval", "index", "controlplane", "connector", "hook", "gate",
		"snapshot/treewriter", "snapshot/gitea", "snapshot/lakefs",
		"retrieval/opensearch", "retrieval/llmhttp", "observability",
		"httpsurface", "home",
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
		t.Errorf("catalog dependency set changed\n  got:  %v\n  want: %v\nIf this is intended, update docs/reviewed/core-architecture.md in the same change.", got, want)
	}
	worktreeWant := []string{"catalog", "internal/gitdir", "internal/journal", "internal/jsonfile", "kernel", "snapshot"}
	gotWorktree := keys(graph.reachable("catalog/worktree"))
	slices.Sort(gotWorktree)
	if !slices.Equal(gotWorktree, worktreeWant) {
		t.Errorf("catalog/worktree dependency set changed\n  got:  %v\n  want: %v\nIf this is intended, update docs/reviewed/core-architecture.md in the same change.", gotWorktree, worktreeWant)
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
	case "snapshot", "snapshot/commandlog", "snapshot/gitea", "snapshot/lakefs", "snapshot/stamp", "snapshot/treewriter":
		return "snapshot", true
	case "catalog", "catalog/worktree":
		return "catalog", true
	case "internal/repofile", "knowledge", "knowledge/maintenance", "knowledge/reader",
		"knowledge/semanticview", "knowledge/serving", "knowledge/unitcodec", "knowledge/writer", "observability":
		return "knowledge", true
	case "retrieval", "retrieval/opensearch", "retrieval/llmhttp", "retrieval/cache", "index":
		return "retrieval", true
	case "cli", "client", "cmd/kc", "cmd/kcfs", "cmd/kc-integration", "integrationruntime", "identity", "connector", "controlplane", "gate", "hook",
		"home", "httpsurface", "internal/telemetry", "internal/testkit", "datasetfs", "delivery", "knowledgeapp",
		"scripts/check-docs", "scripts/docs-serve", "scripts/fixture-deployment", "scripts/validation-inventory", "scripts/coupling":
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
			if imported != modulePath+"/snapshot/gitea" && imported != modulePath+"/snapshot/lakefs" {
				continue
			}
			allowed := rel == "home/authority_drivers.go"
			switch imported {
			case modulePath + "/snapshot/gitea":
				allowed = allowed || strings.HasPrefix(rel, "snapshot/gitea/")
			case modulePath + "/snapshot/lakefs":
				allowed = allowed || strings.HasPrefix(rel, "snapshot/lakefs/")
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
		"cli/verbs_read.go", "cli/dataset_search.go", "index/search.go", "index/relations.go",
		"knowledgeapp/dataset_read.go", "knowledgeapp/dataset_search.go", "knowledgeapp/dataset_relations.go",
		"knowledge/reader/reader.go", "knowledge/reader/repository_service.go", "knowledge/serving/serving.go",
	}
	forbiddenCalls := map[string]bool{
		"Ensure": true, "EnsureAt": true, "Rebuild": true, "Apply": true,
		"ScanSnapshotPage": true, "ChangedObjectIDs": true, "FastChangedObjectIDs": true, "ObjectIDsPage": true,
		"CatchUp": true, "Reconcile": true, "Desire": true,
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

func TestProjectionWorkerStartsOnlyFromServeFacade(t *testing.T) {
	root := moduleRoot(t)
	homeGo, err := os.ReadFile(filepath.Join(root, "home", "home.go"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(homeGo, []byte("Projection.Start")) {
		t.Fatal("Open() must not start the projection worker; one-shot search would CatchUp")
	}
	serveGo, err := os.ReadFile(filepath.Join(root, "cli", "serve_facade.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(serveGo, []byte("Projection.Start")) {
		t.Fatal("kc serve must start the projection worker on the long-lived Home")
	}
}

func TestApplicationPackageBoundaries(t *testing.T) {
	graph := loadGraph(t)
	protocol := []string{
		"kernel", "snapshot", "knowledge", "catalog", "catalog/worktree", "knowledge/writer", "knowledge/reader", "knowledge/serving",
		"retrieval", "index", "controlplane", "connector", "hook", "gate",
		"snapshot/treewriter", "snapshot/gitea", "snapshot/lakefs",
		"retrieval/opensearch", "retrieval/llmhttp", "observability",
	}
	for _, pkg := range protocol {
		for _, denied := range []string{"home", "httpsurface", "cli"} {
			if path, ok := graph.reachable(pkg)[denied]; ok {
				t.Errorf("%s must not depend on application package %s\n  path: %s", pkg, denied, strings.Join(path, " -> "))
			}
		}
	}
	for _, denied := range []string{"cli", "client", "httpsurface"} {
		if path, ok := graph.reachable("home")[denied]; ok {
			t.Errorf("home must not depend on %s\n  path: %s", denied, strings.Join(path, " -> "))
		}
	}
	if deps := graph["httpsurface"]; len(deps) != 0 {
		t.Errorf("httpsurface must not import other kc packages, got %v", deps)
	}
	for _, denied := range []string{"cli", "home", "httpsurface"} {
		if path, ok := graph.reachable("client")[denied]; ok {
			t.Errorf("client must not depend on %s\n  path: %s", denied, strings.Join(path, " -> "))
		}
	}
	for pkg, deps := range graph {
		if pkg == "cli" {
			if slices.Contains(deps, "httpsurface") {
				t.Errorf("production cli must not import httpsurface; argv tables stay independent of the HTTP registry")
			}
			continue
		}
		if pkg == "scripts/fixture-deployment" {
			// This acceptance binary constructs deployment configuration and
			// invokes the one explicit fixture-provisioning seam. The focused
			// AST guard below freezes the exact home symbols it may use.
			continue
		}
		if slices.Contains(deps, "home") {
			t.Errorf("%s must not import home; only cli transport opens Home", pkg)
		}
	}
}

func TestFixtureDeploymentUsesOnlyDeclaredHomeSeams(t *testing.T) {
	root := moduleRoot(t)
	file, err := parser.ParseFile(token.NewFileSet(),
		filepath.Join(root, "scripts", "fixture-deployment", "main.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"DeploymentConfig": true, "CatalogBinding": true, "DefaultStores": true,
		"StoresFile": true, "RepositoryBinding": true, "RepositoryAccess": true,
		"SystemRepositoryAccess": true, "PrepareFixtureAuthority": true,
		"PrepareCatalogAuthority": true,
	}
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, idOK := selector.X.(*ast.Ident)
		if idOK && id.Name == "kchome" && !allowed[selector.Sel.Name] {
			t.Errorf("fixture deployment uses undeclared home seam %s", selector.Sel.Name)
		}
		return true
	})
}

func TestApplicationCoreHasNoTransportOrProviderImports(t *testing.T) {
	graph := loadGraph(t)
	reachable := graph.reachable("knowledgeapp")
	for _, denied := range []string{
		"home", "cli", "httpsurface", "client",
		"snapshot/gitea", "snapshot/lakefs", "retrieval/opensearch",
	} {
		if path, ok := reachable[denied]; ok {
			t.Errorf("knowledgeapp reaches %s through %s", denied, strings.Join(path, " -> "))
		}
	}
	root := moduleRoot(t)
	for _, rel := range []string{"knowledgeapp/read.go", "knowledgeapp/search.go", "knowledgeapp/write.go"} {
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte("FlagValue")) || bytes.Contains(raw, []byte("*http.Request")) ||
			bytes.Contains(raw, []byte(`Verb string`)) {
			t.Errorf("%s contains transport-shaped application input", rel)
		}
	}
}

func TestCLIAndHTTPUseSameTypedApplicationExecutor(t *testing.T) {
	root := moduleRoot(t)
	readVerb, err := os.ReadFile(filepath.Join(root, "cli", "verbs_read.go"))
	if err != nil {
		t.Fatal(err)
	}
	searchVerb, err := os.ReadFile(filepath.Join(root, "cli", "verbs_index.go"))
	if err != nil {
		t.Fatal(err)
	}
	routes, err := os.ReadFile(filepath.Join(root, "cli", "service_routes.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		verb    []byte
		core    []byte
		command []byte
	}{
		{readVerb, []byte("knowledgeapp.ReadExecutor"), []byte(`run: verbRead`)},
		{searchVerb, []byte("knowledgeapp.SearchExecutor"), []byte(`run: verbSearch`)},
	} {
		if !bytes.Contains(check.verb, check.core) || !bytes.Contains(routes, check.command) {
			t.Errorf("CLI/HTTP route is not wired through %s", check.core)
		}
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
	case ".git", ".data", ".venv", ".kc", "node_modules", "docs":
		return true
	}
	// webui is the vendored lakeFS frontend (its own Go module and npm build;
	// see webui/KC-VENDOR.md). Its Go file is not a production package of this
	// module and its JavaScript has no architecture layer.
	return name == "webui"
}
