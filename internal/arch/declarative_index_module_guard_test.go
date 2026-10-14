package arch_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"slices"
	"strings"
	"testing"
)

func declarativeIndexGuardFixture(t *testing.T, path, source string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

func declarativeIndexOwnedFixture(t *testing.T) map[string]*ast.File {
	t.Helper()
	return map[string]*ast.File{
		"discovery/projection/contract.go": declarativeIndexGuardFixture(t, "contract.go", `package projection
type IndexDescriptor struct{}
type IndexSync struct{}
type StateSync struct{}
type ObservationSource interface{}
type ChangeNotice struct{}
type SearchBudget struct{}
func WithSearchBudget() {}
func SearchBudgetExhausted() {}
func WithCandidateScope() {}
`),
		"declarativeindex/projection/index.go": declarativeIndexGuardFixture(t, "index.go", `package projection
type Index struct{}
func NewIndexEngine() *Index { return nil }
`),
		"declarativeindex/projection/plan.go": declarativeIndexGuardFixture(t, "plan.go", `package projection
type ProjectionSpec struct{}
type RetrievalPlan struct{}
type RetrievalFragment struct{}
func PlanRetrieval() RetrievalPlan { return RetrievalPlan{} }
`),
		"declarativeindex/control/controller.go": declarativeIndexGuardFixture(t, "controller.go", `package control
type Controller struct{}
type TargetStore struct{}
func NewController() *Controller { return nil }
func NewTargetStore() *TargetStore { return nil }
`),
	}
}

func TestDeclarativeIndexOwnershipGuardRejectsAliasesAndForwarders(t *testing.T) {
	base := declarativeIndexOwnedFixture(t)
	if signals := declarativeIndexOwnershipSignals(base); len(signals) != 0 {
		t.Fatalf("correct concrete ownership rejected: %v", signals)
	}
	for _, tc := range []struct{ name, path, source, want string }{
		{"generic-index", "discovery/projection/index.go", `package projection; type Index struct{}`, "must define the real Index struct"},
		{"generic-index-alias", "discovery/projection/index.go", `package projection; import concrete "kc/declarativeindex/projection"; type Index = concrete.Index`, "without an implementation alias"},
		{"generic-controller-alias", "discovery/projection/control.go", `package projection; import concrete "kc/declarativeindex/control"; type Controller = concrete.Controller`, "must define the real Controller struct"},
		{"generic-factory-wrapper", "discovery/projection/index.go", `package projection; import concrete "kc/declarativeindex/projection"; func NewIndexEngine() *concrete.Index { return concrete.NewIndexEngine() }`, "declares concrete factory NewIndexEngine"},
		{"generic-factory-variable", "discovery/projection/index.go", `package projection; import concrete "kc/declarativeindex/projection"; var NewIndexEngine = concrete.NewIndexEngine`, "factory forwarding value"},
		{"generic-controller-method", "discovery/projection/control.go", `package projection; func (c *Controller) Start() {}`, "retains concrete Controller method Start"},
		{"generic-planner", "discovery/projection/plan.go", `package projection; func PlanRetrieval() {}`, "declares concrete factory PlanRetrieval"},
		{"generic-planner-alias", "discovery/projection/plan.go", `package projection; import concrete "kc/declarativeindex/projection"; type RetrievalPlan = concrete.RetrievalPlan`, "must define the real RetrievalPlan struct"},
		{"concrete-index-alias", "declarativeindex/projection/index.go", `package projection; import old "kc/discovery/projection"; type Index = old.Index; var NewIndexEngine = old.NewIndexEngine`, "without an implementation alias"},
		{"concrete-defined-forwarding-type", "declarativeindex/control/controller.go", `package control; import old "kc/discovery/projection"; type Controller old.Controller`, "must define the real Controller struct"},
		{"retired-adapter-shim", "discovery/opensearch/shim.go", `package opensearch; import engine "kc/declarativeindex/opensearch"; var Open = engine.Open`, "retired concrete index implementation or adapter shim"},
		{"retired-private-engine", "discovery/internal/opensearchimpl/engine.go", `package opensearchimpl; type engine struct{}`, "retired concrete index implementation or adapter shim"},
		{"retired-module-private-engine", "declarativeindex/internal/opensearchimpl/engine.go", `package opensearchimpl; type engine struct{}`, "retired concrete index implementation or adapter shim"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := maps.Clone(base)
			files[tc.path] = declarativeIndexGuardFixture(t, tc.path, tc.source)
			if signals := declarativeIndexOwnershipSignals(files); !slices.ContainsFunc(signals, func(signal string) bool { return strings.Contains(signal, tc.want) }) {
				t.Fatalf("concrete ownership guard did not report %q: %v", tc.want, signals)
			}
		})
	}
	for _, missing := range []string{"declarativeindex/projection/index.go", "declarativeindex/control/controller.go"} {
		files := maps.Clone(base)
		delete(files, missing)
		if signals := declarativeIndexOwnershipSignals(files); len(signals) == 0 {
			t.Fatalf("concrete ownership guard accepted missing owner %s", missing)
		}
	}
}

