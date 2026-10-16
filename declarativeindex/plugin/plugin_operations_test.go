package plugin_test

import (
	"context"
	"errors"
	consumerplugin "kc/discovery/plugin"
	"testing"

	retrieval "kc/discovery"
	index "kc/discovery/projection"
	"kc/discovery/service"
	"kc/internal/testkit"
	"kc/snapshot"
)

// One instance interprets every deployed source; syncing one source must not
// run another source's physical maintenance inside the same instance.
func TestIndexPluginsSyncOneSourceDoesNotRunAnotherSourcesMaintenance(t *testing.T) {
	f := newPluginFixture(t)
	other := f.setups[0]
	f.pool.rebuildFailures = map[string]error{string(other.RepositoryID): errors.New("source A physical preparation failed")}
	plugins, err := consumerplugin.OpenIndexPlugins(f.configs(), consumerplugin.PluginScope{Repositories: f.repositories()}, map[string]consumerplugin.IndexPluginEntry{"declarative-index": f.entry(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = plugins.Close() })
	source := f.setups[1]
	at := testkit.MustHead(t, source.Repo, snapshot.DefaultRef)
	repo, err := f.lookup(source.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	executor := service.ProjectionSyncExecutor{Repositories: pluginRepositoryLookup{f}, Projection: plugins, Controller: plugins}
	result, err := executor.Execute(context.Background(), service.ProjectionSyncRequest{Repository: source.RepositoryID, Commit: at})
	if err != nil {
		t.Fatalf("source B's independent sync ran source A's physical failure: %v", err)
	}
	// The source's policy plus index/fixture, the object holding its declaration.
	if result.State != nil || result.Snapshot.Repository != source.RepositoryID || result.Snapshot.BasisCommit != at || result.Snapshot.ObjectCount != 2 || result.Snapshot.AccessDigest == "" {
		t.Fatalf("source B did not actually prepare its source basis: %#v", result)
	}
	query, err := plugins.SearchAtContext(context.Background(), repo, at, retrieval.SearchOf(retrieval.SearchMATCH("runbook")))
	assertPluginHit(t, query, err, source.RepositoryID, at, "runbook source 2 at a")
	targets, err := plugins.Targets()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.Repository != source.RepositoryID {
			t.Fatalf("source B's scoped sync created another source's maintenance target: %#v", target)
		}
	}
	if len(targets) != 1 || targets[0].DesiredCommit != at || targets[0].AppliedCommit != at || targets[0].Status != index.TargetReady {
		t.Fatalf("source B's own maintenance target was not ready: %#v", targets)
	}
	if handles := f.pool.handlesFor(other.RepositoryID); handles != 0 {
		t.Fatalf("source B's sync acquired source A's physical resources: %d handles", handles)
	}
}
