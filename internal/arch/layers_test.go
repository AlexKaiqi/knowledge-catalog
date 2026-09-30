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
		denied: []string{"knowledge", "repository", "knowledge/writer", "knowledge/reader", "catalog", "retrieval", "index", "controlplane", "connector", "hook", "gate", "snapshot/commandlog", "snapshot/treewriter", "snapshot/gitea", "snapshot/lakefs", "index/opensearch", "cli"},
		why:    "layer ⓪ knows only path/blob/tree/commit/ref/CAS; optional upper capabilities assert against it",
	},
	{
		pkg:    "knowledge",
		denied: []string{"repository", "knowledge/writer", "knowledge/reader", "catalog", "retrieval", "index", "controlplane", "connector", "hook", "gate", "snapshot/treewriter", "snapshot/gitea", "index/opensearch", "cli"},
		why:    "layer ② contracts may depend on Snapshot coordinates but not their callers or adapters",
	},
	{
		pkg:    "catalog",
		denied: []string{"knowledge", "repository", "retrieval", "index", "knowledge/reader", "knowledge/writer", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "index/opensearch", "cli"},
		why:    "layer ① composes repo refs and Workspace recipes; it must not know object_id, Aspect, IndexPlan, or any concrete store",
	},
	{
		pkg:    "catalog/worktree",
		denied: []string{"knowledge", "repository", "retrieval", "index", "knowledge/reader", "knowledge/writer", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "index/opensearch", "cli"},
		why:    "host git checkout consumes Catalog recipes and pins; it is still layer ① and must not know object_id, Aspect, or any concrete store",
	},
	{
		pkg:    "catalog/internal/worktreeimpl",
		denied: []string{"knowledge", "repository", "retrieval", "index", "knowledge/reader", "knowledge/writer", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "index/opensearch", "cli"},
		why:    "worktree implementation (B11 internalization) obeys the same layer-① rule as its shim; no object_id, Aspect, or concrete store",
	},
	{
		pkg:    "knowledge/writer",
		denied: []string{"repository", "retrieval", "index", "catalog", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "index/opensearch", "cli"},
		why:    "the write surface must not depend on retrieval derivations, composition, or a concrete store",
	},
	{
		pkg:    "knowledge/reader",
		denied: []string{"repository", "knowledge/maintenance", "retrieval", "index", "catalog", "knowledge/writer", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "index/opensearch", "cli", "delivery"},
		why:    "layer ② exact assembly consumes coordinates it is handed; maintenance scanning, index execution, and caller-visible delivery stay outside the consumer reader",
	},
	{
		pkg:    "knowledge/serving",
		denied: []string{"repository", "retrieval", "index", "catalog", "knowledge/writer", "connector", "hook", "gate", "snapshot/treewriter", "snapshot/gitea", "index/opensearch", "cli", "client", "delivery"},
		why:    "consumer Knowledge Serving may compose Reader with an injected State port, but must not own providers, credentials, composition, writes, retrieval, or caller-visible delivery",
	},
	{
		pkg:    "knowledgeapp",
		denied: []string{"home", "cli", "httpsurface", "client", "snapshot/gitea", "index/opensearch"},
		why:    "the typed application core composes protocol ports; it must not parse transports, open deployments, or select concrete providers",
	},
	{
		pkg:    "server",
		denied: []string{"cli"},
		why:    "the service surface serves typed HTTP and must not import an argv front end; front ends compose it and callers use the typed client. The client dependency is the decided wire-shape home for the management surface (B7 ruling): payload types and typed methods live in client, typed service routes own their DTOs in server, and neither side may migrate shapes without re-opening that ruling (CAP-ORG-01 B6/B7)",
	},
	{
		pkg:    "invocation",
		denied: []string{"cli", "server", "httpsurface", "client"},
		why:    "the operation layer is import-neutral between the transports: verbs consume the use-case core (knowledgeapp) and the carrier holds the transport-opened *home.Home, but the seam never knows an argv front end or a service surface (CAP-ORG-01 B5a carrier, B6 operation layer)",
	},
	{
		pkg:    "snapshot/treewriter",
		denied: []string{"knowledge", "knowledge/reader", "knowledge/writer", "catalog", "retrieval", "index", "controlplane", "connector", "snapshot/gitea", "cli"},
		why:    "literal tree mutation is layer ⓪ and must not acquire Knowledge or composition semantics",
	},
	{
		pkg:    "retrieval",
		denied: []string{"catalog", "knowledge/writer", "index", "controlplane", "connector", "snapshot/treewriter", "snapshot/gitea", "index/opensearch", "cli", "delivery"},
		why:    "layer ③ logical retrieval contracts may consume Knowledge declarations but not providers, application wiring, or caller-visible delivery",
	},
	{
		pkg:    "index",
		denied: []string{"repository", "catalog", "knowledge/writer", "snapshot/gitea", "index/opensearch", "cli", "delivery"},
		why:    "layer ③ subscribes through catalog.Hook; it must not import the Catalog, a concrete store, or caller-visible delivery",
	},
	{
		pkg:    "integrationruntime",
		denied: []string{"home", "knowledge/writer", "snapshot/gitea", "snapshot/treewriter", "cli"},
		why:    "the wall-out integration runtime only uses reconciliation and typed client APIs, never Server state or authority implementations",
	},
	{
		pkg:    "connector",
		denied: []string{"repository", "catalog", "knowledge/writer", "knowledge/reader", "index", "controlplane", "hook", "gate", "snapshot/gitea", "index/opensearch", "cli"},
		why:    "the Collector reconciliation helper only produces ChangeSets; the wall-out caller drives source access and Writer",
	},
	{
		pkg:    "hook",
		denied: []string{"catalog", "knowledge/writer", "knowledge/reader", "index", "controlplane", "snapshot", "knowledge", "repository", "cli"},
		why:    "outbound hooks are a CLI concern; the protocol packages must not call user systems",
	},
	{
		pkg:    "delivery",
		denied: []string{"cli", "catalog", "index", "retrieval", "index/opensearch", "client", "hook", "gate", "knowledge/reader", "knowledge/writer", "knowledge/serving"},
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
		denied: []string{"repository", "knowledge", "internal/repofile", "catalog", "knowledge/writer", "knowledge/reader", "index", "controlplane", "index/opensearch", "cli"},
		why:    "the Gitea adapter exposes only Snapshot paths, commits, refs, history, and CAS",
	},
	{
		pkg:    "snapshot/lakefs",
		denied: []string{"repository", "knowledge", "internal/repofile", "catalog", "knowledge/writer", "knowledge/reader", "index", "controlplane", "index/opensearch", "cli"},
		why:    "the LakeFS adapter exposes only Snapshot paths, commits, refs, history, and CAS",
	},

	// App-layer capability denylists (CAP-ORG-01 B2; the §3.4 four-piece audit's
	// seven missing entries). Each set mirrors the capability README's
	// dependency-direction section; denied entries cover subpackages.
	{
		pkg:    "invoke",
		denied: []string{"cli", "httpsurface", "client", "home"},
		why:    "transport neutrality is invoke's reason to exist; transports call the vocabulary, never the reverse (R1 removed the last home import)",
	},
	{
		pkg:    "client",
		denied: []string{"cli", "httpsurface", "home"},
		why:    "callers borrow identity/session from the client; the client never knows a transport, a route table, or a deployment",
	},
	{
		pkg:    "controlplane",
		denied: []string{"cli", "httpsurface", "client", "home"},
		why:    "maintenance-loop operations are transport-agnostic; transports and clients call it, not the other way",
	},
	{
		pkg:    "gate",
		denied: []string{"snapshot", "catalog", "knowledge", "retrieval", "index", "hook", "controlplane", "cli", "httpsurface", "client", "home"},
		why:    "the merge evidence gate reads only its rules and evidence bindings, fail-closed; it must not reach into protocol layers, hooks, or transports",
	},
	{
		pkg:    "identity",
		denied: []string{"snapshot", "catalog", "knowledge", "retrieval", "index", "hook", "controlplane", "cli", "httpsurface", "client", "home"},
		why:    "identity binding reads its own durable store and IdP results only; isolation matches gate's strict set",
	},
	{
		pkg:    "datasetfs",
		denied: []string{"kernel", "snapshot", "catalog", "knowledge", "retrieval", "index", "controlplane", "connector", "integrationruntime", "hook", "gate", "delivery", "identity", "observability", "invoke", "client", "knowledgeapp", "cli", "httpsurface", "home"},
		why:    "the local read-only consumption view mounts what a plan hands it; it never calls back into the protocol ring or any transport",
	},
	{
		pkg:    "observability",
		denied: []string{"retrieval", "index", "controlplane", "hook", "gate", "cli", "httpsurface", "client", "home"},
		why:    "evidence and telemetry are consumed by knowledge/serving and the transports; they must not reach retrieval, index, hooks, gates, or transports (knowledge stays allowed: knowledge/serving imports it)",
	},
}

