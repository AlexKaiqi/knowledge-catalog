package arch_test

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestPluginAdoptionOwnershipGuardRejectsCopiesAndWrongAliases(t *testing.T) {
	base := productionSourceFiles(t, nil)
	if signals := pluginAdoptionOwnershipSignals(base); len(signals) != 0 {
		t.Fatalf("one canonical definition and compatible public aliases rejected: %v", signals)
	}
	for _, fixture := range []struct{ path, source, signal string }{
		{"discovery/plugin/copied.go", "package plugin; type PluginScope struct{}", "copies or misroutes shared adoption PluginScope"},
		{"liveaccess/service/copied.go", "package service; type RepositoryPluginConfig struct{}", "copies or misroutes shared adoption RepositoryPluginConfig"},
		{"home/copied.go", "package home; type PluginSelection struct{Name string; Instance string}", "copies or misroutes shared adoption PluginSelection"},
		{"liveaccess/cli/affordance.go", "package cli; type PluginAffordance struct{}", "copies or misroutes shared adoption PluginAffordance"},
		{"declarativeindex/cli/affordance.go", "package cli; import old \"kc/discovery/plugin\"; type PluginAffordance = old.PluginAffordance", "copies or misroutes shared adoption PluginAffordance"},
		{"discovery/plugin/config.go", "package plugin; import common \"kc/discovery/adoption\"; type PluginScope = common.SourcePlugin", "copies or misroutes shared adoption PluginScope"},
		{"discovery/plugin/config.go", "package plugin; import common \"kc/discovery/adoption\"; func ParseRepositoryPlugins(){return common.SelectRepositoryPlugins()}", "copies or misroutes shared adoption ParseRepositoryPlugins"},
		{"discovery/adoption/extra.go", "package adoption; type ArbitraryContribution struct{Role string; Value any}", "exported surface adds unselected ArbitraryContribution"},
	} {
		t.Run(fixture.signal+fixture.path, func(t *testing.T) {
			files := maps.Clone(base)
			files[fixture.path] = declarativeIndexGuardFixture(t, fixture.path, fixture.source)
			if signals := pluginAdoptionOwnershipSignals(files); !slices.ContainsFunc(signals, func(signal string) bool { return strings.Contains(signal, fixture.signal) }) {
				t.Fatalf("shared contract guard did not report %q: %v", fixture.signal, signals)
			}
		})
	}
	missing := maps.Clone(base)
	delete(missing, "discovery/adoption/scope.go")
	if signals := pluginAdoptionOwnershipSignals(missing); !slices.ContainsFunc(signals, func(signal string) bool { return strings.Contains(signal, "PluginScope must have one definition") }) {
		t.Fatalf("shared contract guard accepted a missing canonical scope: %v", signals)
	}
}

func TestPluginAdoptionIsolationGuardRejectsIndirectExecution(t *testing.T) {
	if signals := pluginAdoptionDependencySignals(depGraph{"discovery/adoption": {"kernel"}}); len(signals) != 0 {
		t.Fatalf("pure common contract was rejected: %v", signals)
	}
	for _, target := range []string{"discovery/plugin", "discovery/projection", "declarativeindex/control", "liveaccess/service", "home", "server", "object/reader"} {
		graph := depGraph{"discovery/adoption": {"bridge"}, "bridge": {target}}
		if signals := pluginAdoptionDependencySignals(graph); !slices.ContainsFunc(signals, func(signal string) bool { return strings.Contains(signal, target) }) {
			t.Fatalf("shared facts reached execution %s without an isolation signal: %v", target, signals)
		}
	}
	for _, owner := range []string{"liveaccess/service", "liveaccess/plugin", "liveaccess/evidence"} {
		graph := depGraph{owner: {"bridge"}, "bridge": {"discovery/plugin/internal/copied"}}
		if signals := liveAccessDependencySignals(graph); len(signals) == 0 {
			t.Fatalf("LiveAccess regained a transitive index registry dependency: %s", owner)
		}
	}
}