func TestDeclarativeIndexSharedContractGuardRejectsCopiedScopes(t *testing.T) {
	base := declarativeIndexOwnedFixture(t)
	base["declarativeindex/projection/dto_alias.go"] = declarativeIndexGuardFixture(t, "dto_alias.go", `package projection
import common "kc/discovery/projection"
type IndexSync = common.IndexSync
type StateSync = common.StateSync
`)
	if signals := declarativeIndexSharedContractSignals(base); len(signals) != 0 {
		t.Fatalf("single shared definition rejected: %v", signals)
	}
	for _, tc := range []struct{ name, source, want string }{
		{"budget-type-copy", `package projection; type SearchBudget struct{}`, "shared projection contract SearchBudget"},
		{"budget-function-copy", `package projection; func WithSearchBudget() {}`, "shared projection contract WithSearchBudget"},
		{"budget-factory-alias", `package projection; import common "kc/discovery/projection"; var WithSearchBudget = common.WithSearchBudget`, "shared projection contract WithSearchBudget"},
		{"scope-wrapper", `package projection; import common "kc/discovery/projection"; func WithCandidateScope() { common.WithCandidateScope() }`, "shared projection contract WithCandidateScope"},
		{"state-dto-copy", `package projection; type StateSync struct{}`, "shared projection contract StateSync"},
		{"observation-port-copy", `package projection; type ObservationSource interface{}`, "shared projection contract ObservationSource"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := maps.Clone(base)
			files["declarativeindex/projection/copy.go"] = declarativeIndexGuardFixture(t, "copy.go", tc.source)
			if signals := declarativeIndexSharedContractSignals(files); !slices.ContainsFunc(signals, func(signal string) bool { return strings.Contains(signal, tc.want) }) {
				t.Fatalf("shared contract guard did not report %q: %v", tc.want, signals)
			}
		})
	}
	files := maps.Clone(base)
	delete(files, "discovery/projection/contract.go")
	if signals := declarativeIndexSharedContractSignals(files); len(signals) == 0 {
		t.Fatal("shared contract guard accepted missing common definitions")
	}
}

