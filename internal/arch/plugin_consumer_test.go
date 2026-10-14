package arch_test

import "testing"

// The contribution registry consumes existing public ports. It must not gain
// the authority, implementation or transport that constructs or calls it.
func TestIndexPluginDependencyGuardRejectsHostAndImplementation(t *testing.T) {
	owner := "discovery/plugin"
	for _, tc := range []struct {
		target string
		denied bool
	}{
		{"discovery", false},
		{"discovery/projection", false},
		{"object", false},
		{"object/reader", false},
		{"object/maintenance", false},
		{"snapshot/dataset", false},
		{"home", true},
		{"discovery/service", true},
		{"object/writer", true},
		{"object/writer/internal/ledger", true},
		{"declarativeindex/plugin", true},
		{"declarativeindex/control", true},
		{"declarativeindex/projection", true},
		{"declarativeindex/opensearch", true},
		{"declarativeindex/opensearch/internal/opensearchimpl", true},
		{"discovery/opensearch", true},
		{"discovery/internal/opensearchimpl", true},
		{"discovery/llmhttp", true},
		{"discovery/client", true},
		{"declarativeindex/cli", true},
	} {
		t.Run(tc.target, func(t *testing.T) {
			for _, graph := range []depGraph{
				{owner: {tc.target}},
				{owner: {"contribution-bridge"}, "contribution-bridge": {tc.target}},
			} {
				if got := pluginDependencyForbidden(t, graph, owner); got != tc.denied {
					t.Fatalf("contribution dependency %s: forbidden=%v want=%v", tc.target, got, tc.denied)
				}
			}
		})
	}
}

func TestModuleClientsCannotReachIndexPluginRuntime(t *testing.T) {
	for _, owner := range []string{"discovery/client", "discovery/cli", "declarativeindex/client", "declarativeindex/cli"} {
		t.Run(owner, func(t *testing.T) {
			for _, target := range []string{"discovery/plugin", "discovery/plugin/internal/registry"} {
				graph := depGraph{owner: {"transport-bridge"}, "transport-bridge": {target}}
				if !pluginDependencyForbidden(t, graph, owner) {
					t.Fatalf("module client %s can reach runtime %s", owner, target)
				}
			}
		})
	}
}

func pluginDependencyForbidden(t *testing.T, graph depGraph, owner string) bool {
	t.Helper()
	var denied []string
	for _, rule := range forbidden {
		if rule.pkg == owner {
			denied = append(denied, rule.denied...)
		}
	}
	if len(denied) == 0 {
		t.Fatalf("%s has no dependency boundary", owner)
	}
	for _, target := range denied {
		if _, found := reachDenied(graph.reachable(owner), target); found {
			return true
		}
	}
	return false
}
