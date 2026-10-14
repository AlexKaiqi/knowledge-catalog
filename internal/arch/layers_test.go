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

	{pkg: "object", denied: []string{"interfaces", "liveaccess", "systemrepo"}, why: "Object owns minimal pure contracts, not ability implementations or complete publication"},
	{pkg: "object/reader", denied: []string{"interfaces", "liveaccess", "systemrepo"}, why: "fixed reads do not install or invoke abilities"},
	{pkg: "object/writer", denied: []string{"interfaces", "liveaccess", "systemrepo"}, why: "basic publication does not require ability execution or complete platform publication"},

	{pkg: "object/service", denied: []string{"interfaces", "liveaccess", "discovery", "declarativeindex", "systemrepo", "home", "cli", "server", "client"}, why: "basic object tasks use fixed read/write and injected delivery ports"},
	{pkg: "interfaces", denied: []string{"liveaccess", "discovery", "declarativeindex", "systemrepo", "home", "cli", "server", "client", "object/writer"}, why: "ordinary Interface contracts and dispatch are independent of concrete abilities"},
	{pkg: "discovery/projection", denied: []string{"liveaccess", "systemrepo"}, why: "projection consumes verified observations; the source bridge owns I/O"},
	{
		pkg:    "kernel",
		denied: []string{"snapshot", "object", "repository", "object/writer", "object/reader", "catalog", "discovery", "declarativeindex", "discovery/projection", "controlplane"},
		why:    "kernel is identity and contracts only; it sits under every layer",
	},
	{
		pkg:    "snapshot",
		denied: []string{"object", "repository", "object/writer", "object/reader", "catalog", "discovery", "declarativeindex", "discovery/projection", "controlplane", "connector", "hook", "gate", "snapshot/commandlog", "snapshot/treewriter", "snapshot/gitea", "snapshot/lakefs", "discovery/opensearch", "cli"},
		why:    "layer ⓪ knows only path/blob/tree/commit/ref/CAS; optional upper capabilities assert against it",
	},
	{
		pkg:    "object",
		denied: []string{"repository", "object/writer", "object/reader", "catalog", "discovery", "declarativeindex", "discovery/projection", "controlplane", "connector", "hook", "gate", "snapshot/treewriter", "snapshot/gitea", "discovery/opensearch", "cli"},
		why:    "layer ② contracts may depend on Snapshot coordinates but not their callers or adapters",
	},
	{
		pkg:    "snapshot/dataset",
		denied: []string{"catalog", "home", "invocation", "invoke", "server", "client", "object", "discovery/service", "discovery", "declarativeindex", "discovery/projection", "controlplane", "connector", "hook", "gate", "snapshot/treewriter", "snapshot/gitea", "snapshot/lakefs", "cli"},
		why:    "Snapshot Store owns file Dataset definitions and release authority; it cannot depend on consumers, knowledge interpretation, or concrete storage providers",
	},
	{
		pkg:    "snapshot/access",
		denied: []string{"snapshot", "catalog", "home", "invocation", "invoke", "server", "client", "identity", "object", "discovery/service", "discovery", "declarativeindex", "discovery/projection", "controlplane", "connector", "hook", "gate", "cli"},
		why:    "generic scoped access rules depend only on coordinates/errors; external application defaults and identity lifecycle stay outside",
	},
	{
		pkg:    "snapshot/service",
		denied: []string{"catalog", "home", "invocation", "invoke", "server", "client", "object", "discovery/service", "discovery", "declarativeindex", "discovery/projection", "controlplane", "connector", "hook", "gate", "snapshot/gitea", "snapshot/lakefs", "cli"},
		why:    "the independent file service assembles Dataset/access/store ports and external identity only; it never uses the KC application as a hidden backend",
	},
	{
		pkg:    "snapshot/console",
		denied: []string{"snapshot", "catalog", "home", "invocation", "invoke", "server", "client", "identity", "object", "discovery", "declarativeindex", "discovery/projection", "controlplane", "cli"},
		why:    "the complete vendored console is independently served static UI with no application or store authority",
	},
	{
		pkg:    "snapshot/client",
		denied: []string{"catalog", "home", "invocation", "invoke", "server", "client", "cli", "identity", "object", "discovery", "declarativeindex", "controlplane", "snapshot/service", "snapshot/console", "snapshot/cli"},
		why:    "the independent Snapshot SDK injects authenticated transport and uses owned wire contracts, never KC session state, service implementation, or command dispatch",
	},
	{
		pkg:    "snapshot/cli",
		denied: []string{"catalog", "home", "invocation", "server", "client", "cli", "identity", "object", "discovery", "declarativeindex", "controlplane", "snapshot/service", "snapshot/console"},
		why:    "module task commands consume the SDK and explicit caller context, never KC application executors or a locally opened authority",
	},
	{
		pkg:    "object/client",
		denied: []string{"home", "invocation", "invoke", "server", "client", "cli", "identity", "object/service", "object/reader", "object/writer", "object/cli", "liveaccess", "discovery", "declarativeindex"},
		why:    "the Object SDK uses caller-owned authenticated transport and Object contracts without application state or capability execution",
	},
	{
		pkg:    "object/cli",
		denied: []string{"home", "invocation", "server", "client", "cli", "identity", "object/service", "object/reader", "liveaccess", "discovery", "declarativeindex"},
		why:    "Object commands and extensions own argv, help and result presentation through their SDK, never the KC host or a local authority",
	},
	{
		pkg:    "discovery/client",
		denied: []string{"home", "invocation", "invoke", "server", "client", "cli", "identity", "object/service", "object/writer", "discovery/service", "discovery/plugin", "discovery/projection", "declarativeindex"},
		why:    "Discovery SDK transports typed requests through borrowed authentication without selecting or constructing capability implementations",
	},
	{
		pkg:    "discovery/cli",
		denied: []string{"home", "invocation", "invoke", "server", "client", "cli", "identity", "object/service", "object/writer", "discovery/service", "discovery/plugin", "discovery/projection", "declarativeindex"},
		why:    "Discovery commands own query inputs, products and help through their independent SDK; hosts only assemble context",
	},
	{
		pkg:    "declarativeindex/client",
		denied: []string{"home", "invocation", "invoke", "server", "client", "cli", "identity", "object/service", "object/reader", "object/writer", "discovery/service", "discovery/plugin", "discovery/projection", "declarativeindex/control", "declarativeindex/projection", "declarativeindex/plugin", "declarativeindex/opensearch", "declarativeindex/internal"},
		why:    "Index maintenance SDK never holds or starts the implementation behind its authenticated server tasks",
	},
	{
		pkg:    "declarativeindex/cli",
		denied: []string{"home", "invocation", "invoke", "server", "client", "cli", "identity", "object/writer", "discovery/plugin", "declarativeindex/control", "declarativeindex/projection", "declarativeindex/plugin", "declarativeindex/opensearch", "declarativeindex/internal"},
		why:    "Index task CLI provides static affordance and SDK adaptation without host state or backend lifecycle ownership",
	},
	{
		pkg:    "internal/httpjson",
		denied: []string{"snapshot", "object", "discovery", "declarativeindex", "identity", "home", "invocation", "invoke", "server", "client", "cli", "controlplane", "observability"},
		why:    "shared HTTP encoding and bounded decoding are pure transport infrastructure, not typed domain or session authority",
	},
	{
		pkg:    "catalog",
		denied: []string{"object", "repository", "discovery", "declarativeindex", "discovery/projection", "object/reader", "object/writer", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "discovery/opensearch", "cli"},
		why:    "layer ① composes repo refs and Workspace recipes; it must not know object_id, Aspect, IndexPlan, or any concrete store",
	},
	{
		pkg:    "catalog/worktree",
		denied: []string{"object", "repository", "discovery", "declarativeindex", "discovery/projection", "object/reader", "object/writer", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "discovery/opensearch", "cli"},
		why:    "host git checkout consumes Catalog recipes and pins; it is still layer ① and must not know object_id, Aspect, or any concrete store",
	},
	{
		pkg:    "catalog/internal/worktreeimpl",
		denied: []string{"object", "repository", "discovery", "declarativeindex", "discovery/projection", "object/reader", "object/writer", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "discovery/opensearch", "cli"},
		why:    "worktree implementation (B11 internalization) obeys the same layer-① rule as its shim; no object_id, Aspect, or concrete store",
	},
	{
		pkg:    "object/writer",
		denied: []string{"repository", "discovery", "declarativeindex", "discovery/projection", "catalog", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "discovery/opensearch", "cli"},
		why:    "the write surface must not depend on retrieval derivations, composition, or a concrete store",
	},
	{
		pkg:    "object/reader",
		denied: []string{"repository", "object/maintenance", "discovery", "declarativeindex", "discovery/projection", "catalog", "object/writer", "connector", "hook", "snapshot/treewriter", "snapshot/gitea", "discovery/opensearch", "cli", "delivery"},
		why:    "layer ② exact assembly consumes coordinates it is handed; maintenance scanning, index execution, and caller-visible delivery stay outside the consumer reader",
	},
	{
		pkg:    "liveaccess",
		denied: []string{"repository", "discovery", "declarativeindex", "discovery/projection", "catalog", "object/writer", "connector", "hook", "gate", "snapshot/treewriter", "snapshot/gitea", "discovery/opensearch", "cli", "client", "delivery"},
		why:    "consumer Knowledge Serving may compose Reader with an injected State port, but must not own providers, credentials, composition, writes, retrieval, or caller-visible delivery",
	},
	{
		pkg:    "discovery/plugin",
		denied: []string{"home", "invocation", "invoke", "server", "httpsurface", "client", "cli", "identity", "object/service", "object/writer", "object/client", "object/cli", "snapshot/service", "snapshot/client", "snapshot/cli", "discovery/service", "discovery/client", "discovery/cli", "discovery/opensearch", "discovery/internal/opensearchimpl", "discovery/llmhttp", "declarativeindex"},
		why:    "typed contribution selection consumes public query, observation and fixed-read ports; hosts and concrete entries assemble it, and transports, execution services, physical drivers and Object publication stay outside",
	},
	{
		pkg:    "discovery/service",
		denied: []string{"home", "cli", "httpsurface", "client", "snapshot/gitea", "discovery/opensearch", "declarativeindex/opensearch", "declarativeindex/projection", "declarativeindex/control", "declarativeindex/plugin", "declarativeindex/internal"},
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
		why:    "the operation layer is import-neutral between the transports: verbs consume the use-case cores (object/service, discovery/service) and the carrier holds the transport-opened *home.Home, but the seam never knows an argv front end or a service surface (CAP-ORG-01 B5a carrier, B6 operation layer)",
	},
	{
		pkg:    "snapshot/treewriter",
		denied: []string{"object", "object/reader", "object/writer", "catalog", "discovery", "declarativeindex", "discovery/projection", "controlplane", "connector", "snapshot/gitea", "cli"},
		why:    "literal tree mutation is layer ⓪ and must not acquire Knowledge or composition semantics",
	},
	{
		pkg:    "discovery",
		denied: []string{"catalog", "object/writer", "discovery/projection", "declarativeindex/control", "declarativeindex/projection", "controlplane", "connector", "snapshot/treewriter", "snapshot/gitea", "discovery/opensearch", "declarativeindex/opensearch", "cli", "delivery"},
		why:    "layer ③ logical retrieval contracts may consume Knowledge declarations but not providers, application wiring, or caller-visible delivery",
	},
	{
		pkg:    "discovery/projection",
		denied: []string{"repository", "catalog", "object/writer", "snapshot/gitea", "discovery/opensearch", "declarativeindex", "cli", "delivery"},
		why:    "generic projection contracts and shared request scopes do not construct an index, own control state, or bind authority/provider implementations",
	},
	{
		pkg:    "declarativeindex/projection",
		denied: []string{"repository", "catalog", "home", "invocation", "invoke", "server", "cli", "client", "identity", "object/writer", "snapshot/gitea", "delivery", "liveaccess", "systemrepo", "discovery/datasets", "discovery/service", "discovery/plugin", "declarativeindex/control", "declarativeindex/plugin", "declarativeindex/opensearch", "declarativeindex/internal"},
		why:    "the concrete index consumes generic contracts, fixed readers and injected engines; queries never depend on their maintenance controller or host assembly",
	},
	{
		pkg:    "declarativeindex/control",
		denied: []string{"repository", "catalog", "home", "invocation", "invoke", "server", "cli", "client", "identity", "object/writer", "snapshot/gitea", "delivery", "liveaccess", "systemrepo", "discovery/datasets", "discovery/service", "discovery/plugin", "declarativeindex/plugin", "declarativeindex/opensearch", "declarativeindex/internal"},
		why:    "index control reconciles selected repository and serving bases through its maintenance port; Dataset restoration, Object coordination and concrete engine construction are composed elsewhere",
	},
	{
		pkg:    "declarativeindex/opensearch",
		denied: []string{"home", "invocation", "invoke", "server", "cli", "client", "identity", "object/writer", "declarativeindex/control", "declarativeindex/plugin", "discovery/service", "discovery/plugin", "discovery/datasets"},
		why:    "the concrete OpenSearch adapter owns physical projection I/O behind public engine ports, not application tasks or maintenance policy",
	},
	{
		pkg:    "declarativeindex/opensearch/internal/opensearchimpl",
		denied: []string{"home", "invocation", "invoke", "server", "cli", "client", "identity", "object/writer", "declarativeindex/control", "declarativeindex/plugin", "discovery/service", "discovery/plugin", "discovery/datasets"},
		why:    "the private OpenSearch implementation preserves the same boundary as its module-owned public adapter",
	},
	{
		pkg:    "declarativeindex/plugin",
		denied: []string{"home", "invocation", "invoke", "server", "cli", "client", "identity", "object/writer"},
		why:    "the real contribution entry assembles its index and controller with supplied scope and resources, without importing the host that installs it",
	},
	{
		pkg:    "integrationruntime",
		denied: []string{"home", "object/writer", "snapshot/gitea", "snapshot/treewriter", "cli"},
		why:    "the wall-out integration runtime only uses reconciliation and typed client APIs, never Server state or authority implementations",
	},
	{
		pkg:    "connector",
		denied: []string{"repository", "catalog", "object/writer", "object/reader", "discovery/projection", "declarativeindex/control", "declarativeindex/projection", "controlplane", "hook", "gate", "snapshot/gitea", "discovery/opensearch", "declarativeindex/opensearch", "cli"},
		why:    "the Collector reconciliation helper only produces ChangeSets; the wall-out caller drives source access and Writer",
	},
	{
		pkg:    "hook",
		denied: []string{"catalog", "object/writer", "object/reader", "discovery/projection", "declarativeindex/control", "declarativeindex/projection", "controlplane", "snapshot", "object", "repository", "cli"},
		why:    "outbound hooks are a CLI concern; the protocol packages must not call user systems",
	},
	{
		pkg:    "delivery",
		denied: []string{"cli", "catalog", "discovery/projection", "discovery", "declarativeindex", "discovery/opensearch", "client", "hook", "gate", "object/reader", "object/writer", "liveaccess"},
		why:    "delivery rewrites a hydrated envelope for the caller; it must not locate, index, assemble Canonical, or read allow.json",
	},
	{
		pkg:    "snapshot/commandlog",
		denied: []string{"snapshot", "object", "catalog", "discovery", "declarativeindex", "discovery/projection", "controlplane", "connector", "cli"},
		why:    "the shared command ledger owns replay mechanics, not any write surface's domain payload",
	},
	{
		pkg:    "internal/gitdir",
		denied: []string{"snapshot", "object", "repository", "catalog", "object/writer", "object/reader", "discovery/projection", "declarativeindex/control", "declarativeindex/projection", "snapshot/gitea"},
		why:    "gitdir is git plumbing shared by layer ⓪ adapters and the layer ① registry; it must stay below both",
	},
	{
		pkg:    "object/format",
		denied: []string{"repository", "catalog", "object/writer", "object/reader", "discovery/projection", "declarativeindex/control", "declarativeindex/projection", "snapshot/gitea"},
		why:    "repofile is the on-disk unit format, not a store",
	},
	{
		pkg:    "snapshot/gitea",
		denied: []string{"repository", "object", "object/format", "catalog", "object/writer", "object/reader", "discovery/projection", "declarativeindex/control", "declarativeindex/projection", "controlplane", "discovery/opensearch", "declarativeindex/opensearch", "cli"},
		why:    "the Gitea adapter exposes only Snapshot paths, commits, refs, history, and CAS",
	},
	{
		pkg:    "snapshot/lakefs",
		denied: []string{"repository", "object", "object/format", "catalog", "object/writer", "object/reader", "discovery/projection", "declarativeindex/control", "declarativeindex/projection", "controlplane", "discovery/opensearch", "declarativeindex/opensearch", "cli"},
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
		denied: []string{"snapshot", "catalog", "object", "discovery", "declarativeindex", "discovery/projection", "hook", "controlplane", "cli", "httpsurface", "client", "home"},
		why:    "the merge evidence gate reads only its rules and evidence bindings, fail-closed; it must not reach into protocol layers, hooks, or transports",
	},
	{
		pkg:    "identity",
		denied: []string{"snapshot", "catalog", "object", "discovery", "declarativeindex", "discovery/projection", "hook", "controlplane", "cli", "httpsurface", "client", "home"},
		why:    "identity binding reads its own durable store and IdP results only; isolation matches gate's strict set",
	},
	{
		pkg:    "datasetfs",
		denied: []string{"kernel", "snapshot", "catalog", "object", "discovery", "declarativeindex", "discovery/projection", "controlplane", "connector", "integrationruntime", "hook", "gate", "delivery", "identity", "observability", "invoke", "client", "discovery/service", "cli", "httpsurface", "home"},
		why:    "the local read-only consumption view mounts what a plan hands it; it never calls back into the protocol ring or any transport",
	},
	{
		pkg:    "observability",
		denied: []string{"discovery", "declarativeindex", "discovery/projection", "controlplane", "hook", "gate", "cli", "httpsurface", "client", "home"},
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
		{"object/writer", "object/writer", true},
		{"object/writer", "object/writer/extra", true},
		{"object/writer", "object/reader", false},
		{"discovery/opensearch", "discovery/opensearch", true},
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
		if dependency == "catalog" || dependency == "home" || dependency == "cli" || dependency == "object/reader" {
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
		"interfaces", "interfaces/service", "liveaccess/service", "liveaccess/contract", "liveaccess/discovery", "object/cache", "object/format", "systemrepo",
		"kernel", "snapshot", "snapshot/dataset", "snapshot/access", "object", "catalog", "catalog/worktree", "object/writer", "object/reader", "liveaccess",
		"catalog/internal/worktreeimpl",
		"discovery", "discovery/projection", "controlplane", "connector", "hook", "gate",
		"snapshot/treewriter", "snapshot/gitea", "snapshot/lakefs",
		"declarativeindex/projection", "declarativeindex/control", "declarativeindex/opensearch", "declarativeindex/opensearch/internal/opensearchimpl", "discovery/llmhttp", "observability",
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
	if _, exists := graph["catalog"]; exists {
		t.Error("Catalog forwarding implementation must be retired")
	}
	for dependency, trail := range graph.reachable("snapshot/dataset") {
		for _, denied := range []string{"object", "discovery/projection", "discovery", "home", "invocation", "server", "internal/gitdir", "internal/journal"} {
			if deniedMatch(denied, dependency) {
				t.Errorf("Dataset reaches %s through %v", dependency, trail)
			}
		}
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
		"dataset":   {"base": true, "infra": true, "snapshot": true, "dataset": true},
		"object":    {"base": true, "infra": true, "snapshot": true, "object": true},
		"discovery": {"base": true, "infra": true, "snapshot": true, "object": true, "discovery": true},
		"app":       {"base": true, "infra": true, "snapshot": true, "dataset": true, "object": true, "discovery": true, "app": true},
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
	case "kernel", "discovery/adoption":
		return "base", true
	case "internal/gitdir", "internal/journal", "internal/jsonfile", "internal/treepath", "internal/httpjson":
		return "infra", true
	case "invocation":
		return "app", true
	case "snapshot", "snapshot/commandlog", "snapshot/gitea", "snapshot/lakefs", "snapshot/stamp", "snapshot/treewriter",
		"snapshot/internal/giteaimpl", "snapshot/internal/lakefsimpl", "snapshot/internal/storecontract", "snapshot/internal/stamp", "snapshot/access":
		return "snapshot", true
	case "catalog", "snapshot/dataset", "catalog/worktree", "catalog/internal/worktreeimpl":
		return "dataset", true
	case "object/format", "object", "object/maintenance", "object/reader",
		"object/semanticview", "object/writer",
		"object/cache", "interfaces", "liveaccess/contract", "liveaccess/evidence", "interfaces/service":
		return "object", true
	case "discovery", "discovery/llmhttp", "liveaccess", "discovery/projection", "declarativeindex/projection", "declarativeindex/control", "declarativeindex/opensearch",
		"declarativeindex/opensearch/internal/opensearchimpl":
		return "discovery", true
	case "snapshot/service", "snapshot/console", "snapshot/client", "snapshot/cli", "object/client", "object/cli", "discovery/client", "discovery/cli", "declarativeindex/client", "declarativeindex/cli", "declarativeindex/plugin", "declarativeindex/internal/testfixture", "liveaccess/service", "liveaccess/discovery", "liveaccess/cli", "liveaccess/client", "liveaccess/plugin", "liveaccess/fixture", "cli", "cli/serve", "client", "cmd/kc", "cmd/kcfs", "cmd/kc-integration", "integrationruntime", "invoke", "identity", "connector", "controlplane", "gate", "hook",
		"home", "httpsurface", "server", "server/internal/readiness", "internal/telemetry", "internal/testkit", "datasetfs", "delivery", "object/service", "controlplane/service", "discovery/datasets", "discovery/service", "discovery/plugin", "systemrepo", "observability",
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
			if imported != modulePath+"/snapshot/internal/lakefsimpl" {
				continue
			}
			allowed := filepath.Dir(rel) == "snapshot" || strings.HasPrefix(rel, "snapshot/internal/lakefsimpl/")
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
		"invocation/verbs_read.go", "invocation/repository_read.go", "object/service/read.go",
		"declarativeindex/projection/search.go", "declarativeindex/projection/relations.go", "discovery/plugin/queries.go", "discovery/service/search.go",
		"object/service/dataset_read.go", "discovery/service/dataset_search.go", "discovery/service/dataset_relations.go",
		"object/reader/reader.go", "object/reader/repository_service.go", "liveaccess/serving.go",
		"liveaccess/service/hydrate.go", "liveaccess/service/repository_read.go",
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
	homeFiles, err := filepath.Glob(filepath.Join(root, "home", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(homeFiles) == 0 {
		t.Fatal("cold Home ownership has no production files")
	}
	files := append(homeFiles,
		filepath.Join(root, "server", "serve_facade.go"), filepath.Join(root, "server", "serve_handler.go"), filepath.Join(root, "server", "serve_close.go"),
		filepath.Join(root, "server", "plugin_help.go"), filepath.Join(root, "server", "service_invoke.go"),
		filepath.Join(root, "server", "snapshot_service.go"), filepath.Join(root, "cli", "serve.go"))
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, path)
		for _, signal := range projectionWorkerActivationSignals(filepath.ToSlash(rel), file) {
			t.Errorf("%s: %s", rel, signal)
		}
	}
}

func TestApplicationPackageBoundaries(t *testing.T) {
	graph := loadGraph(t)
	protocol := []string{
		"kernel", "snapshot", "snapshot/dataset", "snapshot/access", "object", "catalog", "catalog/worktree", "object/writer", "object/reader", "liveaccess",
		"discovery", "discovery/projection", "controlplane", "connector", "hook", "gate",
		"snapshot/treewriter", "snapshot/gitea", "snapshot/lakefs",
		"declarativeindex/projection", "declarativeindex/control", "declarativeindex/opensearch", "declarativeindex/opensearch/internal/opensearchimpl", "discovery/llmhttp", "observability",
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
		"DeploymentConfig": true, "RegistryBinding": true, "DefaultStores": true,
		"StoresFile": true, "RepositoryBinding": true, "RepositoryAccess": true,
		"SystemRepositoryAccess": true, "PrepareFixtureAuthority": true,
		"PrepareRegistryAuthority": true,
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
		"identity":      {"cli", "home", "invocation", "server", "snapshot/service"},
		"observability": {"cli", "client", "home", "invocation", "server", "liveaccess/service"},
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
		"declarativeindex/opensearch/internal/opensearchimpl": {
			"discovery/llmhttp": "the OpenSearch engine assembles the llmhttp embedder (B12 adapter-internal composition)",
		},
	}
	// Snapshot keeps its existing internal implementation rule. OpenSearch's
	// implementation is private to its public adapter capability: even Home and
	// peer packages in declarativeindex must use that actual public adapter.
	adapters := []string{
		"snapshot/internal/lakefsimpl",
		"declarativeindex/opensearch/internal/opensearchimpl", "discovery/llmhttp", "object/cache",
	}
	for pkg, deps := range graph {
		for _, adapter := range adapters {
			// Only exact adapter imports are provider consumption; importing
			// the capability root (kc/retrieval) is normal interface use.
			if !slices.Contains(deps, adapter) {
				continue
			}
			if adapter == "declarativeindex/opensearch/internal/opensearchimpl" {
				if !privateOpenSearchConsumerAllowed(pkg) {
					t.Errorf("%s imports private OpenSearch implementation %s; only its public adapter and implementation tree may", pkg, adapter)
				}
				continue
			}
			root := strings.SplitN(adapter, "/", 2)[0]
			if pkg == "home" || pkg == root || strings.HasPrefix(pkg, root+"/") {
				continue
			}
			if shim, ok := map[string]string{
				"snapshot/internal/lakefsimpl": "snapshot",
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

func privateOpenSearchConsumerAllowed(pkg string) bool {
	return pkg == "declarativeindex/opensearch" || deniedMatch("declarativeindex/opensearch/internal/opensearchimpl", pkg)
}

func TestApplicationCoreHasNoTransportOrProviderImports(t *testing.T) {
	graph := loadGraph(t)
	reachable := graph.reachable("discovery/service")
	for _, denied := range []string{
		"home", "cli", "httpsurface", "client",
		"snapshot/gitea", "snapshot/lakefs", "declarativeindex/projection", "declarativeindex/control", "declarativeindex/opensearch", "declarativeindex/opensearch/internal/opensearchimpl",
	} {
		if path, ok := reachable[denied]; ok {
			t.Errorf("discovery/service reaches %s through %s", denied, strings.Join(path, " -> "))
		}
	}
	root := moduleRoot(t)
	for _, rel := range []string{"object/service/read.go", "discovery/service/search.go", "object/service/write.go"} {
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
	files := productionSourceFiles(t, func(owner string) bool {
		return owner == "invocation" || owner == "cli" || owner == "server"
	})
	// Embedded CLI registration is test harness assembly. Its actual Run
	// bindings must still agree with the independent typed HTTP registration.
	for path, file := range rootCLISourceFiles(t, true) {
		if file.Name.Name == "cli" {
			files[path] = file
		}
	}
	for _, signal := range typedExecutorWiringSignals(files) {
		t.Error(signal)
	}
}

// Semantic object caches are an upper-product concern. Request-local maps are
// allowed, but Reader and Snapshot adapter structs/package variables must not
// retain ObjectID/KnowledgeRef -> KnowledgeValue/CanonicalRelation state.
func TestLowerLayersDoNotDeclareSemanticObjectCaches(t *testing.T) {
	root := moduleRoot(t)
	prefixes := []string{"object/reader", "snapshot"}
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
	case ".git", ".data", ".venv", ".kc", ".validation", ".session_tmps", "node_modules", "docs":
		return true
	}
	// webui is the vendored lakeFS frontend (its own Go module and npm build;
	// see snapshot/console/webui/KC-VENDOR.md). Its Go file is not a production package of this
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
