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

func TestIndexPluginsMissingEntryDoesNotBlockIndependentSourceOrFixedReads(t *testing.T) {
	f := newPluginFixture(t)
	configs := f.configs()
	a, b := f.setups[0], f.setups[1]
	configs.Repositories[a.RepositoryID] = consumerplugin.RepositoryPluginConfig{Version: 1, Plugins: []consumerplugin.PluginSelection{{Name: "missing-index", Instance: "source-a"}}}
	plugins, err := consumerplugin.OpenIndexPlugins(configs, map[string]consumerplugin.IndexPluginEntry{"declarative-index": f.entry(t)})
	if err != nil {
		t.Fatalf("source A's missing entry blocked independent source B: %v", err)
	}
	t.Cleanup(func() { _ = plugins.Close() })
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
	if _, err := plugins.SearchAtContext(context.Background(), repoA, atA, query); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing entry used hidden fallback: %v", err)
	}
	if err := plugins.PrepareServingBasis(context.Background(), repoA, atA); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied {
		t.Fatalf("missing entry counted as prepared: %v", err)
	}
	atB := testkit.MustHead(t, b.Repo, snapshot.DefaultRef)
	repoB, err := f.lookup(b.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if err := plugins.PrepareServingBasis(context.Background(), repoB, atB); err != nil {
		t.Fatal(err)
	}
	result, err := plugins.SearchAtContext(context.Background(), repoB, atB, query)
	assertPluginHit(t, result, err, b.RepositoryID, atB, "runbook source 2 at a")
	if len(f.pool.repositories("declarative-index/main-a")) != 0 {
		t.Fatal("unavailable entry executed another implementation")
	}
}
