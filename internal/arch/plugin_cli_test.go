package arch_test

import "testing"

// Static descriptions and owned command adapters must not pull an execution
// service or plugin runtime into independently adopted module CLIs.
func TestPluginCLIHasNoRuntimeImplementationDependencies(t *testing.T) {
	graph := loadGraph(t)
	for _, owner := range []string{"discovery/cli", "declarativeindex/cli"} {
		reachable := graph.reachable(owner)
		for _, denied := range []string{
			"home", "invocation", "server", "discovery/service", "discovery/plugin", "discovery/projection",
			"declarativeindex/plugin", "declarativeindex/control", "declarativeindex/projection",
			"declarativeindex/opensearch", "declarativeindex/opensearch/internal/opensearchimpl",
			"liveaccess", "object/service", "object/writer", "object/maintenance", "snapshot/service",
		} {
			if path, found := reachDenied(reachable, denied); found {
				t.Errorf("module CLI %s reaches runtime implementation %s: %v", owner, denied, path)
			}
		}
	}
}
