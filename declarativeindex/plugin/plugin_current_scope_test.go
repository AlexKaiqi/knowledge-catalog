package plugin_test

import (
	"context"
	"sync/atomic"
	"testing"

	retrieval "kc/discovery"
	consumerplugin "kc/discovery/plugin"
	"kc/internal/testkit"
	"kc/kernel"
	"kc/object/reader"
	"kc/snapshot"
)

func TestIndexCurrentAdmissionKeepsOneInstanceAndRejectsEveryForeignMember(t *testing.T) {
	f := newPluginFixture(t, true)
	admitted := snapshot.NewRegistry()
	for _, source := range f.setups[:2] {
		if err := admitted.Add(source.Repo.Snapshot()); err != nil {
			t.Fatal(err)
		}
	}
	entry := f.entry(t)
	open := entry.Open
	var opens atomic.Int64
	entry.Open = func(scope consumerplugin.PluginScope) (consumerplugin.IndexContribution, error) {
		opens.Add(1)
		return open(scope)
	}
	plugins, err := consumerplugin.OpenIndexPlugins(f.configs(), consumerplugin.PluginScope{Repositories: f.repositories(), ContainsRepository: func(id kernel.RepositoryID) bool { _, ok := admitted.Get(id); return ok }, RepositoryInventory: admitted.IDs}, map[string]consumerplugin.IndexPluginEntry{"declarative-index": entry})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = plugins.Close() })
	first, late := f.setups[0], f.setups[2]
	at := testkit.MustHead(t, late.Repo, snapshot.DefaultRef)
	pin := reader.DatasetPin{SetID: "late-source", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{late.RepositoryID: at}}
	pin.Items = reader.WholeRepositoryItems(pin.Repositories)
	mixed := reader.DatasetPin{SetID: "mixed", Revision: 1, Repositories: map[kernel.RepositoryID]kernel.CommitID{first.RepositoryID: testkit.MustHead(t, first.Repo, snapshot.DefaultRef), late.RepositoryID: at}}
	mixed.Items = reader.WholeRepositoryItems(mixed.Repositories)
	before := f.pool.operations.Load()
	if err := plugins.PrepareDatasetScope(context.Background(), f.lookup, mixed); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || f.pool.operations.Load() != before {
		t.Fatalf("non-leading foreign member reached the provider: %v; operations %d -> %d", err, before, f.pool.operations.Load())
	}
	if err := admitted.Add(late.Repo.Snapshot()); err != nil {
		t.Fatal(err)
	}
	if !plugins.ConfiguredFor(late.RepositoryID) {
		t.Fatal("late admitted source did not reach installed instance")
	}
	if err := plugins.PrepareDatasetScope(context.Background(), f.lookup, pin); err != nil {
		t.Fatal(err)
	}
	result, err := plugins.SearchDataset(context.Background(), f.lookup, pin, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	if err != nil || len(result.Hits) != 1 || result.Hits[0].Object.Repository != late.RepositoryID || result.Hits[0].Object.Commit != at {
		t.Fatalf("late source lost fixed Dataset results: %#v %v", result, err)
	}
	if err := plugins.CatchUpRepository(context.Background(), late.RepositoryID); err != nil {
		t.Fatalf("maintenance did not use current inventory: %v", err)
	}
	if opens.Load() != 1 {
		t.Fatal("admission reopened plugin instance")
	}
	admitted.Delete(late.RepositoryID)
	admitted.Delete(first.RepositoryID)
	before = f.pool.operations.Load()
	if plugins.ConfiguredFor(late.RepositoryID) || plugins.ConfiguredFor(first.RepositoryID) {
		t.Fatal("removed source retained membership")
	}
	if _, err := plugins.SearchDataset(context.Background(), f.lookup, pin, retrieval.SearchOf(retrieval.SearchMATCH("runbook"))); kernel.CodeOf(err) != kernel.ErrCapabilityUnsatisfied || f.pool.operations.Load() != before {
		t.Fatalf("removed source reached the provider: %v", err)
	}
	if overview, err := plugins.Overview(pin); err != nil || len(overview) != 0 {
		t.Fatalf("removed source retained an installed explanation: %#v %v", overview, err)
	}
}