func TestDeclarativeIndexTestOwnershipGuardRejectsGenericRuntimeFixtures(t *testing.T) {
	for _, tc := range []struct {
		name, path, source string
		denied             bool
	}{
		{"old-index-construction", "discovery/projection/index_test.go", `package projection_test; import index "kc/discovery/projection"; func fixture() { index.NewIndexEngine() }`, true},
		{"private-controller-construction", "discovery/projection/control_test.go", `package projection; func fixture() { NewController() }`, true},
		{"new-runtime-import", "discovery/projection/index_test.go", `package projection_test; import index "kc/declarativeindex/projection"; var _ = index.NewIndexEngine`, true},
		{"new-controller-import", "discovery/projection/control_test.go", `package projection_test; import index "kc/declarativeindex/control"; var _ = index.NewController`, true},
		{"generic-registry-runtime-import", "discovery/plugin/contribution_test.go", `package plugin_test; import impl "kc/declarativeindex/plugin"; var _ = impl.Entry`, true},
		{"generic-registry-contract", "discovery/plugin/config_test.go", `package plugin_test; import common "kc/discovery/plugin"; var _ = common.OpenIndexPlugins`, false},
		{"old-index-literal", "discovery/projection/index_test.go", `package projection; var value = &Index{}`, true},
		{"shared-budget", "discovery/projection/execution_test.go", `package projection_test; import common "kc/discovery/projection"; func fixture() { common.WithSearchBudget() }`, false},
		{"owned-index-fixture", "declarativeindex/projection/index_test.go", `package projection_test; import index "kc/declarativeindex/projection"; func fixture() { index.NewIndexEngine() }`, false},
		{"owned-control-fixture", "declarativeindex/control/control_test.go", `package control; func fixture() { NewController() }`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			signals := declarativeIndexGenericTestSignals(tc.path, declarativeIndexGuardFixture(t, tc.path, tc.source))
			if (len(signals) != 0) != tc.denied {
				t.Fatalf("test ownership guard result %v, want violation=%v", signals, tc.denied)
			}
		})
	}
}

func TestDeclarativeIndexDependencyGuardRejectsIndirectImplementation(t *testing.T) {
	for _, tc := range []struct {
		name, owner, target string
		denied              bool
	}{
		{"generic-query", "discovery/service", "declarativeindex/projection", true},
		{"generic-port", "discovery/projection", "declarativeindex/projection", true},
		{"registry", "discovery/plugin", "declarativeindex/plugin", true},
		{"dataset-recovery", "discovery/datasets", "declarativeindex/control", true},
		{"generic-sdk", "discovery/client", "declarativeindex/projection", true},
		{"generic-cli", "discovery/cli", "declarativeindex/control", true},
		{"owned-sdk", "declarativeindex/client", "declarativeindex/plugin", true},
		{"owned-cli", "declarativeindex/cli", "declarativeindex/adapter/opensearch", true},
		{"private-runtime", "object/client", "declarativeindex/opensearch/internal/opensearchimpl", true},
		{"renamed-runtime", "snapshot/cli", "declarativeindex/another-engine", true},
		{"query-control", "declarativeindex/projection", "declarativeindex/control/internal/worker", true},
		{"query-contract", "declarativeindex/projection", "discovery/projection", false},
		{"control-query", "declarativeindex/control", "declarativeindex/projection", false},
		{"plugin-assembly", "declarativeindex/plugin", "declarativeindex/control", false},
		{"home-assembly", "home", "declarativeindex/plugin", false},
		{"cli-sdk", "declarativeindex/cli", "declarativeindex/client", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			graphs := []depGraph{{tc.owner: {tc.target}}}
			if tc.owner != "home" {
				graphs = append(graphs, depGraph{tc.owner: {tc.owner + "/bridge"}, tc.owner + "/bridge": {tc.target}})
			}
			for _, graph := range graphs {
				signals := declarativeIndexDependencySignals(graph)
				if (len(signals) != 0) != tc.denied {
					t.Fatalf("implementation direction guard result %v, want violation=%v", signals, tc.denied)
				}
			}
		})
	}
	// Transports retain their existing route through the assembly owner. They
	// cannot become additional factories by directly importing the plugin.
	for _, host := range []string{"server", "cli"} {
		if signals := declarativeIndexDependencySignals(depGraph{host: {"home"}, "home": {"declarativeindex/plugin"}}); len(signals) != 0 {
			t.Fatalf("transport through Home assembly rejected: %v", signals)
		}
		if signals := declarativeIndexDependencySignals(depGraph{host: {"declarativeindex/plugin"}}); len(signals) == 0 {
			t.Fatalf("transport %s accepted as an extra concrete factory", host)
		}
	}
}

