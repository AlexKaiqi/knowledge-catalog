package plugin_test

import (
	"context"
	consumerplugin "kc/discovery/plugin"
	"testing"

	retrieval "kc/discovery"
	"kc/internal/testkit"
	"kc/kernel"
	objectmodel "kc/object"
	"kc/snapshot"
)

// An instance of a plugin this build does not compile is never opened: the
// deployment has no index capability, declarations still read, and search
// fails closed without a hidden fallback implementation.
func TestIndexPluginsUncompiledEntryInstallsNoCapabilityAndKeepsFixedReads(t *testing.T) {
	f := newPluginFixture(t)
	configs := consumerplugin.PluginsConfig{"missing-index": {Instances: map[string]consumerplugin.PluginInstanceConfig{"source-a": {Catalog: "missing-index/v1"}}}}
	plugins, err := consumerplugin.OpenIndexPlugins(configs, consumerplugin.PluginScope{Repositories: f.repositories()}, map[string]consumerplugin.IndexPluginEntry{"declarative-index": f.entry(t)})
	if err != nil {
		t.Fatalf("uncompiled entry blocked the deployment registry: %v", err)
	}
	t.Cleanup(func() { _ = plugins.Close() })
	if plugins.Configured() {
		t.Fatal("an uncompiled entry was counted as an installed index instance")
	}
	a := f.setups[0]
	query := retrieval.SearchOf(retrieval.SearchMATCH("runbook"))
	atA := testkit.MustHead(t, a.Repo, snapshot.DefaultRef)
	value, err := f.reader.Read(objectmodel.ObjectRef{Repository: a.RepositoryID, Object: "article/one"}, atA, nil)
	if err != nil || value.Value.(map[string]any)["body"] != "runbook source 1 at a" {
		t.Fatalf("fixed read depended on unavailable implementation: %#v %v", value, err)
	}
	repoA, err := f.lookup(a.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	before := f.pool.operations.Load()
	if _, err := plugins.SearchAtContext(context.Background(), repoA, atA, query); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing entry used hidden fallback: %v", err)
	}
	if err := plugins.PrepareServingBasis(context.Background(), repoA, atA); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing entry counted as prepared: %v", err)
	}
	if f.pool.operations.Load() != before || len(f.pool.namespaces()) != 0 {
		t.Fatal("unavailable entry executed another implementation")
	}
}
