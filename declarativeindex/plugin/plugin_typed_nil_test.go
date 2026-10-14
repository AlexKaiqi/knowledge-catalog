package plugin_test

import (
	consumerplugin "kc/discovery/plugin"
	"testing"

	indexcontrol "kc/declarativeindex/control"
	indeximplementation "kc/declarativeindex/projection"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

func TestIndexPluginsRejectTypedNilContributionsAndReleaseActualResources(t *testing.T) {
	for _, missing := range []string{"queries", "maintenance"} {
		t.Run(missing, func(t *testing.T) {
			f := newPluginFixture(t)
			entry := f.entry(t)
			open := entry.Open
			entry.Open = func(scope consumerplugin.PluginScope) (consumerplugin.IndexContribution, error) {
				contribution, err := open(scope)
				if err != nil {
					return contribution, err
				}
				// Acquire a physical engine through the actual Index before making
				// the contribution invalid. Its release is observable, not a
				// stand-in registration or resource callback.
				for _, id := range scope.Repositories {
					repo, err := f.lookup(id)
					if err != nil {
						return contribution, err
					}
					if _, err := contribution.Queries.Ensure(repo, testkit.MustHead(t, repo, snapshot.DefaultRef)); err != nil {
						return contribution, err
					}
				}
				if missing == "queries" {
					var absent *indeximplementation.Index
					contribution.Queries = absent
				} else {
					var absent *indexcontrol.Controller
					contribution.Maintenance = absent
				}
				return contribution, nil
			}
			plugins, err := consumerplugin.OpenIndexPlugins(f.configs(), map[string]consumerplugin.IndexPluginEntry{"declarative-index": entry})
			if plugins != nil {
				t.Cleanup(func() { _ = plugins.Close() })
			}
			if plugins != nil || kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
				t.Errorf("typed nil %s was accepted as an executable contribution: runtimeAccepted=%t error=%v", missing, plugins != nil, err)
			}
			assertPluginEngineCloseCounts(t, f.pool, 1)
		})
	}
}
