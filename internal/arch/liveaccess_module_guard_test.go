package arch_test

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestLiveAccessOwnershipGuardRejectsCopiedImplementationsAndShims(t *testing.T) {
	base := liveAccessSourceFiles(t)
	if signals := liveAccessOwnershipSignals(base); len(signals) != 0 {
		t.Fatalf("correct module ownership rejected: %v", signals)
	}
	for _, fixture := range []struct{ name, path, source, symbol string }{
		{"object-binding", "object/reader/copied.go", "package reader; type ResolvedBinding struct{}", "ResolvedBinding"},
		{"object-handler", "object/copied.go", "package object; type LiveAccessHandler struct{}", "LiveAccessHandler"},
		{"generic-plugin-selector", "discovery/plugin/copied.go", "package plugin; type Plugins struct{}", "Plugins"},
		{"host-evidence", "invocation/copied.go", "package invocation; func RetainReadEvidence() {}", "RetainReadEvidence"},
		{"host-hydration", "cli/copied.go", "package cli; func HydrateRepositoryRead() {}", "HydrateRepositoryRead"},
		{"alias-in-owner", "liveaccess/state_lookup.go", "package liveaccess; import old \"kc/object/reader\"; type HTTPStateLookup = old.HTTPStateLookup", "HTTPStateLookup"},
		{"defined-forwarding-type", "liveaccess/copied.go", "package liveaccess; import old \"kc/object/reader\"; type LiveAccessHandler old.LiveAccessHandler", "LiveAccessHandler"},
		{"forwarding-value", "liveaccess/copied.go", "package liveaccess; import old \"kc/object/reader\"; var NewHTTPStateLookup = old.NewHTTPStateLookup", "NewHTTPStateLookup"},
		{"evidence-alias", "liveaccess/evidence/copied.go", "package evidence; import old \"kc/observability\"; type Collector = old.Collector", "Collector"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			files := maps.Clone(base)
			files[fixture.path] = declarativeIndexGuardFixture(t, fixture.path, fixture.source)
			signals := liveAccessOwnershipSignals(files)
			if !slices.ContainsFunc(signals, func(signal string) bool {
				return strings.Contains(signal, fixture.path) && strings.Contains(signal, "concrete "+fixture.symbol)
			}) {
				t.Fatalf("implementation ownership regression was not identified: %v", signals)
			}
		})
	}
	missing := maps.Clone(base)
	delete(missing, "liveaccess/service/evidence_request.go")
	if signals := liveAccessOwnershipSignals(missing); !slices.ContainsFunc(signals, func(signal string) bool {
		return strings.Contains(signal, "one concrete RetainReadEvidence declaration")
	}) {
		t.Fatalf("missing owned implementation was accepted: %v", signals)
	}
}

func TestLiveAccessDependencyGuardPreservesCoreDenominator(t *testing.T) {
	// These base prohibitions cannot disappear when new module SDKs or
	// evidence paths are admitted as explicit public capabilities. The handler
	// contract now lives inside LiveAccess and is checked as an owner below.
	for _, owner := range []string{"object", "object/reader", "object/writer", "object/service", "discovery/projection"} {
		t.Run(owner, func(t *testing.T) {
			good := depGraph{owner: {"independent-port"}, "independent-port": {"kernel"}}
			if signals := liveAccessDependencySignals(good); len(signals) != 0 {
				t.Fatalf("neutral base contract was rejected: %v", signals)
			}
			bad := depGraph{owner: {"hidden-bridge"}, "hidden-bridge": {"liveaccess/evidence/internal/copied"}}
			if signals := liveAccessDependencySignals(bad); !slices.ContainsFunc(signals, func(signal string) bool {
				return strings.Contains(signal, "base consumer "+owner+" reaches LiveAccess")
			}) {
				t.Fatalf("core prohibition no longer covers indirect LiveAccess subpackages: %v", signals)
			}
		})
	}
}

