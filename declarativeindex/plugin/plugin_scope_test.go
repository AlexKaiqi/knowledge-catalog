package plugin_test

import (
	"context"
	"testing"

	retrieval "kc/discovery"
	consumerplugin "kc/discovery/plugin"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/snapshot"
)

func TestIndexPluginScopeCallbackCannotRebindConfiguredRepository(t *testing.T) {
	f := newPluginFixture(t)
	entry := f.entry(t)
	open := entry.Open
	entry.Open = func(scope consumerplugin.PluginScope) (consumerplugin.IndexContribution, error) {
		contribution, err := open(scope)
		if err == nil && scope.Instance == "main-a" {
			scope.Repositories[0] = f.setups[2].RepositoryID
		}
		return contribution, err
	}
	plugins, err := consumerplugin.OpenIndexPlugins(f.configs(), map[string]consumerplugin.IndexPluginEntry{"declarative-index": entry})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = plugins.Close() })
	assertScopeUnconfiguredSourceRejected(t, f, plugins)
	source := f.setups[0]
	repo, err := f.lookup(source.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	at := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
	if err := plugins.PrepareServingBasis(context.Background(), repo, at); err != nil {
		t.Fatalf("Entry callback replaced the configured Repository routing: %v", err)
	}
	result, err := plugins.SearchAtContext(context.Background(), repo, at, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	assertPluginHit(t, result, err, source.RepositoryID, at, "runbook source 1 at a")
}

func TestIndexPluginScopeRetainedSliceCannotChangeControllerInventory(t *testing.T) {
	f := newPluginFixture(t)
	entry := f.entry(t)
	open := entry.Open
	var retained []kernel.RepositoryID
	entry.Open = func(scope consumerplugin.PluginScope) (consumerplugin.IndexContribution, error) {
		contribution, err := open(scope)
		if err == nil && scope.Instance == "main-a" {
			retained = scope.Repositories
		}
		return contribution, err
	}
	plugins, err := consumerplugin.OpenIndexPlugins(f.configs(), map[string]consumerplugin.IndexPluginEntry{"declarative-index": entry})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = plugins.Close() })
	if len(retained) != 1 {
		t.Fatalf("fixture did not retain the actual source scope: %v", retained)
	}
	retained[0] = f.setups[2].RepositoryID
	assertScopeUnconfiguredSourceRejected(t, f, plugins)
	source := f.setups[0]
	// The actual Controller must discover and build its configured source;
	// direct preparation would bypass the inventory whose ownership is tested.
	if err := plugins.CatchUpRepository(context.Background(), source.RepositoryID); err != nil {
		t.Fatalf("retained scope changed the actual Controller inventory: %v", err)
	}
	repo, err := f.lookup(source.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	at := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
	result, err := plugins.SearchAtContext(context.Background(), repo, at, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	assertPluginHit(t, result, err, source.RepositoryID, at, "runbook source 1 at a")
}

func assertScopeUnconfiguredSourceRejected(t *testing.T, f *pluginFixture, plugins *consumerplugin.IndexPlugins) {
	t.Helper()
	source := f.setups[2]
	repo, err := f.lookup(source.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	at := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
	before := f.pool.operations.Load()
	result, err := plugins.SearchAtContext(context.Background(), repo, at, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || len(result.Hits) != 0 || f.pool.operations.Load() != before {
		t.Errorf("scope mutation admitted an unconfigured source to the physical index: %#v %v; operations before=%d after=%d", result, err, before, f.pool.operations.Load())
	}
}