func TestForbiddenDependencies(t *testing.T) {
	graph := loadGraph(t)
	for _, rule := range forbidden {
		t.Run(rule.pkg, func(t *testing.T) {
			reach := graph.reachable(rule.pkg)
			for _, denied := range rule.denied {
				if path, ok := reachDenied(reach, denied); ok {
					t.Errorf("%s must not depend on %s (or its subpackages): %s\n  path: %s",
						rule.pkg, denied, rule.why, strings.Join(path, " -> "))
				}
			}
		})
	}
}

// reachDenied returns the shortest violation path for a denied entry. The
// entry denies its exact package and every package that package owns, so a
// new subpackage cannot quietly escape a layer rule by adding a path segment.
func reachDenied(reach map[string][]string, denied string) ([]string, bool) {
	var best []string
	for pkg, path := range reach {
		if deniedMatch(denied, pkg) && (best == nil || len(path) < len(best)) {
			best = path
		}
	}
	return best, best != nil
}

func deniedMatch(denied, pkg string) bool {
	return pkg == denied || strings.HasPrefix(pkg, denied+"/")
}

func TestDeniedMatchCoversSubpackages(t *testing.T) {
	cases := []struct {
		denied, pkg string
		want        bool
	}{
		{"cli", "cli", true},
		{"cli", "cli/serve", true},
		{"cli", "client", false},
		{"cli", "clip", false},
		{"knowledge/writer", "knowledge/writer", true},
		{"knowledge/writer", "knowledge/writer/extra", true},
		{"knowledge/writer", "knowledge/reader", false},
		{"index/opensearch", "index/opensearch", true},
	}
	for _, c := range cases {
		if got := deniedMatch(c.denied, c.pkg); got != c.want {
			t.Errorf("deniedMatch(%q, %q) = %v, want %v", c.denied, c.pkg, got, c.want)
		}
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
		"catalog/internal/worktreeimpl", "knowledge/internal/unitcodecimpl", "knowledge/internal/semanticviewimpl", "knowledge/internal/maintenanceimpl",
		"retrieval", "index", "controlplane", "connector", "hook", "gate",
		"snapshot/treewriter", "snapshot/gitea", "snapshot/lakefs",
		"index/opensearch", "index/internal/opensearchimpl", "retrieval/llmhttp", "observability",
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
	worktreeWant := []string{"catalog", "catalog/internal/worktreeimpl", "internal/gitdir", "internal/journal", "internal/jsonfile", "kernel", "snapshot"}
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
	case "invocation":
		return "app", true
	case "snapshot", "snapshot/commandlog", "snapshot/gitea", "snapshot/lakefs", "snapshot/stamp", "snapshot/treewriter",
		"snapshot/internal/giteaimpl", "snapshot/internal/lakefsimpl":
		return "snapshot", true
	case "catalog", "catalog/worktree", "catalog/internal/worktreeimpl":
		return "catalog", true
	case "internal/repofile", "knowledge", "knowledge/maintenance", "knowledge/reader",
		"knowledge/semanticview", "knowledge/serving", "knowledge/unitcodec", "knowledge/writer",
		"knowledge/internal/unitcodecimpl", "knowledge/internal/semanticviewimpl", "knowledge/internal/maintenanceimpl":
		return "knowledge", true
	case "retrieval", "retrieval/llmhttp", "retrieval/cache", "index", "index/opensearch",
		"index/internal/opensearchimpl":
		return "retrieval", true
	case "cli", "cli/serve", "client", "cmd/kc", "cmd/kcfs", "cmd/kc-integration", "integrationruntime", "invoke", "identity", "connector", "controlplane", "gate", "hook",
		"home", "httpsurface", "server", "server/internal/readiness", "internal/telemetry", "internal/testkit", "datasetfs", "delivery", "knowledgeapp", "observability",
		"scripts/check-docs", "scripts/docs-serve", "scripts/fixture-deployment", "scripts/validation-inventory", "scripts/coupling", "scripts/export-audit":
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
	serveGo, err := os.ReadFile(filepath.Join(root, "server", "serve_facade.go"))
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
		"index/opensearch", "retrieval/llmhttp", "observability",
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
		if pkg == "invocation" {
			// The shared execution carrier holds the transport-opened
			// *home.Home that verb bodies consume (CAP-ORG-01 B5a); it never
			// opens one itself, so it may reference home the way the
			// transports do while staying import-neutral between them.
			continue
		}
		if slices.Contains(deps, "home") && pkg != "server" && !strings.HasPrefix(pkg, "server/") {
			t.Errorf("%s must not import home; only the transports open Home (cli and, from CAP-ORG-01 B6, the server facade)", pkg)
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

// TestProviderAdaptersImportedOnlyAtAssembly extends A-01 (CAP-ORG-01 B7): the
// concrete provider adapters may be imported only by their own capability
// tree, by the assembly root (home), or by a registered, why-documented
// exception. Everything else consumes providers through ports.
// TestCapabilityConsumerSets closes the Phase 1 consumer story (CAP-ORG-01
// B8): after the transports converged on the invocation seam, the shared
// application-infrastructure capabilities have a fixed consumer set. A new
// consumer must extend the allowlist here deliberately, not drift in.
func TestCapabilityConsumerSets(t *testing.T) {
	graph := loadGraph(t)
	allowed := map[string][]string{
		"delivery":      {"invocation"},
		"gate":          {"controlplane", "home", "invocation"},
		"hook":          {"home", "invocation", "invoke"},
		"identity":      {"cli", "home", "invocation", "server"},
		"observability": {"cli", "client", "home", "invocation", "server"},
	}
	for capability, consumers := range allowed {
		for pkg, deps := range graph {
			if !slices.Contains(deps, capability) {
				continue
			}
			if slices.Contains(consumers, pkg) {
				continue
			}
			t.Errorf("%s is consumed by %s; the Phase 1 consumer set is %v - extend the allowlist deliberately (CAP-ORG-01 B8)", capability, pkg, consumers)
		}
	}
}

func TestProviderAdaptersImportedOnlyAtAssembly(t *testing.T) {
	graph := loadGraph(t)
	// No exceptions remain: the rerank/embedder provider assembly moved to the
	// assembly root (home/llm_ports.go, B7 port pattern; 2026-09-29 review).
	exceptions := map[string]map[string]string{}
	// Adapter trees may consume peer adapters when their own engine needs them
	// (B12: the OpenSearch engine assembles the llmhttp embedder; that
	// composition is adapter-internal, not assembly).
	peerAdapterImports := map[string]map[string]string{
		"index/internal/opensearchimpl": {
			"retrieval/llmhttp": "the OpenSearch engine assembles the llmhttp embedder (B12 adapter-internal composition)",
		},
	}
	// Ruling C (CAP-ORG-01 B10): lakefs/gitea adapters are internal impls
	// reachable only through their registration shims (snapshot/lakefs,
	// snapshot/gitea); the shim itself is in the snapshot tree. The remaining
	// public adapters keep the assembly-root rule.
	adapters := []string{
		"snapshot/internal/lakefsimpl", "snapshot/internal/giteaimpl",
		"index/internal/opensearchimpl", "retrieval/llmhttp", "retrieval/cache",
	}
	for pkg, deps := range graph {
		for _, adapter := range adapters {
			// Only exact adapter imports are provider consumption; importing
			// the capability root (kc/retrieval) is normal interface use.
			if !slices.Contains(deps, adapter) {
				continue
			}
			root := strings.SplitN(adapter, "/", 2)[0]
			if pkg == "home" || pkg == root || strings.HasPrefix(pkg, root+"/") {
				continue
			}
			if shim, ok := map[string]string{
				"snapshot/internal/lakefsimpl":  "snapshot/lakefs",
				"snapshot/internal/giteaimpl":   "snapshot/gitea",
				"index/internal/opensearchimpl": "index/opensearch",
			}[adapter]; ok && pkg == shim {
				continue
			}
			// Adapter trees may consume peer adapters when their own engine
			// needs them (B12: the OpenSearch engine assembles the llmhttp
			// embedder; that composition is adapter-internal, not assembly).
			if peers, ok := peerAdapterImports[pkg]; ok && peers[adapter] != "" {
				continue
			}
			if allowed, ok := exceptions[pkg]; ok && allowed[adapter] != "" {
				continue
			}
			t.Errorf("%s imports provider adapter %s; only the assembly root (home), the adapter tree, and registered exceptions may (CAP-ORG-01 B7)", pkg, adapter)
		}
	}
}

func TestApplicationCoreHasNoTransportOrProviderImports(t *testing.T) {
	graph := loadGraph(t)
	reachable := graph.reachable("knowledgeapp")
	for _, denied := range []string{
		"home", "cli", "httpsurface", "client",
		"snapshot/gitea", "snapshot/lakefs", "index/opensearch",
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
	readVerb, err := os.ReadFile(filepath.Join(root, "invocation", "verbs_read.go"))
	if err != nil {
		t.Fatal(err)
	}
	searchVerb, err := os.ReadFile(filepath.Join(root, "invocation", "verbs_index.go"))
	if err != nil {
		t.Fatal(err)
	}
	routes, err := os.ReadFile(filepath.Join(root, "server", "service_routes.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		verb    []byte
		core    []byte
		command []byte
	}{
		{readVerb, []byte("knowledgeapp.ReadExecutor"), []byte(`Run: inv.VerbRead`)},
		{searchVerb, []byte("knowledgeapp.SearchExecutor"), []byte(`Run: inv.VerbSearch`)},
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

// TestEveryTopLevelCapabilityHasAReadme freezes the four-piece identity rule
// (§2.2 ①): every top-level directory that carries Go packages of its own must
// have a README.md. A capability without its identity card is the §1.2 signal
// that its boundary was never finished. internal/ is the infrastructure tree
// (guarded by layer rules), not a capability.
func TestEveryTopLevelCapabilityHasAReadme(t *testing.T) {
	root := moduleRoot(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || skipDir(name) || name == "internal" {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		hasGo := false
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".go") {
				hasGo = true
				break
			}
		}
		if !hasGo {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, name, "README.md")); err != nil {
			t.Errorf("%s/ has Go packages but no README.md: a capability without its identity card (§1.3)", name)
		}
	}
}