func TestLiveAccessDependencyGuardRejectsIndirectHostAndRuntime(t *testing.T) {
	for _, owner := range []string{"liveaccess/cli", "liveaccess/client"} {
		for _, target := range []string{"liveaccess", "liveaccess/service", "liveaccess/plugin", "liveaccess/evidence", "object/writer", "object/service", "discovery/plugin", "declarativeindex/control", "invocation", "home", "server", "client"} {
			t.Run(owner+"/"+target, func(t *testing.T) {
				graph := depGraph{owner: {"hidden-bridge"}, "hidden-bridge": {target + "/internal/copy"}}
				if signals := liveAccessDependencySignals(graph); len(signals) == 0 {
					t.Fatalf("independent consumer reached runtime %s without a signal", target)
				}
			})
		}
	}
	good := depGraph{"liveaccess/cli": {"liveaccess/client"}, "liveaccess/client": {"object/client", "internal/httpjson"}, "object/client": {"kernel", "object"}}
	if signals := liveAccessDependencySignals(good); len(signals) != 0 {
		t.Fatalf("module-owned caller transport was rejected: %v", signals)
	}
	for _, owner := range []string{"liveaccess/service", "liveaccess/plugin", "liveaccess/evidence"} {
		for _, target := range []string{"home", "invocation", "server", "cli", "client", "identity", "object/writer"} {
			graph := depGraph{owner: {"hidden-bridge"}, "hidden-bridge": {target}}
			if signals := liveAccessDependencySignals(graph); len(signals) == 0 {
				t.Fatalf("owned implementation %s reached host %s without a signal", owner, target)
			}
		}
	}
}

func TestLiveAccessCLIHostOwnershipGuardRejectsScatteredLeaves(t *testing.T) {
	owned := liveAccessCLILeaves(t)
	for path := range owned.paths {
		for boundary, source := range map[string]string{
			"command surface":  fmt.Sprintf("var cliSurface = map[string]surface{%q:{}}", path),
			"helpEntries":      fmt.Sprintf("var helpEntries = map[string]entry{%q:{}}", path),
			"helpGroups":       fmt.Sprintf("var helpGroups = map[string][]string{\"owned\":{%q}}", path),
			"runRemoteRequest": fmt.Sprintf("func runRemoteRequest(path string){ if path==%q {runLeaf()} }", path),
			"encodeCLIProduct": fmt.Sprintf("func encodeCLIProduct(path string){ switch path {case %q: shapeLeaf()} }", path),
		} {
			want := boundary
			if boundary == "command surface" {
				want = "cliSurface"
			}
			t.Run(path+"/"+boundary, func(t *testing.T) {
				assertSnapshotCLIOwnershipFixture(t, "cli/host.go", source, owned, want)
			})
		}
	}
	for handler := range owned.handlers {
		assertSnapshotCLIOwnershipFixture(t, "cli/operands.go", fmt.Sprintf("func applyPositionals(handler string){ if handler==%q {bindLeaf()} }", handler), owned, "applyPositionals")
	}
	assertSnapshotCLIOwnershipFixture(t, "cli/output.go", `import "strings"; func runRemoteRequest(path string){ if strings.HasPrefix(path,"binding "){runLeaf()} }`, owned, "runRemoteRequest")
	assertSnapshotCLIOwnershipFixture(t, "cli/surface.go", `func register(){ cliSurface["access"]=surface{} }`, owned, "cliSurface")
}

func TestLiveAccessCLIHostOwnershipGuardAllowsAssembly(t *testing.T) {
	source := `
var cliSurface = composeLiveAccessCLICommands(Commands(), map[string]surface{"create":{}})
var helpEntries = mergeHelpEntries(liveAccessHelpEntries(), hostOtherHelpEntries())
func hostOtherHelpEntries() map[string]entry { return map[string]entry{"operations audit access":{}} }
var helpGroups = composeLiveAccessHelpGroups(map[string][]string{"journey":{"read","access","operations audit access"}})
func applyPositionals(handler string) { if entry, module := liveAccessCommandByHandler(handler); module { bindModuleOperands(entry) } }
func runRemoteRequest(path string) { if liveAccessCommandPath(path) { executeLiveAccessCommand(path) } }
func encodeCLIProduct(path string) { if path == "read" { shapeHost() } }
`
	assertSnapshotCLIOwnershipFixture(t, "cli/adapter.go", source, liveAccessCLILeaves(t), "")
}