func TestDeclarativeIndexLayerGuardRejectsNewOwnerBackEdges(t *testing.T) {
	for _, tc := range []struct {
		owner, target string
		denied        bool
	}{
		{"declarativeindex/projection", "declarativeindex/control/internal/worker", true},
		{"declarativeindex/projection", "declarativeindex/opensearch", true},
		{"declarativeindex/projection", "home", true},
		{"declarativeindex/projection", "liveaccess", true},
		{"declarativeindex/projection", "discovery/projection", false},
		{"declarativeindex/control", "discovery/datasets", true},
		{"declarativeindex/control", "object/writer", true},
		{"declarativeindex/control", "home", true},
		{"declarativeindex/control", "discovery/projection", false},
		{"declarativeindex/opensearch", "discovery/plugin", true},
		{"declarativeindex/opensearch/internal/opensearchimpl", "declarativeindex/control", true},
		{"declarativeindex/plugin", "home", true},
		{"declarativeindex/plugin", "declarativeindex/control", false},
		{"discovery/projection", "declarativeindex/projection", true},
		{"discovery/service", "declarativeindex/control", true},
		{"declarativeindex/client", "declarativeindex/opensearch/internal/opensearchimpl", true},
		{"declarativeindex/cli", "declarativeindex/opensearch", true},
	} {
		t.Run(tc.owner+"/"+tc.target, func(t *testing.T) {
			graph := depGraph{tc.owner: {"bridge"}, "bridge": {tc.target}}
			if got := pluginDependencyForbidden(t, graph, tc.owner); got != tc.denied {
				t.Fatalf("new owner boundary forbidden=%v want=%v", got, tc.denied)
			}
		})
	}
	for _, owner := range []string{"declarativeindex/projection", "declarativeindex/control", "declarativeindex/opensearch", "declarativeindex/opensearch/internal/opensearchimpl"} {
		if layer, assigned := architectureLayer(owner); !assigned || layer != "discovery" {
			t.Fatalf("concrete projection capability %s has incorrect layer %q, assigned=%v", owner, layer, assigned)
		}
	}
	if layer, assigned := architectureLayer("declarativeindex/internal/testfixture"); !assigned || layer != "app" {
		t.Fatalf("private test assembly has incorrect layer %q, assigned=%v", layer, assigned)
	}
}

func TestDeclarativeIndexCoordinatorGuardRejectsPrivateObjectLoop(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		denied       bool
	}{
		{"construct", `package control; import maint "kc/object/maintenance"; func f() { maint.NewCoordinator() }`, true},
		{"factory-alias", `package control; import maint "kc/object/maintenance"; var f = maint.NewCoordinator`, true},
		{"owned-field", `package control; import maint "kc/object/maintenance"; type Controller struct { loop *maint.Coordinator }`, true},
		{"public-port", `package control; import maint "kc/object/maintenance"; var inventory maint.InventoryFunc`, false},
		{"other-provider", `package control; import maint "another/maintenance"; func f() { maint.NewCoordinator() }`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			signals := declarativeIndexObjectCoordinatorSignals("declarativeindex/control/control.go", declarativeIndexGuardFixture(t, "control.go", tc.source))
			if (len(signals) != 0) != tc.denied {
				t.Fatalf("Object coordinator guard result %v, want violation=%v", signals, tc.denied)
			}
		})
	}
}

func TestPrivateOpenSearchGuardRestrictsFactoryToPublicAdapter(t *testing.T) {
	for _, tc := range []struct {
		consumer string
		allowed  bool
	}{
		{"declarativeindex/opensearch", true},
		{"declarativeindex/opensearch/internal/opensearchimpl", true},
		{"declarativeindex/opensearch/internal/opensearchimpl/encoding", true},
		{"declarativeindex/projection", false},
		{"declarativeindex/control", false},
		{"declarativeindex/plugin", false},
		{"declarativeindex/internal/testfixture", false},
		{"declarativeindex/opensearch/another-factory", false},
		{"home", false},
		{"discovery", false},
	} {
		t.Run(tc.consumer, func(t *testing.T) {
			if got := privateOpenSearchConsumerAllowed(tc.consumer); got != tc.allowed {
				t.Fatalf("private driver consumer %s allowed=%v want=%v", tc.consumer, got, tc.allowed)
			}
		})
	}
}
